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
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/ghodss/yaml"
	"github.com/openshift/library-go/pkg/operator/events"
	operatorhelpers "github.com/openshift/library-go/pkg/operator/v1helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(controllerName)

// ReconcileManifestWork reconciles the ManagedClusters of the ManifestWorks object
type ReconcileManifestWork struct {
	clientHolder   *helpers.ClientHolder
	informerHolder *source.InformerHolder
	scheme         *runtime.Scheme
	recorder       events.Recorder
}

// blank assignment to verify that ReconcileManifestWork implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileManifestWork{}

// Reconcile the ManagedClusters of the ManifestWorks.
//   - When the manifester works are created in one managed cluster namespace, we will add a manifest work
//     finalizer to the managed cluster
//   - When a managed cluster is deleting, we delete the manifest works and remove the manifest work
//     finalizer from the managed cluster
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileManifestWork) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

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

	reqLogger.Info("Reconciling the manifest works of the managed cluster")

	if !managedCluster.DeletionTimestamp.IsZero() {
		// use work client to list all works only when a managed cluster is deleting
		manifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(managedClusterName).List(ctx, metav1.ListOptions{})
		if err != nil {
			return reconcile.Result{}, err
		}

		// if there no works, remove the manifest work finalizer from the managed cluster
		if err := helpers.AssertManifestWorkFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder,
			managedCluster, len(manifestWorks.Items)); err != nil {
			return reconcile.Result{}, err
		}

		if len(manifestWorks.Items) == 0 {
			return reconcile.Result{}, nil
		}

		// the managed cluster is deleting, delete its addons and manifestworks
		// Note: we only informer the klusterlet works, so we need to requeue here
		return reconcile.Result{RequeueAfter: 5 * time.Second}, r.deleteAddonsAndWorks(ctx, managedCluster, manifestWorks.Items)
	}

	workSelector := labels.SelectorFromSet(map[string]string{constants.KlusterletWorksLabel: "true"})
	manifestWorks, err := r.informerHolder.KlusterletWorkLister.ManifestWorks(managedClusterName).List(workSelector)
	if err != nil {
		return reconcile.Result{}, err
	}

	// after the klusterlet works are created, make sure the managed cluster has manifest work finalizer
	if err := helpers.AssertManifestWorkFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder,
		managedCluster, len(manifestWorks)); err != nil {
		return reconcile.Result{}, err
	}

	// apply klusterlet manifest works from import secret
	// Note: create the klusterlet manifest works before importing cluster to avoid the klusterlet applied manifest
	// works are deleted from managed cluster if the restored hub has same host with the backup hub in the
	// backup-restore case.
	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.informerHolder.ImportSecretLister.Secrets(managedClusterName).Get(importSecretName)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ValidateImportSecret(importSecret); err != nil {
		return reconcile.Result{}, err
	}

	_, err = helpers.ApplyResources(
		r.clientHolder,
		r.recorder,
		r.scheme,
		managedCluster,
		createKlusterletCRDsManifestWork(managedCluster, importSecret),
		createKlusterletManifestWork(managedCluster, importSecret),
	)
	return reconcile.Result{}, err
}

func (r *ReconcileManifestWork) deleteAddonsAndWorks(ctx context.Context,
	cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork) error {
	errs := append(
		[]error{},
		helpers.DeleteManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, r.recorder, cluster),
		r.deleteManifestWorks(ctx, cluster, works),
	)
	return operatorhelpers.NewMultiLineAggregate(errs)
}

// deleteManifestWorks deletes manifest works when a managed cluster is deleting
// If the managed cluster is unavailable, we will force delete all manifest works
// If the managed cluster is available, we will
//  1. delete the manifest work with the postpone-delete annotation until 10 min after the cluster is deleted.
//  2. delete the manifest works that do not include klusterlet works and klusterlet addon works
//  3. delete the klusterlet manifest work, the delete option of the klusterlet manifest work
//     is orphan, so we can delete it safely
//  4. after the klusterlet manifest work is deleted, we delete the klusterlet-crds manifest work,
//     after the klusterlet-crds manifest work is deleted from the hub cluster, its klusterlet
//     crds will be deleted from the managed cluster, then the kube system will delete the klusterlet
//     cr from the managed cluster, once the klusterlet cr is deleted, the klusterlet operator will
//     clean up the klusterlet on the managed cluster
func (r *ReconcileManifestWork) deleteManifestWorks(
	ctx context.Context,
	cluster *clusterv1.ManagedCluster,
	works []workv1.ManifestWork) error {

	if helpers.IsClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all manifest works
		return helpers.ForceDeleteAllManifestWorks(ctx, r.clientHolder.WorkClient, r.recorder, works)
	}

	// delete works that do not include klusterlet works and klusterlet addon works, the addon works were removed
	// above, we need to wait them to be deleted.
	//
	// if there are any Hosted mode manifestworks we also wait for users to detach the managed cluster first.
	ignoreKlusterletAndAddons := func(clusterName string, manifestWork workv1.ManifestWork) bool {
		// if the addon is implemented by addon-framekwork, the format of manifestwork name is
		// the old format : addon-<addon name>-deploy,
		// the new format : addon-<addon name>-deploy-<index>
		// and the manifestwork has the addon label.
		// so cannot only use the name to filter.
		workLabels := manifestWork.GetLabels()
		if _, ok := workLabels[addonapiv1alpha1.AddonLabelKey]; ok {
			return true
		}

		manifestWorkName := manifestWork.GetName()
		switch {
		case manifestWorkName == fmt.Sprintf("%s-%s", clusterName, constants.KlusterletSuffix):
		case manifestWorkName == fmt.Sprintf("%s-%s", clusterName, constants.KlusterletCRDsSuffix):
		case manifestWorkName == fmt.Sprintf("%s-%s", clusterName, constants.HostedKlusterletManifestworkSuffix):
		case manifestWorkName == fmt.Sprintf("%s-%s", clusterName, constants.HostedManagedKubeconfigManifestworkSuffix):
		case strings.HasPrefix(manifestWorkName, fmt.Sprintf("%s-klusterlet-addon", manifestWork.GetNamespace())):
		case strings.HasPrefix(manifestWorkName, "addon-") && strings.HasSuffix(manifestWork.GetName(), "-deploy"):
		case strings.HasPrefix(manifestWorkName, "addon-") && strings.HasSuffix(manifestWork.GetName(), "-pre-delete"):
		default:
			return false
		}
		return true
	}
	err := helpers.DeleteManifestWorkWithSelector(ctx, r.clientHolder.WorkClient, r.recorder,
		cluster, works, ignoreKlusterletAndAddons)
	if err != nil {
		return err
	}

	noAddons, err := helpers.NoManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, cluster.GetName())
	if err != nil {
		return err
	}
	if !noAddons {
		// wait for addons deletion
		return nil
	}

	// check whether there are only klusterlet manifestworks
	ignoreKlusterlet := func(clusterName string, manifestWork workv1.ManifestWork) bool {
		return manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, constants.KlusterletSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, constants.KlusterletCRDsSuffix)
	}
	noPendingManifestWorks, err := helpers.NoPendingManifestWorks(ctx, log, cluster.GetName(), works, ignoreKlusterlet)
	if err != nil {
		return err
	}
	if !noPendingManifestWorks {
		// still have other works, do nothing
		return nil
	}

	// only have klusterlet manifest works, delete klusterlet manifest works
	klusterletName := fmt.Sprintf("%s-%s", cluster.Name, constants.KlusterletSuffix)
	klusterletWork, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(cluster.Name).Get(ctx, klusterletName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// the klusterlet work could be deleted, ensure the klusterlet crds work is deleted
		return helpers.ForceDeleteManifestWork(ctx, r.clientHolder.WorkClient, r.recorder,
			cluster.Name, fmt.Sprintf("%s-%s", cluster.Name, constants.KlusterletCRDsSuffix))
	}
	if err != nil {
		return err
	}

	// Note: we don't wait for the manifest work is applied, so there is a corner case: when the cluster is availabel
	// but the klusterlet works is not applied, in this time, user delete the cluster, this will cause that the
	// klusterlet cannot be deleted from the mangaed cluser, we need user to handle this manually

	return helpers.DeleteManifestWork(ctx, r.clientHolder.WorkClient, r.recorder, klusterletWork.Namespace, klusterletWork.Name)
}

func createKlusterletCRDsManifestWork(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret) *workv1.ManifestWork {
	crdsKey := constants.ImportSecretCRDSV1YamlKey
	if managedCluster.Status.Version.Kubernetes != "" &&
		!helpers.IsAPIExtensionV1Supported(managedCluster.Status.Version.Kubernetes) {
		log.Info("crd v1 is not supported, put v1beta1 to manifest work")
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
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.KlusterletCRDsSuffix),
			Namespace: managedCluster.Name,
			Labels: map[string]string{
				constants.KlusterletWorksLabel: "true",
			},
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
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.KlusterletSuffix),
			Namespace: managedCluster.Name,
			Labels: map[string]string{
				constants.KlusterletWorksLabel: "true",
			},
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
