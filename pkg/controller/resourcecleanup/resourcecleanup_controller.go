package resourcecleanup

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	kevents "k8s.io/client-go/tools/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(ControllerName)

// ReconcileResourceCleanup reconciles a ManagedCluster object
type ReconcileResourceCleanup struct {
	clientHolder *helpers.ClientHolder
	recorder     events.Recorder
	mcRecorder   kevents.EventRecorder
}

// NewReconcileResourceCleanup creates a new ReconcileResourceCleanup
func NewReconcileResourceCleanup(
	clientHolder *helpers.ClientHolder,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
) *ReconcileResourceCleanup {
	return &ReconcileResourceCleanup{
		clientHolder: clientHolder,
		recorder:     recorder,
		mcRecorder:   mcRecorder,
	}
}

// This controller is to clean up the resources after the cluster is deleted.
// From MCE 2.9, ResourceCleanup featureGate is enabled in registration controller, and the addons and manifestWorks in
// the cluster ns will be deleted by the registration controller except the manifestWorks in the hosting cluster ns.
// This controller doing these jobs:
//  1. if cluster is not found, will check the cluster ns, and force delete all addons, manifestoWorks and workRoleBinding.
//  2. if cluster is available, force delete the klusterletCRD manifestWork after there is no addons and other manifestWorks,
//     delete the manifestWorks in the hosting cluster ns if the cluster is hosted mode.
//  3. if cluster is unavailable, force delete all addons, manifestWorks and workRoleBinding in the cluster ns,
//     and the manifestWorks in the hosting cluster ns if the cluster is hosted mode.
var _ reconcile.Reconciler = &ReconcileResourceCleanup{}

func (r *ReconcileResourceCleanup) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	cluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: request.Name}, cluster)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, r.orphanCleanup(ctx, request.Name)
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if cluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	copyCluster := cluster.DeepCopy()

	if err = r.updateDetachingCondition(copyCluster); err != nil {
		return reconcile.Result{}, err
	}

	if clusterNeedForceDelete(copyCluster) {
		reqLogger.Info(fmt.Sprintf("cluster %s is unavailable or not accepted, start force cleanup.", copyCluster.Name))
		if err = r.forceCleanup(ctx, copyCluster); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if err = r.Cleanup(ctx, copyCluster); err != nil {
			return reconcile.Result{}, err
		}
	}

	if completed, err := r.cleanupCompleted(ctx, copyCluster); err != nil || !completed {
		return reconcile.Result{RequeueAfter: 2 * time.Second}, err
	}

	// remove finalizers
	return reconcile.Result{}, r.removeClusterFinalizers(ctx, copyCluster)
}

func (r *ReconcileResourceCleanup) updateDetachingCondition(cluster *clusterv1.ManagedCluster) error {
	conditionReason := constants.ConditionReasonManagedClusterDetaching
	conditionMsg := "The managed cluster is being detached now"

	if clusterNeedForceDelete(cluster) {
		conditionReason = constants.ConditionReasonManagedClusterForceDetaching
		conditionMsg = "The managed cluster is being detached by force"
	}

	// add a detaching condition to the managed cluster if the managed cluster is deleting
	ic := meta.FindStatusCondition(cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if ic == nil || (ic.Reason != conditionReason && ic.Message != conditionMsg) {
		return helpers.UpdateManagedClusterImportCondition(
			r.clientHolder.RuntimeClient,
			cluster,
			helpers.NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				conditionReason,
				conditionMsg,
			),
			r.mcRecorder,
		)
	}
	return nil
}

func (r *ReconcileResourceCleanup) forceDeleteManifestWorks(
	ctx context.Context, clusterName string) error {
	manifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(clusterName).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	return helpers.ForceDeleteAllManifestWorks(ctx, r.clientHolder.WorkClient, r.recorder, manifestWorks.Items)

}

func (r *ReconcileResourceCleanup) forceDeleteHostingManifestWorks(ctx context.Context,
	hostingCluster, hostedCluster string) error {
	hostingWorksSelector := labels.SelectorFromSet(map[string]string{constants.HostedClusterLabel: hostedCluster})
	hostingManifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).List(
		ctx, metav1.ListOptions{LabelSelector: hostingWorksSelector.String()})
	if err != nil || len(hostingManifestWorks.Items) == 0 {
		return err
	}

	return helpers.ForceDeleteAllManifestWorks(ctx, r.clientHolder.WorkClient, r.recorder, hostingManifestWorks.Items)
}

func (r *ReconcileResourceCleanup) orphanCleanup(ctx context.Context, clusterName string) error {
	var errs []error
	_, err := r.clientHolder.KubeClient.CoreV1().Namespaces().Get(ctx, clusterName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = helpers.ForceDeleteAllManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, clusterName, r.recorder); err != nil {
		errs = append(errs, err)
	}

	if err = r.forceDeleteManifestWorks(ctx, clusterName); err != nil {
		errs = append(errs, err)
	}

	if err = helpers.ForceDeleteWorkRoleBinding(ctx, r.clientHolder.KubeClient, clusterName, r.recorder); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

func (r *ReconcileResourceCleanup) forceCleanup(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	var errs []error
	_, err := r.clientHolder.KubeClient.CoreV1().Namespaces().Get(ctx, cluster.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = helpers.ForceDeleteAllManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, cluster.Name, r.recorder); err != nil {
		errs = append(errs, err)
	}

	if err = r.forceDeleteManifestWorks(ctx, cluster.Name); err != nil {
		errs = append(errs, err)
	}

	hostingCluster, _ := helpers.GetHostingCluster(cluster)
	if helpers.IsHostedCluster(cluster) && hostingCluster != "" {
		if err = r.forceDeleteHostingManifestWorks(ctx, hostingCluster, cluster.Name); err != nil {
			errs = append(errs, err)
		}
	}

	if err = helpers.ForceDeleteWorkRoleBinding(ctx, r.clientHolder.KubeClient, cluster.Name, r.recorder); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

func (r *ReconcileResourceCleanup) Cleanup(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	// the addons and manifestWorks in the cluster ns should be deleted by registration controller.
	// klusterletCRD manifestWork will be left at the last, need to force delete it.
	// need to clean up the manifestWorks in the hosting cluster ns after the addons and manifestWorks
	// in the cluster ns are deleted.
	if addons, err := helpers.ListManagedClusterAddons(ctx,
		r.clientHolder.RuntimeClient, cluster.Name); err != nil || len(addons.Items) != 0 {
		return err
	}

	works, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(cluster.Name).List(ctx, metav1.ListOptions{})
	if err != nil || len(works.Items) > 1 {
		return err
	}
	// only klusterletCRD manifestWork will be orphaned at the last, need to force delete.
	klusterletCRDWorkName := fmt.Sprintf("%s-%s", cluster.Name, constants.KlusterletCRDsSuffix)
	if len(works.Items) == 1 && works.Items[0].Name == klusterletCRDWorkName {
		if err = helpers.ForceDeleteManifestWork(ctx, r.clientHolder.WorkClient, r.recorder,
			cluster.Name, klusterletCRDWorkName); err != nil {
			return err
		}
	}

	hostingCluster, _ := helpers.GetHostingCluster(cluster)
	if !helpers.IsHostedCluster(cluster) || hostingCluster == "" {
		return nil
	}

	hostingWorksSelector := labels.SelectorFromSet(map[string]string{constants.HostedClusterLabel: cluster.Name})
	hostingManifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).List(
		ctx, metav1.ListOptions{
			LabelSelector: hostingWorksSelector.String(),
		})
	if err != nil || len(hostingManifestWorks.Items) == 0 {
		return err
	}

	var errs []error
	for _, manifestWork := range hostingManifestWorks.Items {
		if err = r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).
			Delete(ctx, manifestWork.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

func (r *ReconcileResourceCleanup) cleanupCompleted(ctx context.Context, cluster *clusterv1.ManagedCluster) (bool, error) {
	manifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(cluster.Name).List(ctx, metav1.ListOptions{})
	if err != nil || len(manifestWorks.Items) != 0 {
		return false, err
	}

	addons, err := helpers.ListManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, cluster.Name)
	if err != nil || len(addons.Items) != 0 {
		return false, err
	}

	workRoleBinding, err := helpers.GetWorkRoleBinding(ctx, r.clientHolder.RuntimeClient, cluster.Name)
	if err != nil || workRoleBinding != nil {
		return false, err
	}

	hostingCluster, _ := helpers.GetHostingCluster(cluster)
	if !helpers.IsHostedCluster(cluster) || hostingCluster == "" {
		return true, nil
	}

	// check the manifestWorks on hosting cluster ns if cluster is hosted mode
	hostingWorksSelector := labels.SelectorFromSet(map[string]string{constants.HostedClusterLabel: cluster.Name})
	hostingManifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).List(
		ctx, metav1.ListOptions{
			LabelSelector: hostingWorksSelector.String(),
		})
	if err != nil || len(hostingManifestWorks.Items) != 0 {
		return false, err
	}

	return true, nil
}

func (r *ReconcileResourceCleanup) removeClusterFinalizers(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	copiedFinalizers := []string{}
	for _, finalizer := range cluster.Finalizers {
		if finalizer == constants.ImportFinalizer ||
			finalizer == constants.ManifestWorkFinalizer {
			continue
		}
		copiedFinalizers = append(copiedFinalizers, finalizer)
	}

	if len(cluster.Finalizers) == len(copiedFinalizers) {
		return nil
	}

	patch := client.MergeFrom(cluster.DeepCopy())
	cluster.Finalizers = copiedFinalizers
	err := r.clientHolder.RuntimeClient.Patch(ctx, cluster, patch)
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func clusterNeedForceDelete(cluster *clusterv1.ManagedCluster) bool {
	// need to do force deletion when cluster is deleting but not accepted or not available
	if !cluster.Spec.HubAcceptsClient {
		return true
	}
	return helpers.IsClusterUnavailable(cluster)
}
