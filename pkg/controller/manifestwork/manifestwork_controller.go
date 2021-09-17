// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"fmt"
	"strings"

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

const manifestWorkFinalizer = "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup"

var log = logf.Log.WithName(controllerName)

// ReconcileManifestWork reconciles the ManagedClusters of the ManifestWorks object
type ReconcileManifestWork struct {
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
//func (r *ReconcileManifestWork) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
func (r *ReconcileManifestWork) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling the manifest works of the managed cluster")

	managedClusterName := request.Name
	ctx := context.TODO()

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

	// after managed clusrter joined, apply klusterlet manifest works from import secret
	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecretKey := types.NamespacedName{Namespace: managedClusterName, Name: importSecretName}
	importSecret := &corev1.Secret{}
	err = r.clientHolder.RuntimeClient.Get(ctx, importSecretKey, importSecret)
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
		err := helpers.RemoveManagedClusterFinalizer(r.clientHolder.RuntimeClient, r.recorder, cluster, manifestWorkFinalizer)
		if err != nil {
			return err
		}
		return nil
	}

	// there are manifest works in the managed cluster namespace, make sure the managed cluster has the manifest work finalizer
	modified := resourcemerge.BoolPtr(false)
	helpers.AddManagedClusterFinalizer(modified, cluster, manifestWorkFinalizer)
	if !*modified {
		return nil
	}

	if err := r.clientHolder.RuntimeClient.Update(ctx, cluster); err != nil {
		return err
	}

	r.recorder.Eventf("ManagedClusterMetaObjModified",
		"The managed cluster %s meta data is modified: manifestwork finalizer is added", cluster.Name)
	return nil
}

// deleteManifestWorks deletes manifest works when a managed cluster is deleting
// If the managed clsuter is unavailable, we will force delete all manifest works
// If the managed clsuter is available, we will
//   1. delete the manifest works that do not include klusterlet works
//   2. delete the klusterlet-crds manifest work after above manifest works are deleted.
// After the klusterlet-crds manifest work is deleted from the hub cluster, its included klusterlet crds will be deleted
// from managed cluser, then the kube system will delete the klusterlet cr from managed cluster. After the klusterlet is
// deleted from managed clsuter, the managed cluster will become unavailable, then this controller will force deleted the
// klusterlet manifest work. Because this process is slow, we speeded up the local cluster delation by delete its klusterlet
// explicitly.
func (r *ReconcileManifestWork) deleteManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork) error {
	if isClusterUnavailable(cluster) {
		// the managed cluster is is offline, force delete all manifest works
		return r.forceDeleteAllManifestWorks(ctx, works)
	}

	// delete works that do not include klusterlet works
	for _, manifestWork := range works {
		if manifestWork.GetName() == fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix) {
			continue
		}

		if !manifestWork.DeletionTimestamp.IsZero() {
			// the manifest work is deleting, do nothing
			continue
		}

		if err := r.deleteManifestWork(ctx, manifestWork.Namespace, manifestWork.Name); err != nil {
			return err
		}
	}

	names, err := r.filterKlusterletManifestWorks(ctx, cluster)
	if err != nil {
		return err
	}
	if len(names) != 0 {
		// still have other works, do nothing
		log.Info(fmt.Sprintf("In addition to klusterlet manifest works, there are also have %s", strings.Join(names, ",")))
		return nil
	}

	// only have klusterlet manifest works, delete klusterlet-crds manifest work firstly
	klusterletCRDsWorkName := fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix)
	klusterletCRDsWorkKey := types.NamespacedName{Namespace: cluster.Name, Name: klusterletCRDsWorkName}

	klusterletCRDsWork := &workv1.ManifestWork{}
	err = r.clientHolder.RuntimeClient.Get(ctx, klusterletCRDsWorkKey, klusterletCRDsWork)
	if errors.IsNotFound(err) {
		// the klusterlet-crds manifest work is deleted, to speed up the local cluster deletion, we clean up the
		// klusterlet explicitly
		return r.cleanUpSelfManagedCluster(ctx, cluster)
	}
	if err != nil {
		return err
	}

	if !klusterletCRDsWork.DeletionTimestamp.IsZero() {
		// the klusterlet-crds work is deleting, do nothing
		return nil
	}

	return r.deleteManifestWork(ctx, cluster.Name, klusterletCRDsWorkName)
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

	err = r.clientHolder.RuntimeClient.Get(ctx, manifestWorkKey, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// if the manifest work is not deleted, force remove its finalizers
	if len(manifestWork.Finalizers) != 0 {
		manifestWork.Finalizers = []string{}
		if err := r.clientHolder.RuntimeClient.Update(ctx, manifestWork); err != nil {
			return err
		}
	}

	r.recorder.Eventf("ManifestWorksForceDeleted",
		fmt.Sprintf("The manifest work %s/%s is force deleted", manifestWork.Namespace, manifestWork.Name))
	return nil
}

func (r *ReconcileManifestWork) filterKlusterletManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster) ([]string, error) {
	listOpts := &client.ListOptions{Namespace: cluster.Name}
	manifestWorks := &workv1.ManifestWorkList{}
	if err := r.clientHolder.RuntimeClient.List(ctx, manifestWorks, listOpts); err != nil {
		return nil, err
	}

	manifestWorkNames := []string{}
	for _, manifestWork := range manifestWorks.Items {
		if manifestWork.GetName() != fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix) &&
			manifestWork.GetName() != fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix) {
			manifestWorkNames = append(manifestWorkNames, manifestWork.GetName())
		}
	}

	return manifestWorkNames, nil
}

func (r *ReconcileManifestWork) deleteManifestWork(ctx context.Context, namespace, name string) error {
	manifestWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	err := r.clientHolder.RuntimeClient.Delete(ctx, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	r.recorder.Eventf("ManifestWorksDeleted", fmt.Sprintf("The manifest work %s/%s is deleted", namespace, name))
	return nil
}

func (r *ReconcileManifestWork) cleanUpSelfManagedCluster(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	if selfManaged, ok := cluster.Labels[constants.SelfManagedLabel]; !ok || !strings.EqualFold(selfManaged, "true") {
		// managed cluster is not self managed cluster, do nothing
		return nil
	}

	// the klusterlet-crds has already be deleted, delete the klusterlet mainifest work
	if err := r.forceDeleteManifestWork(ctx, cluster.Name, fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix)); err != nil {
		return err
	}

	klusterlets := r.clientHolder.OperatorClient.OperatorV1().Klusterlets()
	// ensure to delete klusterlet, if klusterlet is not deleted, it will stop the klusterlet-crds deletion
	klusterlet, err := klusterlets.Get(ctx, "klusterlet", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if len(klusterlet.Finalizers) == 0 {
		return nil
	}

	// force remove finalizers
	klusterlet = klusterlet.DeepCopy()
	klusterlet.Finalizers = []string{}
	if _, err := klusterlets.Update(ctx, klusterlet, metav1.UpdateOptions{}); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("delete the klusterlet from managed cluster %s", cluster.Name))
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
	manifests := []workv1.Manifest{}

	crdV1beta1Yaml := importSecret.Data[constants.ImportSecretCRDSV1beta1YamlKey]
	jsonData, err := yaml.YAMLToJSON(crdV1beta1Yaml)
	if err != nil {
		panic(err)
	}
	manifests = append(manifests, workv1.Manifest{RawExtension: runtime.RawExtension{Raw: jsonData}})

	if helpers.IsAPIExtensionV1Supported(managedCluster.Status.Version.Kubernetes) {
		crdV1Yaml := importSecret.Data[constants.ImportSecretCRDSV1YamlKey]
		jsonData, err := yaml.YAMLToJSON(crdV1Yaml)
		if err != nil {
			panic(err)
		}
		manifests = append(manifests, workv1.Manifest{RawExtension: runtime.RawExtension{Raw: jsonData}})
	}

	return &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, klusterletCRDsSuffix),
			Namespace: managedCluster.Name,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
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
		manifests = append(manifests, workv1.Manifest{RawExtension: runtime.RawExtension{Raw: jsonData}})
	}

	if helpers.IsAPIExtensionV1Supported(managedCluster.Status.Version.Kubernetes) {
		// the two versions of the crd are added to avoid the unexpected removal during the work-agent upgrade.
		// we will remove the v1beta1 in a future z-release. see: https://github.com/open-cluster-management/backlog/issues/13631
		crdV1Yaml := importSecret.Data[constants.ImportSecretCRDSV1YamlKey]
		jsonData, err := yaml.YAMLToJSON(crdV1Yaml)
		if err != nil {
			panic(err)
		}
		manifests = append(manifests, workv1.Manifest{RawExtension: runtime.RawExtension{Raw: jsonData}})
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
		},
	}
}
