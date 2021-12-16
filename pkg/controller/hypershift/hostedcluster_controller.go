// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hypershift

import (
	"context"
	"fmt"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/library-go/pkg/operator/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "hostedcluster-controller"

var log = logf.Log.WithName(controllerName)

// Add creates a new managedcluster controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder,
	importSecretInformer, autoImportSecretInformer cache.SharedIndexInformer) (string, error) {
	_ = autoImportSecretInformer
	return controllerName, add(importSecretInformer, mgr, newReconciler(clientHolder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(clientHolder *helpers.ClientHolder) reconcile.Reconciler {
	return &ReconcileHostedcluster{
		client:     clientHolder.RuntimeClient,
		kubeClient: clientHolder.KubeClient,
		recorder:   helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
	}
}

// adds a new Controller to mgr with r as the reconcile.Reconciler
func add(importSecretInformer cache.SharedIndexInformer, mgr manager.Manager, r reconcile.Reconciler) error {
	_ = importSecretInformer
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return err
	}

	// TODO might need to watch with hostedcluster label
	if err := c.Watch(
		&runtimesource.Kind{Type: &hyperv1.HostedCluster{}},
		&handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	return nil
}

// ReconcileHostedcluster reconciles the hostedcluster that is in the hosted cluster namespace
// to import the managed cluster
type ReconcileHostedcluster struct {
	client     client.Client
	kubeClient kubernetes.Interface
	recorder   events.Recorder
}

// blank assignment to verify that ReconcileHostedcluster implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileHostedcluster{}

// Reconcile the hostedcluster that is in the managed cluster namespace to import the managed cluster.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileHostedcluster) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request", request)
	reqLogger.Info("Reconciling HostedCluster")
	defer reqLogger.Info("Reocileing HostedCluster Done")

	clusterName := request.Name
	hCluster := &hyperv1.HostedCluster{}
	err := r.client.Get(ctx, request.NamespacedName, hCluster)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if !hCluster.DeletionTimestamp.IsZero() {
		// the hostedcluster is deleting, its managed cluster may already be detached (the managed cluster has been deleted,
		// but the namespace is remained), if it has import finalizer, we remove its namespace
		return reconcile.Result{}, r.removeImportFinalizer(ctx, hCluster)
	}

	// TODO: check if UI would do so or not
	if err := r.ensureManagedClusterCR(ctx, request.NamespacedName); err != nil {
		return reconcile.Result{}, err
	}

	if hCluster.Status.KubeConfig == nil {
		return reconcile.Result{}, fmt.Errorf("hostedcluster's kubeconfig secret is not generated yet")
	}

	hosteKubeSecertKey := types.NamespacedName{
		Name:      hCluster.Status.KubeConfig.Name,
		Namespace: hCluster.Namespace}

	hostedKubeconfigSecret := &corev1.Secret{}

	if err := r.client.Get(ctx, hosteKubeSecertKey, hostedKubeconfigSecret); err != nil {
		return reconcile.Result{}, err
	}

	// add a managed cluster finalizer to the hosted cluster, to handle the managed cluster detach case.
	if err := r.addClusterImportFinalizer(ctx, hCluster); err != nil {
		return reconcile.Result{}, err
	}

	hostedClusterClient, restMapper, err := helpers.GenerateClientFromSecret(hostedKubeconfigSecret)
	if err != nil {
		return reconcile.Result{}, err
	}

	importSecretName := fmt.Sprintf("%s-%s", clusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.kubeClient.CoreV1().Secrets(clusterName).Get(ctx, importSecretName, metav1.GetOptions{})
	if err != nil {
		return reconcile.Result{}, err
	}

	importCondition := metav1.Condition{
		Type:    "ManagedClusterImportSucceeded",
		Status:  metav1.ConditionTrue,
		Message: "Import succeeded",
		Reason:  "ManagedClusterImported",
	}

	errs := []error{}
	err = helpers.ImportManagedClusterFromSecret(hostedClusterClient, restMapper, r.recorder, importSecret)
	if err != nil {
		errs = append(errs, err)

		importCondition.Status = metav1.ConditionFalse
		importCondition.Message = fmt.Sprintf("Unable to import %s: %s", clusterName, err.Error())
		importCondition.Reason = "ManagedClusterNotImported"
	}

	if err := helpers.UpdateManagedClusterStatus(r.client, r.recorder, clusterName, importCondition); err != nil {
		errs = append(errs, err)
	}

	// TODO: check if UI would do so or not
	if err := r.enableAddons(ctx, request.NamespacedName); err != nil {
		errs = append(errs, err)
	}

	return reconcile.Result{}, utilerrors.NewAggregate(errs)
}

func (r *ReconcileHostedcluster) ensureManagedClusterCR(ctx context.Context, hClusterKey types.NamespacedName) error {
	managedClusterNs := &corev1.Namespace{}
	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: hClusterKey.Name},
		Spec:       clusterv1.ManagedClusterSpec{HubAcceptsClient: true},
	}

	err := r.client.Get(ctx, types.NamespacedName{Name: hClusterKey.Name}, managedClusterNs)
	if errors.IsNotFound(err) {
		managedClusterNs.SetName(hClusterKey.Name)
		if err := r.client.Create(ctx, managedClusterNs); err != nil {
			return fmt.Errorf("failed to created managedcluster namespace, err: %w", err)
		}

		if err := r.client.Create(ctx, managedCluster); err != nil {
			return fmt.Errorf("failed to created managedcluster CR, err: %w", err)
		}

		return nil
	}

	if err != nil {
		return err
	}

	if !managedClusterNs.DeletionTimestamp.IsZero() {
		return nil
	}

	err = r.client.Get(ctx, types.NamespacedName{Name: hClusterKey.Name}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could be deleted, do nothing
		return nil
	}

	return err
}

func (r *ReconcileHostedcluster) enableAddons(ctx context.Context, hClusterKey types.NamespacedName) error {
	addOnKey := types.NamespacedName{Name: hClusterKey.Name, Namespace: hClusterKey.Name}

	// https://github.com/open-cluster-management/klusterlet-addon-controller/blob/f2dc73ecfb2046fc2e0dbc93a9e766b8c7d50c69/pkg/apis/agent/v1/klusterletaddonconfig_types.go#L18-L56
	addOnConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "agent.open-cluster-management.io/v1",
			"kind":       "KlusterletAddonConfig",
			"metadata": map[string]interface{}{
				"name":      hClusterKey.Name,
				"namespace": hClusterKey.Name,
			},
			"spec": map[string]interface{}{
				"clusterLabels": map[string]string{
					"cloud":  "auto-detect",
					"vendor": "auto-detect",
				},
				"applicationManager": map[string]interface{}{
					"enabled": true,
				},
				"certPolicyController": map[string]interface{}{
					"enabled": true,
				},
				"iamPolicyController": map[string]interface{}{
					"enabled": true,
				},
				"policyController": map[string]interface{}{
					"enabled": true,
				},
				"searchCollector": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	if err := r.client.Get(ctx, addOnKey, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "agent.open-cluster-management.io/v1",
			"kind":       "KlusterletAddonConfig"},
	}); err != nil {
		if errors.IsNotFound(err) {
			return r.client.Create(ctx, addOnConfig)
		}

		return fmt.Errorf("failed to enable klusteraddonconfig, err: %w", err)
	}

	return nil
}

func (r *ReconcileHostedcluster) addClusterImportFinalizer(
	ctx context.Context, hCluster *hyperv1.HostedCluster) error {
	patch := client.MergeFrom(hCluster.DeepCopy())
	for i := range hCluster.Finalizers {
		if hCluster.Finalizers[i] == constants.ImportFinalizer {
			return nil
		}
	}

	hCluster.Finalizers = append(hCluster.Finalizers, constants.ImportFinalizer)
	if err := r.client.Patch(ctx, hCluster, patch); err != nil {
		return err
	}

	r.recorder.Eventf("HostedClusterFinalizerAdded",
		"The hostedcluster %s finalizer %s is added", hCluster.Name, constants.ImportFinalizer)
	return nil
}

func (r *ReconcileHostedcluster) removeImportFinalizer(ctx context.Context, hCluster *hyperv1.HostedCluster) error {
	hasImportFinalizer := false

	for _, finalizer := range hCluster.Finalizers {
		if finalizer == constants.ImportFinalizer {
			hasImportFinalizer = true
			break
		}
	}

	if !hasImportFinalizer {
		// the hostedcluster does not have import finalizer, ignore it
		log.Info(fmt.Sprintf("the hostedCluster %s does not have import finalizer, skip it", hCluster.Name))
		return nil
	}

	// TODO: maybe I don't need to wait till the end
	//	if len(hCluster.Finalizers) != 1 {
	//		// the hostedcluster has other finalizers, wait hive to remove them
	//		log.Info(fmt.Sprintf("wait hypershift to remove the finalizers from the hostedCluster %s", hCluster.Name))
	//		return nil
	//	}

	// the hostedcluster alreay be cleaned up by hive, we delete its namespace and remove the import finalizer
	err := r.client.Get(ctx, types.NamespacedName{Name: hCluster.Name, Namespace: hCluster.Namespace}, &hyperv1.HostedCluster{})
	if errors.IsNotFound(err) {
		err := r.client.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hCluster.Name}})
		if err != nil {
			return err
		}

		r.recorder.Eventf("ManagedClusterNamespaceDeleted",
			"The managed cluster namespace %s is deleted", hCluster.Name)
	}

	if err := r.client.Delete(ctx, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "agent.open-cluster-management.io/v1",
			"kind":       "KlusterletAddonConfig",
			"metadata": map[string]interface{}{
				"name":      hCluster.GetName(),
				"namespace": hCluster.GetName(),
			},
		},
	}); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("hypershift failed to delete KlusterletAddonConfig CR, err: %w", err)
		}
	}

	if err := r.client.Delete(ctx,
		&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{Name: hCluster.Name}},
	); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("hypershift failed to delete managedCluster CR, err: %w", err)
		}
	}

	patch := client.MergeFrom(hCluster.DeepCopy())
	hCluster.Finalizers = []string{}
	if err := r.client.Patch(ctx, hCluster, patch); err != nil {
		return err
	}

	r.recorder.Eventf("HostedClusterFinalizerRemoved",
		"The hostedcluster %s finalizer %s is removed", hCluster.Name, constants.ImportFinalizer)
	return nil

}
