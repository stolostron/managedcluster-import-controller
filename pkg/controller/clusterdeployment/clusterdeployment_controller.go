// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusterdeployment

import (
	"context"
	"fmt"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(controllerName)

// ReconcileClusterDeployment reconciles the clusterdeployment that is in the managed cluster namespace
// to import the managed cluster
type ReconcileClusterDeployment struct {
	client     client.Client
	kubeClient kubernetes.Interface
	recorder   events.Recorder
}

// blank assignment to verify that ReconcileClusterDeployment implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// Reconcile the clusterdeployment that is in the managed cluster namespace to import the managed cluster.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileClusterDeployment) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling clusterdeployment")

	clusterName := request.Name

	clusterDeployment := &hivev1.ClusterDeployment{}
	err := r.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: clusterName}, clusterDeployment)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if !clusterDeployment.DeletionTimestamp.IsZero() {
		// the clusterdeployment is deleting, its managed cluster may already be detached (the managed cluster has been deleted,
		// but the namespace is remained), if it has import finalizer, we remove its namespace
		return reconcile.Result{}, r.removeImportFinalizer(ctx, clusterDeployment)
	}

	managedCluster := &clusterv1.ManagedCluster{}
	err = r.client.Get(ctx, types.NamespacedName{Name: clusterName}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could be deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if !clusterDeployment.Spec.Installed {
		// cluster deployment is not installed yet, do nothing
		return reconcile.Result{}, nil
	}

	// add a managed cluster finalizer to the cluster deployment, to handle the managed cluster detach case.
	if err := r.addClusterImportFinalizer(ctx, clusterDeployment); err != nil {
		return reconcile.Result{}, err
	}

	// set managed cluster created-via annotation
	if err := r.setCreatedViaAnnotation(ctx, clusterDeployment, managedCluster); err != nil {
		return reconcile.Result{}, err
	}

	// if there is an auto import secret in the managed cluster namespce, we will use the auto import secret to import the cluster
	_, err = r.kubeClient.CoreV1().Secrets(clusterName).Get(ctx, constants.AutoImportSecretName, metav1.GetOptions{})
	if err == nil {
		reqLogger.Info(fmt.Sprintf("The hive managed cluster %s has auto import secret, skipped", clusterName))
		return reconcile.Result{}, nil
	}
	if !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	secretRefName := clusterDeployment.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name
	hiveSecret, err := r.kubeClient.CoreV1().Secrets(clusterName).Get(ctx, secretRefName, metav1.GetOptions{})
	if err != nil {
		return reconcile.Result{}, err
	}
	hiveClient, restMapper, err := helpers.GenerateClientFromSecret(hiveSecret)
	if err != nil {
		return reconcile.Result{}, err
	}

	importSecretName := fmt.Sprintf("%s-%s", clusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.kubeClient.CoreV1().Secrets(clusterName).Get(ctx, importSecretName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
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
	err = helpers.ImportManagedClusterFromSecret(hiveClient, restMapper, r.recorder, importSecret)
	if err != nil {
		errs = append(errs, err)

		importCondition.Status = metav1.ConditionFalse
		importCondition.Message = fmt.Sprintf("Unable to import %s: %s", clusterName, err.Error())
		importCondition.Reason = "ManagedClusterNotImported"
	}

	if err := helpers.UpdateManagedClusterStatus(r.client, r.recorder, clusterName, importCondition); err != nil {
		errs = append(errs, err)
	}

	return reconcile.Result{}, utilerrors.NewAggregate(errs)
}

func (r *ReconcileClusterDeployment) setCreatedViaAnnotation(
	ctx context.Context, clusterDeployment *hivev1.ClusterDeployment, cluster *clusterv1.ManagedCluster) error {
	patch := client.MergeFrom(cluster.DeepCopy())
	modified := resourcemerge.BoolPtr(false)
	if clusterDeployment.Spec.Platform.AgentBareMetal != nil {
		resourcemerge.MergeMap(modified, &cluster.Labels, map[string]string{constants.CreatedViaAnnotation: constants.CreatedViaAI})
	} else {
		resourcemerge.MergeMap(modified, &cluster.Labels, map[string]string{constants.CreatedViaAnnotation: constants.CreatedViaHive})
	}

	if !*modified {
		return nil
	}

	// using patch method to avoid error: "the object has been modified; please apply your changes to the
	// latest version and try again", see:
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1509
	// https://github.com/kubernetes-sigs/controller-runtime/issues/741
	if err := r.client.Patch(ctx, cluster, patch); err != nil {
		return err
	}

	r.recorder.Eventf("ManagedClusterLabelsUpdated", "The managed cluster %s labels is added", cluster.Name)
	return nil
}

func (r *ReconcileClusterDeployment) addClusterImportFinalizer(
	ctx context.Context, clusterDeployment *hivev1.ClusterDeployment) error {
	patch := client.MergeFrom(clusterDeployment.DeepCopy())
	for i := range clusterDeployment.Finalizers {
		if clusterDeployment.Finalizers[i] == constants.ImportFinalizer {
			return nil
		}
	}

	clusterDeployment.Finalizers = append(clusterDeployment.Finalizers, constants.ImportFinalizer)
	if err := r.client.Patch(ctx, clusterDeployment, patch); err != nil {
		return err
	}

	r.recorder.Eventf("ClusterDeploymentFinalizerAdded",
		"The clusterdeployment %s finalizer %s is added", clusterDeployment.Name, constants.ImportFinalizer)
	return nil
}

func (r *ReconcileClusterDeployment) removeImportFinalizer(ctx context.Context, clusterDeployment *hivev1.ClusterDeployment) error {
	hasImportFinalizer := false

	for _, finalizer := range clusterDeployment.Finalizers {
		if finalizer == constants.ImportFinalizer {
			hasImportFinalizer = true
			break
		}
	}

	if !hasImportFinalizer {
		// the clusterdeployment does not have import finalizer, ignore it
		log.Info(fmt.Sprintf("the clusterDeployment %s does not have import finalizer, skip it", clusterDeployment.Name))
		return nil
	}

	if len(clusterDeployment.Finalizers) != 1 {
		// the clusterdeployment has other finalizers, wait hive to remove them
		log.Info(fmt.Sprintf("wait hive to remove the finalizers from the clusterdeployment %s", clusterDeployment.Name))
		return nil
	}

	// the clusterdeployment alreay be cleaned up by hive, we delete its namespace and remove the import finalizer
	err := r.client.Get(ctx, types.NamespacedName{Name: clusterDeployment.Namespace}, &clusterv1.ManagedCluster{})
	if errors.IsNotFound(err) {
		err := r.client.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterDeployment.Namespace}})
		if err != nil {
			return err
		}

		r.recorder.Eventf("ManagedClusterNamespaceDeleted",
			"The managed cluster namespace %s is deleted", clusterDeployment.Namespace)
	}

	patch := client.MergeFrom(clusterDeployment.DeepCopy())
	clusterDeployment.Finalizers = []string{}
	if err := r.client.Patch(ctx, clusterDeployment, patch); err != nil {
		return err
	}

	r.recorder.Eventf("ClusterDeploymentFinalizerRemoved",
		"The clusterdeployment %s finalizer %s is removed", clusterDeployment.Name, constants.ImportFinalizer)
	return nil
}
