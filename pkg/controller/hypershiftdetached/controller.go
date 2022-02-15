// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hypershiftdetached

import (
	"context"
	"embed"
	"encoding/base64"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//go:embed manifests
var manifestFiles embed.FS

var klusterletDetachedExternalKubeconfig = "manifests/external_managed_secret.yaml"

var log = logf.Log.WithName(controllerName)

// HypershiftDetachedManifestworkSuffix is a suffix of the hypershift detached mode manifestwork name.
const HypershiftDetachedManifestworkSuffix = "klusterlet"

// ReconcileHypershift reconciles the ManagedClusters of the ManifestWorks object
type ReconcileHypershift struct {
	clientHolder *helpers.ClientHolder
	scheme       *runtime.Scheme
	client       client.Client
	kubeClient   kubernetes.Interface
	recorder     events.Recorder
}

// blank assignment to verify that ReconcileManifestWork implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileHypershift{}

// Reconcile the hypershift detached mode ManagedClusters of the ManifestWorks.
// - When a hypershift detached mode ManagedCluster created, we will create a klusterlet manifestwork to
//   trigger the cluster importing process
// - When an auto import secret created for the hypershift detached mode managed cluster, we update the
//   klusterlet to create an external managed kubeconfig secret on the management cluster
// - When the manifester works are created in one managed cluster namespace, we will add a manifest work
//   finalizer to the managed cluster
// - When a managed cluster is deleting, we delete the manifest works and remove the manifest work
//   finalizer from the managed cluster
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileHypershift) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
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

	mode, err := helpers.DetermineKlusterletMode(managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}
	if mode != constants.KlusterletDeployModeHypershiftDetached {
		return reconcile.Result{}, nil
	}

	reqLogger.Info("Reconciling the manifest works of the hypershift managed cluster")

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

	// after managed cluster joined, apply klusterlet manifest works from import secret
	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.clientHolder.KubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, importSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// wait for the import secret to exist, do nothing
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if err := helpers.ValidateHypershiftDetachedImportSecret(importSecret); err != nil {
		return reconcile.Result{}, err
	}

	managementCluster, err := helpers.GetManagementCluster(managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	manifestWork := createHypershiftDetachedManifestWork(managedCluster, importSecret, managementCluster)
	containAutoImport := false

	autoImportSecret, err := r.kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, constants.AutoImportSecretName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		// the auto import secret has not be created or has been deleted, continue
	} else {
		// add auto import secret to the manifestwork
		err := r.addAutoImportSecret(managedCluster.Name, autoImportSecret, manifestWork)
		if err != nil {
			return reconcile.Result{}, err
		}
		containAutoImport = true
	}

	err = helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, manifestWork)
	if !containAutoImport {
		return reconcile.Result{}, err
	}
	if err != nil {
		errStatus := helpers.UpdateManagedClusterStatus(r.client, r.recorder, managedClusterName, metav1.Condition{
			Type:    "ExternalManagedKubeconfigCreatedSucceeded",
			Status:  metav1.ConditionFalse,
			Message: fmt.Sprintf("Unable to create external managed kubeconfig for %s: %s", managedCluster.Name, err.Error()),
			Reason:  "ExternalManagedKubeconfigNotCreated",
		})
		if errStatus != nil {
			return reconcile.Result{}, errStatus
		}

		errRetry := helpers.UpdateAutoImportRetryTimes(ctx, autoImportSecret.DeepCopy(), r.kubeClient, r.recorder)
		return reconcile.Result{}, utilerrors.NewAggregate([]error{err, errRetry})
	}

	err = helpers.UpdateManagedClusterStatus(r.client, r.recorder, managedClusterName, metav1.Condition{
		Type:    "ExternalManagedKubeconfigCreatedSucceeded",
		Status:  metav1.ConditionTrue,
		Message: "Created succeeded",
		Reason:  "ExternalManagedKubeconfigCreated",
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.kubeClient.CoreV1().Secrets(autoImportSecret.Namespace).Delete(ctx, autoImportSecret.Name, metav1.DeleteOptions{})
	if err != nil {
		return reconcile.Result{}, err
	}

	r.recorder.Eventf("AutoImportSecretDeleted",
		fmt.Sprintf("The managed cluster %s is imported, delete its auto import secret", managedClusterName))

	return reconcile.Result{}, nil

}

func klusterletNamespace(managedCluster string) string {
	return fmt.Sprintf("klusterlet-%s", managedCluster)
}

func (r *ReconcileHypershift) addAutoImportSecret(managedClusterName string, secret *corev1.Secret, manifestWork *workv1.ManifestWork) error {
	kubeconfig := secret.Data["kubeconfig"]
	if len(kubeconfig) == 0 {
		return fmt.Errorf("import secret invalid, the field kubeconfig must exist in the secret for detached mode")
	}

	config := struct {
		KlusterletNamespace       string
		ExternalManagedKubeconfig string
	}{
		KlusterletNamespace:       klusterletNamespace(managedClusterName),
		ExternalManagedKubeconfig: base64.StdEncoding.EncodeToString(kubeconfig),
	}

	template, err := manifestFiles.ReadFile(klusterletDetachedExternalKubeconfig)
	if err != nil {
		return err
	}
	externalKubeYAML := helpers.MustCreateAssetFromTemplate(klusterletDetachedExternalKubeconfig, template, config)
	externalKubeJSON, err := yaml.YAMLToJSON(externalKubeYAML)
	if err != nil {
		return err
	}

	manifestWork.Spec.Workload.Manifests = append(manifestWork.Spec.Workload.Manifests,
		workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: externalKubeJSON},
		})

	return nil
}

// deleteManifestWorks deletes manifest works when a managed cluster is deleting
// If the managed cluster is unavailable, we will force delete all manifest works
// If the managed cluster is available, we will
//   1. delete the manifest work with the postpone-delete annotation until 10 min
//      after the cluster is deleted.
//   2. delete the manifest works that do not include klusterlet addon works
//   3. delete the klusterlet manifest work
func (r *ReconcileHypershift) deleteManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork) error {
	if len(works) == 0 {
		return nil
	}

	if helpers.IsClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all manifest works
		if err := helpers.ForceDeleteAllManifestWorks(ctx, r.clientHolder.RuntimeClient, r.recorder, works); err != nil {
			return err
		}
		return r.deleteKlusterletManifestWork(ctx, r.clientHolder.RuntimeClient, r.recorder, cluster)
	}

	// delete works that do not include klusterlet addon works, the addon works will be removed by
	// klusterlet-addon-controller, we need to wait the klusterlet-addon-controller delete them
	ignoreAddons := func(clusterName string, manifestWork workv1.ManifestWork) bool {
		return strings.HasPrefix(manifestWork.GetName(), fmt.Sprintf("%s-klusterlet-addon", manifestWork.GetNamespace()))
	}
	err := helpers.DeleteManifestWorkWithSelector(ctx, r.clientHolder.RuntimeClient, r.recorder, cluster, works, ignoreAddons)
	if err != nil {
		return err
	}

	ignoreNothing := func(_ string, _ workv1.ManifestWork) bool { return false }
	noPending, err := helpers.NoPendingManifestWorks(ctx, r.clientHolder.RuntimeClient, log, cluster.GetName(), ignoreNothing)
	if err != nil {
		return err
	}
	if !noPending {
		// still have other works, do nothing
		return nil
	}

	// no other manifest works, delete klusterlet manifest works in the management cluster namespace
	return r.deleteKlusterletManifestWork(ctx, r.clientHolder.RuntimeClient, r.recorder, cluster)
}

func (r *ReconcileHypershift) deleteKlusterletManifestWork(ctx context.Context, runtimeClient client.Client,
	recorder events.Recorder, cluster *clusterv1.ManagedCluster) error {
	klusterletName := fmt.Sprintf("%s-%s", cluster.Name, constants.HypershiftDetachedManifestworkSuffix)
	managementCluster, err := helpers.GetManagementCluster(cluster)
	if err != nil {
		return err
	}

	return helpers.DeleteManifestWork(ctx, runtimeClient, recorder, managementCluster, klusterletName)
}

// createHypershiftDetachedManifestWork creates a manifestwork from import secret for hypershift detached mode cluster
func createHypershiftDetachedManifestWork(managedCluster *clusterv1.ManagedCluster,
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

	// For detached mode, the klusterletManifestWork contains a klusterlet CR and a bootstrap secret,
	// if auto import secret is provided, there also be a external managed kubeconfig secret, and when
	// deleting, we need to leave the external managed kubeconfig on the managed cluster(klusterlet
	// operator will use this secret to clean resources on the managed cluster, after the cleanup
	// finished, the klusterlet operator will delete the secret.) and delete others.
	return &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, HypershiftDetachedManifestworkSuffix),
			Namespace: manifestWorkNamespace,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: workv1.DeletePropagationPolicyTypeSelectivelyOrphan,
				SelectivelyOrphan: &workv1.SelectivelyOrphan{
					OrphaningRules: []workv1.OrphaningRule{
						{
							Resource:  "Secret",
							Name:      "external-managed-kubeconfig",
							Namespace: klusterletNamespace(managedCluster.GetName()),
						},
					},
				},
			},
		},
	}
}
