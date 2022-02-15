// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"fmt"
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/openshift/library-go/pkg/operator/events"

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

var log = logf.Log.WithName(controllerName)

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

	if helpers.DetermineKlusterletMode(managedCluster) != constants.KlusterletDeployModeDefault {
		return reconcile.Result{}, nil
	}

	listOpts := &client.ListOptions{Namespace: managedClusterName}
	manifestWorks := &workv1.ManifestWorkList{}
	if err := r.clientHolder.RuntimeClient.List(ctx, manifestWorks, listOpts); err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.AssertManifestWorkFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder,
		managedCluster, len(manifestWorks.Items)); err != nil {
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

	if helpers.IsClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all manifest works
		return helpers.ForceDeleteAllManifestWorks(ctx, r.clientHolder.RuntimeClient, r.recorder, works)
	}

	// delete works that do not include klusterlet works and klusterlet addon works, the addon works will be removed by
	// klusterlet-addon-controller, we need to wait the klusterlet-addon-controller delete them.
	//
	// if there are any hypershift detached mode manifestworks we also wait for users to detach the hypershift managed
	// cluster first.
	ignoreKlusterletAndAddons := func(clusterName string, manifestWork workv1.ManifestWork) bool {
		return manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, klusterletSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, klusterletCRDsSuffix) ||
			strings.HasPrefix(manifestWork.GetName(), fmt.Sprintf("%s-klusterlet-addon", manifestWork.GetNamespace())) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, constants.HypershiftDetachedKlusterletManifestworkSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, constants.HypershiftDetachedManagedKubeconfigManifestworkSuffix)
	}
	err := helpers.DeleteManifestWorkWithSelector(ctx, r.clientHolder.RuntimeClient, r.recorder, cluster, works, ignoreKlusterletAndAddons)
	if err != nil {
		return err
	}

	// check wether there are only klusterlet manifestworks
	ignoreKlusterlet := func(clusterName string, manifestWork workv1.ManifestWork) bool {
		return manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, klusterletSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, klusterletCRDsSuffix)
	}
	noPending, err := helpers.NoPendingManifestWorks(ctx, r.clientHolder.RuntimeClient, log, cluster.GetName(), ignoreKlusterlet)
	if err != nil {
		return err
	}
	if !noPending {
		// still have other works, do nothing
		return nil
	}

	// only have klusterlet manifest works, delete klusterlet manifest works
	klusterletName := fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix)
	klusterletWork := &workv1.ManifestWork{}
	err = r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Namespace: cluster.Name, Name: klusterletName}, klusterletWork)
	if errors.IsNotFound(err) {
		// the klusterlet work could be deleted, ensure the klusterlet crds work is deleted
		return helpers.ForceDeleteManifestWork(ctx, r.clientHolder.RuntimeClient, r.recorder,
			cluster.Name, fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix))
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

	return helpers.DeleteManifestWork(ctx, r.clientHolder.RuntimeClient, r.recorder, klusterletWork.Namespace, klusterletWork.Name)
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
