// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
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
	clientHolder *helpers.ClientHolder
	scheme       *runtime.Scheme
	recorder     events.Recorder
}

// blank assignment to verify that ReconcileManifestWork implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileManifestWork{}

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
		return reconcile.Result{}, r.deleteManifestWorks(ctx, managedCluster, manifestWorks.Items)
	}

	if !meta.IsStatusConditionTrue(managedCluster.Status.Conditions, clusterv1.ManagedClusterConditionJoined) {
		// managed cluster does not join, do nothing
		return reconcile.Result{}, nil
	}

	// after managed cluster joined, apply klusterlet manifest works from import secret
	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.clientHolder.KubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, importSecretName, metav1.GetOptions{})
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ValidateImportSecret(importSecret); err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ApplyResources(
		r.clientHolder,
		r.recorder,
		r.scheme,
		managedCluster,
		createKlusterletCRDsManifestWork(managedCluster, importSecret),
		createKlusterletManifestWork(managedCluster, importSecret),
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

// deleteManifestWorks deletes manifest works when a managed cluster is deleting
// If the managed cluster is unavailable, we will force delete all manifest works
// If the managed cluster is available, we will
//   1. delete the manifest work with the postpone-delete annotation until 10 min after the cluster is deleted.
//   2. delete the manifest works that do not include klusterlet works and klusterlet addon works
//   3. delete the klusterlet manifest work, the delete option of the klusterlet manifest work
//      is orphan, so we can delete it safely
//   4. after the klusterlet manifest work is deleted, we delete the klusterlet-crds manifest work,
//      after the klusterlet-crds manifest work is deleted from the hub cluster, its klusterlet
//      crds will be deleted from the managed cluster, then the kube system will delete the klusterlet
//      cr from the managed cluster, once the klusterlet cr is deleted, the klusterlet operator will
//      clean up the klusterlet on the managed cluster
func (r *ReconcileManifestWork) deleteManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork) error {
	if len(works) == 0 {
		return nil
	}

	if isClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all manifest works
		return r.forceDeleteAllManifestWorks(ctx, works)
	}

	// delete works that do not include klusterlet works and klusterlet addon works, the addon works will be removed by
	// klusterlet-addon-controller, we need to wait the klusterlet-addon-controller delete them
	for _, manifestWork := range works {
		if manifestWork.GetName() == fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix) ||
			strings.HasPrefix(manifestWork.GetName(), fmt.Sprintf("%s-klusterlet-addon", manifestWork.GetNamespace())) {
			continue
		}

		annotations := manifestWork.GetAnnotations()
		if _, ok := annotations[postponeDeletionAnnotation]; ok {
			if time.Since(cluster.DeletionTimestamp.Time) < manifestWorkPostponeDeleteTime {
				continue
			}
		}
		if err := r.deleteManifestWork(ctx, manifestWork.Namespace, manifestWork.Name); err != nil {
			return err
		}
	}

	onlyHas, err := r.onlyHasKlusterletManifestWorks(ctx, cluster)
	if err != nil {
		return err
	}
	if !onlyHas {
		// still have other works, do nothing
		return nil
	}

	// only have klusterlet manifest works, delete klusterlet manifest works
	klusterletName := fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix)
	klusterletWork := &workv1.ManifestWork{}
	err = r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Namespace: cluster.Name, Name: klusterletName}, klusterletWork)
	if errors.IsNotFound(err) {
		// the klusterlet work could be deleted, ensure the klusterlet crds work is deleted
		return r.forceDeleteManifestWork(ctx, cluster.Name, fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix))
	}
	if err != nil {
		return err
	}

	// if the manifest work is not applied, we do nothing to avoid to delete the cluster prematurely
	// Note: there is a corner case, the registration-agent is availabel, but the work-agent is unavailablel,
	// this will cause that the klusterlet work cannot be deleted, we need user to handle this manually
	if !meta.IsStatusConditionTrue(klusterletWork.Status.Conditions, workv1.WorkApplied) {
		log.Info(fmt.Sprintf("delete the manifest work %s until it is applied ...", klusterletWork.Name))
		return nil
	}

	return r.deleteManifestWork(ctx, klusterletWork.Namespace, klusterletWork.Name)
}

func (r *ReconcileManifestWork) forceDeleteAllManifestWorks(ctx context.Context, manifestWorks []workv1.ManifestWork) error {
	for _, item := range manifestWorks {
		if err := r.forceDeleteManifestWork(ctx, item.Namespace, item.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileManifestWork) forceDeleteManifestWork(ctx context.Context, namespace, name string) error {
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

func (r *ReconcileManifestWork) onlyHasKlusterletManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster) (bool, error) {
	listOpts := &client.ListOptions{Namespace: cluster.Name}
	manifestWorks := &workv1.ManifestWorkList{}
	if err := r.clientHolder.RuntimeClient.List(ctx, manifestWorks, listOpts); err != nil {
		return false, err
	}

	manifestWorkNames := []string{}
	for _, manifestWork := range manifestWorks.Items {
		if manifestWork.GetName() != fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix) &&
			manifestWork.GetName() != fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix) {
			manifestWorkNames = append(manifestWorkNames, manifestWork.GetName())
		}
	}

	if len(manifestWorkNames) != 0 {
		log.Info(fmt.Sprintf("In addition to klusterlet manifest works, there are also have %s", strings.Join(manifestWorkNames, ",")))
		return false, nil
	}

	return true, nil
}

func (r *ReconcileManifestWork) deleteManifestWork(ctx context.Context, namespace, name string) error {
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

func createKlusterletManifestWork(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret) *workv1.ManifestWork {
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

	return &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, klusterletSuffix),
			Namespace: managedCluster.Name,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			},
		},
	}
}
