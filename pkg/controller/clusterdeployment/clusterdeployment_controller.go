// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusterdeployment

import (
	"context"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kevents "k8s.io/client-go/tools/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(ControllerName)

// ReconcileClusterDeployment reconciles the clusterdeployment that is in the managed cluster namespace
// to import the managed cluster
type ReconcileClusterDeployment struct {
	client                   client.Client
	kubeClient               kubernetes.Interface
	informerHolder           *source.InformerHolder
	recorder                 events.Recorder
	mcRecorder               kevents.EventRecorder
	importHelper             *helpers.ImportHelper
	autoImportStrategyGetter helpers.AutoImportStrategyGetterFunc
}

func NewReconcileClusterDeployment(
	client client.Client,
	kubeClient kubernetes.Interface,
	informerHolder *source.InformerHolder,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
	autoImportStrategyGetter helpers.AutoImportStrategyGetterFunc,
) *ReconcileClusterDeployment {

	return &ReconcileClusterDeployment{
		client:         client,
		kubeClient:     kubeClient,
		informerHolder: informerHolder,
		recorder:       recorder,
		mcRecorder:     mcRecorder,
		importHelper: helpers.NewImportHelper(informerHolder, recorder, log).
			WithGenerateClientHolderFunc(helpers.GenerateImportClientFromKubeConfigSecret),
		autoImportStrategyGetter: autoImportStrategyGetter,
	}
}

// blank assignment to verify that ReconcileClusterDeployment implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// Reconcile the clusterdeployment that is in the managed cluster namespace to import the managed cluster.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileClusterDeployment) Reconcile(
	ctx context.Context, request reconcile.Request) (reconcile.Result, error) {

	reqLogger := log.WithValues("Request.Name", request.Name)

	clusterName := request.Name

	clusterDeployment := &hivev1.ClusterDeployment{}
	err := r.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: clusterName}, clusterDeployment)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.V(5).Info("Reconciling clusterdeployment")

	if !clusterDeployment.DeletionTimestamp.IsZero() {
		// We do not set this finalizer anymore, but we still need to remove it for backward compatible
		// the clusterdeployment is deleting, its managed cluster may already be detached (the managed
		// cluster has been deleted, but the namespace is remained), if it has import finalizer, we
		// remove its namespace
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

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	if _, autoImportDisabled := managedCluster.Annotations[apiconstants.DisableAutoImportAnnotation]; autoImportDisabled {
		// skip if auto import is disabled
		reqLogger.Info("Auto import is disabled", "managedCluster", managedCluster.Name)
		return reconcile.Result{}, nil
	} else {
		reqLogger.V(5).Info("Auto import is enabled", "managedCluster", managedCluster.Name)
	}

	autoImportStrategy, err := r.autoImportStrategyGetter()
	if err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Auto import strategy is fetched", "managedCluster", managedCluster.Name, "AutoImportStrategy", autoImportStrategy)
	importSucceeded := meta.IsStatusConditionTrue(managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if autoImportStrategy == apiconstants.AutoImportStrategyImportOnly && importSucceeded {
		reqLogger.Info("Auto import is skipped due to the auto import strategy",
			"managedCluster", managedCluster.Name,
			"autoImportStrategy", autoImportStrategy,
			"importSucceeded", importSucceeded,
		)
		return reconcile.Result{}, nil
	}

	if !clusterDeployment.Spec.Installed {
		// cluster deployment is not installed yet, do nothing
		reqLogger.Info("The hive managed cluster is not installed, skipped", "managedcluster", clusterName)
		return reconcile.Result{}, nil
	}

	if clusterDeployment.Spec.ClusterPoolRef != nil && clusterDeployment.Spec.ClusterPoolRef.ClaimedTimestamp.IsZero() {
		// cluster deployment is not claimed yet, do nothing
		reqLogger.Info("The hive managed cluster is not claimed, skipped", "managedcluster", clusterName)
		return reconcile.Result{}, nil
	}

	// set managed cluster created-via annotation
	if err := r.setCreatedViaAnnotation(ctx, clusterDeployment, managedCluster); err != nil {
		return reconcile.Result{}, err
	}

	// if there is an auto import secret in the managed cluster namespace, we will use the auto import secret
	// to import the cluster
	_, err = r.informerHolder.AutoImportSecretLister.Secrets(clusterName).Get(constants.AutoImportSecretName)
	if err == nil {
		reqLogger.Info("The hive managed cluster has auto import secret, skipped", "managedcluster", clusterName)
		return reconcile.Result{}, nil
	}
	if !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	if clusterDeployment.Spec.ClusterMetadata == nil {
		reqLogger.Info("the clusterMetaData is not updated, skipped", "managedcluster", clusterName)
		return reconcile.Result{}, nil
	}

	secretRefName := clusterDeployment.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name
	hiveSecret, err := r.kubeClient.CoreV1().Secrets(clusterName).Get(ctx, secretRefName, metav1.GetOptions{})
	if err != nil {
		return reconcile.Result{}, err
	}

	result, condition, modified, iErr := r.importHelper.Import(false, managedCluster, hiveSecret)
	// if resources are applied but NOT modified, will not update the condition, keep the original condition.
	// This check is to prevent the current controller and import status controller from modifying the
	// ManagedClusterImportSucceeded condition of the managed cluster in a loop
	if !helpers.ImportingResourcesApplied(&condition) || modified {
		if err := helpers.UpdateManagedClusterImportCondition(
			r.client,
			managedCluster,
			condition,
			r.mcRecorder,
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	return result, iErr
}

func (r *ReconcileClusterDeployment) setCreatedViaAnnotation(
	ctx context.Context, clusterDeployment *hivev1.ClusterDeployment, cluster *clusterv1.ManagedCluster) error {
	patch := client.MergeFrom(cluster.DeepCopy())

	viaAnnotation := cluster.Annotations[constants.CreatedViaAnnotation]
	if viaAnnotation == constants.CreatedViaDiscovery {
		// create-via annotaion is discovery, do nothing
		return nil
	}

	modified := resourcemerge.BoolPtr(false)
	if clusterDeployment.Spec.Platform.AgentBareMetal != nil {
		resourcemerge.MergeMap(modified,
			&cluster.Annotations, map[string]string{constants.CreatedViaAnnotation: constants.CreatedViaAI})
	} else {
		resourcemerge.MergeMap(modified,
			&cluster.Annotations, map[string]string{constants.CreatedViaAnnotation: constants.CreatedViaHive})
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

func (r *ReconcileClusterDeployment) removeImportFinalizer(
	ctx context.Context, clusterDeployment *hivev1.ClusterDeployment) error {

	hasImportFinalizer := false

	for _, finalizer := range clusterDeployment.Finalizers {
		if finalizer == constants.ImportFinalizer {
			hasImportFinalizer = true
			break
		}
	}

	if !hasImportFinalizer {
		// the clusterdeployment does not have import finalizer, ignore it
		log.Info("the clusterDeployment does not have import finalizer, skip it",
			"clusterDeployment", clusterDeployment.Name)
		return nil
	}

	if len(clusterDeployment.Finalizers) != 1 {
		// the clusterdeployment has other finalizers, wait hive to remove them
		log.Info("wait hive to remove the finalizers from the clusterdeployment",
			"clusterdeployment", clusterDeployment.Name)
		return nil
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
