// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hosted

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	"github.com/openshift/library-go/pkg/operator/events"
	operatorhelpers "github.com/openshift/library-go/pkg/operator/v1helpers"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/ghodss/yaml"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//go:embed manifests
var manifestFiles embed.FS

var klusterletHostedExternalKubeconfig = "manifests/external_managed_secret.yaml"

var log = logf.Log.WithName(controllerName)

// ReconcileHosted reconciles the Hosted mode ManagedClusters of the ManifestWorks object
type ReconcileHosted struct {
	clientHolder   *helpers.ClientHolder
	informerHolder *source.InformerHolder
	scheme         *runtime.Scheme
	recorder       events.Recorder
}

// blank assignment to verify that ReconcileHosted implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileHosted{}

// Reconcile the hosted mode ManagedClusters of the ManifestWorks.
//   - When a hosted mode ManagedCluster created, we will create a klusterlet manifestwork to trigger the
//     cluster importing process
//   - When an auto import secret created for the hosted mode managed cluster, we create a managed
//     kubeconfig manifestwork to create an external managed kubeconfig secret on the hosting cluster
//   - When the manifester works are created in one managed cluster namespace, we will add a manifest work
//     finalizer to the managed cluster
//   - When a managed cluster is deleting, we delete the manifest works and remove the manifest work
//     finalizer from the managed cluster
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileHosted) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
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

	if helpers.DetermineKlusterletMode(managedCluster) != constants.KlusterletDeployModeHosted {
		return reconcile.Result{}, nil
	}

	// TODO(zhujian7): check if annotation hosting cluster is provided, check if the hosting cluster
	// is a managed cluster of hub, and check its status.

	reqLogger.Info("Reconciling the manifest works of the hosted mode managed cluster")

	hostedWorksSelector := labels.SelectorFromSet(map[string]string{constants.HostedClusterLabel: managedClusterName})

	managementCluster, err := helpers.GetHostingCluster(managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		// use work client to list all works only when a managed cluster is deleting
		hostedWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(managementCluster).List(ctx, metav1.ListOptions{
			LabelSelector: hostedWorksSelector.String(),
		})
		if err != nil {
			return reconcile.Result{}, err
		}

		manifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(managedClusterName).List(ctx, metav1.ListOptions{})
		if err != nil {
			return reconcile.Result{}, err
		}

		// if there no works, remove the manifest work finalizer from the managed cluster
		if err := helpers.AssertManifestWorkFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder,
			managedCluster, len(manifestWorks.Items)+len(hostedWorks.Items)); err != nil {
			return reconcile.Result{}, err
		}

		if (len(manifestWorks.Items) + len(hostedWorks.Items)) == 0 {
			return reconcile.Result{}, nil
		}

		// the managed cluster is deleting, delete its addons and manifestworks
		// Note: we only informer the hosted works, so we need to requeue here
		return reconcile.Result{RequeueAfter: 5 * time.Second},
			r.deleteAddonsAndWorks(ctx, managedCluster, manifestWorks.Items, hostedWorks.Items)
	}

	hostedWorks, err := r.informerHolder.HostedWorkLister.ManifestWorks(managementCluster).List(hostedWorksSelector)
	if err != nil {
		return reconcile.Result{}, err
	}

	// after the hosted works are created, make sure the managed cluster has manifest work finalizer
	if err := helpers.AssertManifestWorkFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder,
		managedCluster, len(hostedWorks)); err != nil {
		return reconcile.Result{}, err
	}

	// apply klusterlet manifest works klustelet to the management namespace from import secret to trigger the joining process.
	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.informerHolder.ImportSecretLister.Secrets(managedClusterName).Get(importSecretName)
	if errors.IsNotFound(err) {
		// wait for the import secret to exist, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ValidateHostedImportSecret(importSecret); err != nil {
		return reconcile.Result{}, err
	}

	manifestWork := createHostedManifestWork(managedCluster.Name, importSecret, managementCluster)
	err = helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, manifestWork)
	if err != nil {
		return reconcile.Result{}, err
	}

	autoImportSecret, err := r.informerHolder.AutoImportSecretLister.Secrets(managedClusterName).Get(constants.AutoImportSecretName)
	if errors.IsNotFound(err) {
		// the auto import secret has not be created or has been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	manifestWork, err = createManagedKubeconfigManifestWork(managedCluster.Name, autoImportSecret, managementCluster)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, manifestWork)
	if err != nil {
		errStatus := helpers.UpdateManagedClusterStatus(r.clientHolder.RuntimeClient, r.recorder, managedClusterName, metav1.Condition{
			Type:    "ExternalManagedKubeconfigCreatedSucceeded",
			Status:  metav1.ConditionFalse,
			Message: fmt.Sprintf("Unable to create external managed kubeconfig for %s: %s", managedCluster.Name, err.Error()),
			Reason:  "ExternalManagedKubeconfigNotCreated",
		})
		if errStatus != nil {
			return reconcile.Result{}, errStatus
		}

		errRetry := helpers.UpdateAutoImportRetryTimes(ctx, r.clientHolder.KubeClient, r.recorder, autoImportSecret.DeepCopy())
		return reconcile.Result{}, utilerrors.NewAggregate([]error{err, errRetry})
	}

	// TODO: should check the manifest work status to determine this status
	err = helpers.UpdateManagedClusterStatus(r.clientHolder.RuntimeClient, r.recorder, managedClusterName, metav1.Condition{
		Type:    "ExternalManagedKubeconfigCreatedSucceeded",
		Status:  metav1.ConditionTrue,
		Message: "Created succeeded",
		Reason:  "ExternalManagedKubeconfigCreated",
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("External managed kubeconfig is created, try to delete its auto import secret %s/%s",
		autoImportSecret.Namespace, autoImportSecret.Name))
	return reconcile.Result{}, helpers.DeleteAutoImportSecret(ctx, r.clientHolder.KubeClient, autoImportSecret, r.recorder)
}

func klusterletNamespace(managedCluster string) string {
	return fmt.Sprintf("klusterlet-%s", managedCluster)
}

func (r *ReconcileHosted) deleteAddonsAndWorks(
	ctx context.Context, cluster *clusterv1.ManagedCluster, works, hostedWorks []workv1.ManifestWork) error {
	errs := append(
		[]error{},
		helpers.DeleteManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, r.recorder, cluster),
		r.deleteManifestWorks(ctx, cluster, works, hostedWorks),
	)
	return operatorhelpers.NewMultiLineAggregate(errs)
}

// deleteManifestWorks deletes manifest works when a managed cluster is deleting
// If the managed cluster is unavailable, we will force delete all manifest works
// If the managed cluster is available, we will
//  1. delete the manifest work with the postpone-delete annotation until 10 min
//     after the cluster is deleted.
//  2. delete the manifest works that do not include klusterlet addon works
//  3. delete the klusterlet and managed kubeconfig manifest works
func (r *ReconcileHosted) deleteManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster,
	works, hostedWorks []workv1.ManifestWork) error {
	if helpers.IsClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all manifest works
		return helpers.ForceDeleteAllManifestWorks(ctx, r.clientHolder.WorkClient, r.recorder, append(works, hostedWorks...))
	}

	// delete works that do not include klusterlet works and klusterlet addon works, the addon works were removed
	// above, we need to wait them to be deleted.
	ignoreAddons := func(clusterName string, manifestWork workv1.ManifestWork) bool {
		manifestWorkName := manifestWork.GetName()
		switch {
		case strings.HasPrefix(manifestWorkName, fmt.Sprintf("%s-klusterlet-addon", manifestWork.GetNamespace())):
		case strings.HasPrefix(manifestWorkName, "addon-") && strings.HasSuffix(manifestWork.GetName(), "-deploy"):
		case strings.HasPrefix(manifestWorkName, "addon-") && strings.HasSuffix(manifestWork.GetName(), "-pre-delete"):
		default:
			return false
		}
		return true
	}
	err := helpers.DeleteManifestWorkWithSelector(ctx, r.clientHolder.WorkClient, r.recorder, cluster, works, ignoreAddons)
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

	ignoreNothing := func(_ string, _ workv1.ManifestWork) bool { return false }
	noPending, err := helpers.NoPendingManifestWorks(ctx, log, cluster.GetName(), works, ignoreNothing)
	if err != nil {
		return err
	}
	if !noPending {
		// still have other works, do nothing
		return nil
	}

	// delete hosted manifestWorks from the hosting cluster namespace
	for _, hostedWork := range hostedWorks {
		if err := helpers.DeleteManifestWork(ctx, r.clientHolder.WorkClient, r.recorder,
			hostedWork.Namespace, hostedWork.Name); err != nil {
			return err
		}
	}

	return nil
}

// createHostedManifestWork creates a manifestwork from import secret for hosted mode cluster
func createHostedManifestWork(managedClusterName string,
	importSecret *corev1.Secret, manifestWorkNamespace string) *workv1.ManifestWork {
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

	// For hosted mode, the klusterletManifestWork only contains a klusterlet CR
	// and a bootstrap secret, delete it in foreground.
	return &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostedKlusterletManifestWorkName(managedClusterName),
			Namespace: manifestWorkNamespace,
			Labels: map[string]string{
				constants.HostedClusterLabel: managedClusterName,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: workv1.DeletePropagationPolicyTypeForeground,
			},
		},
	}
}

func createManagedKubeconfigManifestWork(managedClusterName string, importSecret *corev1.Secret,
	manifestWorkNamespace string) (*workv1.ManifestWork, error) {
	kubeconfig := importSecret.Data["kubeconfig"]
	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("import secret invalid, the field kubeconfig must exist in the secret for hosted mode")
	}

	config := struct {
		KlusterletNamespace       string
		ExternalManagedKubeconfig string
	}{
		KlusterletNamespace:       klusterletNamespace(managedClusterName),
		ExternalManagedKubeconfig: base64.StdEncoding.EncodeToString(kubeconfig),
	}

	template, err := manifestFiles.ReadFile(klusterletHostedExternalKubeconfig)
	if err != nil {
		return nil, err
	}
	externalKubeYAML := helpers.MustCreateAssetFromTemplate(klusterletHostedExternalKubeconfig, template, config)
	externalKubeJSON, err := yaml.YAMLToJSON(externalKubeYAML)
	if err != nil {
		return nil, err
	}

	manifests := []workv1.Manifest{
		{
			RawExtension: runtime.RawExtension{Raw: externalKubeJSON},
		},
	}

	mw := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostedManagedKubeconfigManifestWorkName(managedClusterName),
			Namespace: manifestWorkNamespace,
			Labels: map[string]string{
				constants.HostedClusterLabel: managedClusterName,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				// For hosted mode, we will not delete the "external-managed-kubeconfig" since
				// klusterlet operator will use this secret to clean resources on the managed
				// cluster, after the cleanup finished, the klusterlet operator will delete the
				// secret.
				PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			},
		},
	}

	return mw, nil
}

func hostedKlusterletManifestWorkName(managedClusterName string) string {
	return fmt.Sprintf("%s-%s", managedClusterName, constants.HostedKlusterletManifestworkSuffix)
}

func hostedManagedKubeconfigManifestWorkName(managedClusterName string) string {
	return fmt.Sprintf("%s-%s", managedClusterName, constants.HostedManagedKubeconfigManifestworkSuffix)
}
