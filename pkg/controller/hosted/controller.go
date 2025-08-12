// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hosted

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//go:embed manifests
var manifestFiles embed.FS

var klusterletHostedExternalKubeconfig = "manifests/external_managed_secret.yaml"

var log = logf.Log.WithName(ControllerName)

// ReconcileHosted reconciles the Hosted mode ManagedClusters of the ManifestWorks object
type ReconcileHosted struct {
	clientHolder   *helpers.ClientHolder
	informerHolder *source.InformerHolder
	scheme         *runtime.Scheme
	recorder       events.Recorder
	mcRecorder     kevents.EventRecorder
}

// blank assignment to verify that ReconcileHosted implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileHosted{}

func NewReconcileHosted(clientHolder *helpers.ClientHolder, informerHolder *source.InformerHolder,
	scheme *runtime.Scheme, recorder events.Recorder, mcRecorder kevents.EventRecorder) *ReconcileHosted {
	return &ReconcileHosted{
		clientHolder:   clientHolder,
		informerHolder: informerHolder,
		scheme:         scheme,
		recorder:       recorder,
		mcRecorder:     mcRecorder,
	}
}

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

	if helpers.DetermineKlusterletMode(managedCluster) != operatorv1.InstallModeHosted {
		return reconcile.Result{}, nil
	}

	reqLogger.V(5).Info("Reconciling the manifest works of the hosted mode managed cluster")

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	cn := meta.FindStatusCondition(managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if cn == nil {
		return reconcile.Result{Requeue: true}, helpers.UpdateManagedClusterImportCondition(
			r.clientHolder.RuntimeClient,
			managedCluster,
			helpers.NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterWaitForImporting,
				"Wait for importing"),
			r.mcRecorder,
		)
	}

	autoImportSecret, err := r.informerHolder.AutoImportSecretLister.Secrets(managedCluster.Name).Get(
		constants.AutoImportSecretName)
	// if it is not found error, still continue to check other things
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	result, condition, iErr := r.importCluster(ctx, managedCluster, autoImportSecret)
	if err := helpers.UpdateManagedClusterImportCondition(
		r.clientHolder.RuntimeClient,
		managedCluster,
		condition,
		r.mcRecorder,
	); err != nil {
		return reconcile.Result{}, err
	}

	// if the auto import secret exists and the cluster is imported successfully, delete the secret
	if autoImportSecret != nil && condition.Status == metav1.ConditionTrue {
		reqLogger.Info(fmt.Sprintf("External managed kubeconfig is created, try to delete its auto import secret %s/%s",
			managedCluster.Name, constants.AutoImportSecretName))
		if err := helpers.DeleteAutoImportSecret(ctx,
			r.clientHolder.KubeClient, autoImportSecret, r.recorder); err != nil {
			return reconcile.Result{}, err
		}
	}

	return result, iErr
}

func (r *ReconcileHosted) importCluster(ctx context.Context, managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret) (reconcile.Result, metav1.Condition, error) {
	hostedWorksSelector := labels.SelectorFromSet(map[string]string{constants.HostedClusterLabel: managedCluster.Name})

	hostingClusterName, err := helpers.GetHostingCluster(managedCluster)
	if err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterWaitForImporting,
				"Waiting for the user to specify the hosting cluster"),
			nil
	}

	hostingCluster := &clusterv1.ManagedCluster{}
	err = r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: hostingClusterName}, hostingCluster)
	if err != nil {
		message := fmt.Sprintf("Validate the hosting cluster, get managed cluster failed, error: %v", err)
		if errors.IsNotFound(err) {
			message = "Hosting cluster is not a managed cluster of the hub"
		}
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				message),
			err
	}

	if !hostingCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImportFailed,
				fmt.Sprintf("The hosting cluster %s is being deleted", hostingClusterName)),
			nil
	}

	hostedWorks, err := r.informerHolder.HostedWorkLister.ManifestWorks(hostingClusterName).List(hostedWorksSelector)
	if err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Get hosted manifest works failed, error: %v", err)),
			err
	}

	// after the hosted works are created, make sure the managed cluster has manifest work finalizer
	if err := helpers.AssertManifestWorkFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder,
		managedCluster, len(hostedWorks)); err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Add finalizer for manifest work failed, error: %v", err)),
			err
	}

	// apply klusterlet manifest works klustelet to the management namespace from import secret
	// to trigger the joining process.
	importSecretName := fmt.Sprintf("%s-%s", managedCluster.Name, constants.ImportSecretNameSuffix)
	importSecret, err := r.informerHolder.ImportSecretLister.Secrets(managedCluster.Name).Get(importSecretName)
	if errors.IsNotFound(err) {
		// wait for the import secret to exist, do nothing
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				"Wait for import secret to be created"),
			nil
	}
	if err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Get import secret failed, error: %v", err)),
			err
	}

	if err := helpers.ValidateImportSecret(importSecret); err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImportFailed,
				fmt.Sprintf("Import secret is invalid, error: %v", err)),
			nil
	}

	manifestWork := createHostingManifestWork(managedCluster.Name, importSecret, hostingClusterName)
	_, err = helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, manifestWork)
	if err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Apply importing resources to the hosting cluster failed, error: %v", err)),
			err
	}

	available, err := helpers.IsManifestWorksAvailable(ctx,
		r.clientHolder.WorkClient, manifestWork.Namespace, manifestWork.Name)
	if err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Check importing resources availability on the hosting cluster failed, error: %v", err)),
			err
	}
	if !available {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				"Wait for importing resources to be available on the hosting cluster"),
			nil
	}

	// if the auto import secret exists; create it on the hosting cluster by manifestwork
	if autoImportSecret != nil {
		manifestWork, err = createManagedKubeconfigManifestWork(
			managedCluster.Name, autoImportSecret, hostingClusterName, hostingCluster)
		if err != nil {
			return reconcile.Result{},
				helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
					constants.ConditionReasonManagedClusterImporting,
					fmt.Sprintf("Build external managed kubeconfig manifest work failed, error: %v", err)),
				err
		}

		_, err = helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, manifestWork)
		if err != nil {
			return reconcile.Result{},
				helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
					constants.ConditionReasonManagedClusterImporting,
					fmt.Sprintf("Apply external managed kubeconfig to the hosting cluster failed, error: %v", err)),
				err
		}
	}

	// check the klusterlet feedback rule
	created, err := r.externalManagedKubeconfigCreated(ctx, managedCluster.Name, hostingClusterName)
	if err != nil {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Check external managed kubeconfig availability failed, error: %v", err)),
			err
	}
	if !created {
		return reconcile.Result{},
			helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				"Wait for the user to provide the external managed kubeconfig"),
			err
	}
	return reconcile.Result{},
		helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionTrue,
			constants.ConditionReasonManagedClusterImported,
			"Import succeeded"),
		nil
}

func (r *ReconcileHosted) externalManagedKubeconfigCreated(
	ctx context.Context, managedClusterName, hostingClusterName string) (bool, error) {
	name := helpers.HostedKlusterletManifestWorkName(managedClusterName)
	namespace := hostingClusterName
	mw, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	for _, manifest := range mw.Status.ResourceStatus.Manifests {
		if manifest.ResourceMeta.Group != operatorv1.GroupName ||
			(manifest.ResourceMeta.Kind != "Klusterlet" && manifest.ResourceMeta.Resource != "klusterlets") ||
			manifest.ResourceMeta.Name != hostedKlusterletCRName(managedClusterName) {
			continue
		}

		for _, fb := range manifest.StatusFeedbacks.Values {
			if fb.Name == "ReadyToApply-status" &&
				fb.Value.String != nil && strings.EqualFold(*fb.Value.String, "True") {
				return true, nil
			}

		}
	}

	return false, nil
}

func klusterletNamespace(managedCluster string) string {
	return fmt.Sprintf("klusterlet-%s", managedCluster)
}

// createHostingManifestWork creates the manifestwork from import secret for hosted mode cluster
// into the hosting cluster
func createHostingManifestWork(managedClusterName string,
	importSecret *corev1.Secret, manifestWorkNamespace string) *workv1.ManifestWork {
	manifests := []workv1.Manifest{}
	importYaml := importSecret.Data[constants.ImportSecretImportYamlKey]
	for _, yamlData := range helpers.SplitYamls(importYaml) {
		jsonData, err := yaml.YAMLToJSON(yamlData)
		if err != nil {
			panic(err)
		}

		unstructuredObj := &unstructured.Unstructured{}
		if err := json.Unmarshal(jsonData, unstructuredObj); err != nil {
			klog.Errorf("failed to unmarshal object %s: %v", string(jsonData), err)
			continue
		}
		// the namespace has been created by other component, so should not be managed by manifestWork.
		// otherwise, the ns will be deleted when the manifestWork is deleting, may bring some cleanup issue.
		if unstructuredObj.GetKind() == "Namespace" {
			continue
		}
		manifests = append(manifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: jsonData},
		})
	}

	// For hosted mode, the klusterletManifestWork only contains a klusterlet CR, a bootstrap secret and an agent namespace,
	// delete klusterlet and bootstrap in foreground.
	return &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      helpers.HostedKlusterletManifestWorkName(managedClusterName),
			Namespace: manifestWorkNamespace,
			Labels: map[string]string{
				constants.HostedClusterLabel: managedClusterName,
			},
			Annotations: map[string]string{
				// make sure the klusterlet manifestWork is the last to be deleted.
				clusterv1.CleanupPriorityAnnotationKey: "100",
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: workv1.DeletePropagationPolicyTypeForeground,
			},
			ManifestConfigs: []workv1.ManifestConfigOption{
				{
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "ReadyToApply-reason",
									Path: `.status.conditions[?(@.type=="ReadyToApply")].reason`,
								},
								{
									Name: "ReadyToApply-status",
									Path: `.status.conditions[?(@.type=="ReadyToApply")].status`,
								},
								{
									Name: "ReadyToApply-message",
									Path: `.status.conditions[?(@.type=="ReadyToApply")].message`,
								},
								{
									Name: "ReadyToApply-lastTransitionTime",
									Path: `.status.conditions[?(@.type=="ReadyToApply")].lastTransitionTime`,
								},
								{
									Name: "ReadyToApply-observedGeneration",
									Path: `.status.conditions[?(@.type=="ReadyToApply")].observedGeneration`,
								},
							},
						},
					},
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:    operatorv1.GroupName,
						Resource: "klusterlets",
						// TODO: read this name by parsing the klusterlel resource in the importSecret
						Name: hostedKlusterletCRName(managedClusterName),
					},
				},
			},
		},
	}
}

func hostedKlusterletCRName(managedClusterName string) string {
	return fmt.Sprintf("klusterlet-%s", managedClusterName)
}

func createManagedKubeconfigManifestWork(managedClusterName string, importSecret *corev1.Secret,
	manifestWorkNamespace string, hostingCluster *clusterv1.ManagedCluster) (*workv1.ManifestWork, error) {
	kubeconfig := importSecret.Data["kubeconfig"]
	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("import secret invalid, the field kubeconfig must exist in the secret for hosted mode")
	}

	// Check if the hosting cluster has the local-cluster label set to "true"
	isLocalCluster := false
	if hostingCluster != nil && hostingCluster.Labels != nil {
		localClusterValue, exists := hostingCluster.Labels[constants.SelfManagedLabel]
		if exists && localClusterValue == "true" {
			isLocalCluster = true
		}
	}

	config := struct {
		KlusterletNamespace       string
		ExternalManagedKubeconfig string
		IsLocalCluster            bool
	}{
		KlusterletNamespace:       klusterletNamespace(managedClusterName),
		ExternalManagedKubeconfig: base64.StdEncoding.EncodeToString(kubeconfig),
		IsLocalCluster:            isLocalCluster,
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
			Name:      helpers.HostedManagedKubeConfigManifestWorkName(managedClusterName),
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
