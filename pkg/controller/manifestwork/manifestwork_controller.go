// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	"github.com/ghodss/yaml"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	manifestWorkFinalizer = "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup"

	// postponeDeletionAnnotation is used to delete the manifest work with this annotation until 10 min after the cluster is deleted.
	postponeDeletionAnnotation = "open-cluster-management/postpone-delete"
)

var log = logf.Log.WithName(controllerName)

// manifestWorkPostponeDeleteTime is the postponed time to delete manifest work with postpone-delete annotation
var manifestWorkPostponeDeleteTime = 10 * time.Minute

// ReconcileManifestWork reconciles the ManagedClusters of the ManifestWorks object
type ReconcileManifestWork struct {
	commonProcessor

	// the key of the map represents klusterlet deploy mode
	workers map[string]manifestWorker
}

// blank assignment to verify that ReconcileManifestWork implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileManifestWork{}

type commonProcessor struct {
	clientHolder *helpers.ClientHolder
	scheme       *runtime.Scheme
	recorder     events.Recorder
}

// Reconcile the ManagedClusters of the ManifestWorks.
// - When the manifester works are created in one managed cluster namespace, we will add a manifest work
//   finalizer to the managed cluster
// - When a managed cluster is deleting, we delete the manifest works and remove the manifest work
//   finalizer from the managed cluster
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileManifestWork) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling the manifest works of the managed cluster")
	managedClusterName := request.Name

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	mode, err := helpers.DetermineKlusterletMode(managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}
	worker, ok := r.workers[mode]
	if !ok {
		reqLogger.Error(nil, "klusterlet deploy mode not supportted", "mode", mode)
		return reconcile.Result{}, nil
	}

	listOpts := &client.ListOptions{Namespace: managedClusterName}
	manifestWorks := &workv1.ManifestWorkList{}
	if err := r.clientHolder.RuntimeClient.List(ctx, manifestWorks, listOpts); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.assertManifestWorkFinalizer(ctx, managedCluster, len(manifestWorks.Items)); err != nil {
		return reconcile.Result{}, err
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		// the managed cluster is deleting, delete its manifestworks
		return reconcile.Result{}, worker.deleteManifestWorks(ctx, managedCluster, manifestWorks.Items)
	}

	// For Default mode, managed cluster does not join, do nothing
	// But for Detached mode, try to create klusterlet CR by creating manifestwork in the
	// management cluster to trigger the joining process.
	if mode == constants.KlusterletDeployModeDefault && !meta.IsStatusConditionTrue(managedCluster.Status.Conditions, clusterv1.ManagedClusterConditionJoined) {
		return reconcile.Result{}, nil
	}

	// For default mode, after managed cluster joined, apply klusterlet manifest works from import secret to the managed cluster namespace
	// For detached mode, apply klusterlet manifest works from import secret to the management cluster namespace
	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.clientHolder.KubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, importSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) && mode == constants.KlusterletDeployModeHypershiftDetached {
			// For Detached mode, import secret does not exist, do nothing
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if err := worker.validateImportSecret(importSecret); err != nil {
		return reconcile.Result{}, err
	}

	objs, err := worker.generateManifestWorks(managedCluster, importSecret)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ApplyResources(
		r.clientHolder,
		r.recorder,
		r.scheme,
		managedCluster,
		objs...,
	); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileManifestWork) assertManifestWorkFinalizer(ctx context.Context, cluster *clusterv1.ManagedCluster, works int) error {
	if works == 0 {
		// there are no manifest works, remove the manifest work finalizer
		err := helpers.RemoveManagedClusterFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder, cluster, manifestWorkFinalizer)
		if err != nil {
			return err
		}
		return nil
	}

	// there are manifest works in the managed cluster namespace, make sure the managed cluster has the manifest work finalizer
	patch := client.MergeFrom(cluster.DeepCopy())
	modified := resourcemerge.BoolPtr(false)
	helpers.AddManagedClusterFinalizer(modified, cluster, manifestWorkFinalizer)
	if !*modified {
		return nil
	}

	if err := r.clientHolder.RuntimeClient.Patch(ctx, cluster, patch); err != nil {
		return err
	}

	r.recorder.Eventf("ManagedClusterMetaObjModified",
		"The managed cluster %s meta data is modified: manifestwork finalizer is added", cluster.Name)
	return nil
}

func (r *commonProcessor) forceDeleteAllManifestWorks(ctx context.Context, manifestWorks []workv1.ManifestWork) error {
	for _, item := range manifestWorks {
		if err := r.forceDeleteManifestWork(ctx, item.Namespace, item.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *commonProcessor) forceDeleteManifestWork(ctx context.Context, namespace, name string) error {
	manifestWorkKey := types.NamespacedName{Namespace: namespace, Name: name}
	manifestWork := &workv1.ManifestWork{}
	err := r.clientHolder.RuntimeClient.Get(ctx, manifestWorkKey, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := r.clientHolder.RuntimeClient.Delete(ctx, manifestWork); err != nil {
		return err
	}

	// reload the manifest work
	err = r.clientHolder.RuntimeClient.Get(ctx, manifestWorkKey, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// if the manifest work is not deleted, force remove its finalizers
	if len(manifestWork.Finalizers) != 0 {
		patch := client.MergeFrom(manifestWork.DeepCopy())
		manifestWork.Finalizers = []string{}
		if err := r.clientHolder.RuntimeClient.Patch(ctx, manifestWork, patch); err != nil {
			return err
		}
	}

	r.recorder.Eventf("ManifestWorksForceDeleted",
		fmt.Sprintf("The manifest work %s/%s is force deleted", manifestWork.Namespace, manifestWork.Name))
	return nil
}

func (r *commonProcessor) noPendingManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster,
	ignoredSelector func(clusterName string, manifestWork workv1.ManifestWork) bool) (bool, error) {
	listOpts := &client.ListOptions{Namespace: cluster.Name}
	manifestWorks := &workv1.ManifestWorkList{}
	if err := r.clientHolder.RuntimeClient.List(ctx, manifestWorks, listOpts); err != nil {
		return false, err
	}

	manifestWorkNames := []string{}
	ignoredManifestWorkNames := []string{}
	for _, manifestWork := range manifestWorks.Items {
		if ignoredSelector(cluster.GetName(), manifestWork) {
			ignoredManifestWorkNames = append(ignoredManifestWorkNames, manifestWork.GetName())
		} else {
			manifestWorkNames = append(manifestWorkNames, manifestWork.GetName())
		}
	}

	if len(manifestWorkNames) != 0 {
		log.Info(fmt.Sprintf("In addition to ignored manifest works %s, there are also have %s",
			strings.Join(ignoredManifestWorkNames, ","), strings.Join(manifestWorkNames, ",")))
		return false, nil
	}

	return true, nil
}

func (r *commonProcessor) deleteManifestWork(ctx context.Context, namespace, name string) error {
	manifestWork := &workv1.ManifestWork{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if !manifestWork.DeletionTimestamp.IsZero() {
		// the manifest work is deleting, do nothing
		return nil
	}

	if err := r.clientHolder.RuntimeClient.Delete(ctx, manifestWork); err != nil {
		return err
	}

	r.recorder.Eventf("ManifestWorksDeleted", fmt.Sprintf("The manifest work %s/%s is deleted", namespace, name))
	return nil
}

func isClusterUnavailable(cluster *clusterv1.ManagedCluster) bool {
	if meta.IsStatusConditionFalse(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
		return true
	}
	if meta.IsStatusConditionPresentAndEqual(
		cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
		return true
	}

	return false
}

func createKlusterletCRDsManifestWork(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret) *workv1.ManifestWork {
	crdsKey := constants.ImportSecretCRDSV1YamlKey
	if !helpers.IsAPIExtensionV1Supported(managedCluster.Status.Version.Kubernetes) {
		log.Info("crd v1 is not supported, add v1beta1")
		crdsKey = constants.ImportSecretCRDSV1beta1YamlKey
	}

	crdYaml := importSecret.Data[crdsKey]
	jsonData, err := yaml.YAMLToJSON(crdYaml)
	if err != nil {
		panic(err)
	}

	return &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, klusterletCRDsSuffix),
			Namespace: managedCluster.Name,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: jsonData}},
				},
			},
		},
	}
}

func createKlusterletManifestWork(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret,
	manifestWorkNamespace string, deletePolicy workv1.DeletePropagationPolicyType) *workv1.ManifestWork {
	manifests := []workv1.Manifest{}
	importYaml := importSecret.Data[constants.ImportSecretImportYamlKey]
	for _, yamlData := range helpers.SplitYamls(importYaml) {
		jsonData, err := yaml.YAMLToJSON(yamlData)
		if err != nil {
			panic(err)
		}
		manifests = append(manifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: jsonData},
		})
	}

	mw := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, klusterletSuffix),
			Namespace: manifestWorkNamespace,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: deletePolicy,
			},
		},
	}

	return mw
}
