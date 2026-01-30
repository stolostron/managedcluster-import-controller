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
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
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

	// The CRD manifestwork should NOT be force deleted directly because force delete
	// removes the finalizers before work-agent can process the deletion, which means
	// the CRD (contained in the manifestwork) won't be deleted by work-agent.
	// Instead, we delete other manifestworks first and let the CRD manifestwork be
	// deleted through normal cleanup or registration controller's GC.
	klusterletCRDWorkName := fmt.Sprintf("%s-%s", clusterName, constants.KlusterletCRDsSuffix)
	var nonCRDWorks []workv1.ManifestWork
	var crdWork *workv1.ManifestWork
	for i := range manifestWorks.Items {
		if manifestWorks.Items[i].Name == klusterletCRDWorkName {
			crdWork = &manifestWorks.Items[i]
		} else {
			nonCRDWorks = append(nonCRDWorks, manifestWorks.Items[i])
		}
	}

	// Force delete non-CRD manifestworks first
	if err := helpers.ForceDeleteAllManifestWorks(ctx, r.clientHolder.WorkClient, r.recorder, nonCRDWorks); err != nil {
		return err
	}

	// For CRD manifestwork, only force delete if work-agent has started processing (WorkDeleting condition)
	// This ensures the CRD gets deleted by work-agent before we remove the finalizers
	if crdWork != nil {
		// Force delete if:
		// 1. WorkDeleting condition is true (work-agent has started processing), OR
		// 2. ManifestWork has been deleting for more than 30 seconds (work-agent is not responding)
		shouldForceDelete := meta.IsStatusConditionTrue(crdWork.Status.Conditions, workv1.WorkDeleting)
		if !shouldForceDelete && !crdWork.DeletionTimestamp.IsZero() {
			// Check if deletion has been pending for too long (30 seconds)
			if time.Since(crdWork.DeletionTimestamp.Time) > 30*time.Second {
				log.Info(fmt.Sprintf("CRD manifestwork %s has been deleting for over 30s without WorkDeleting condition, force deleting",
					klusterletCRDWorkName))
				shouldForceDelete = true
			}
		}

		if shouldForceDelete {
			return helpers.ForceDeleteManifestWork(ctx, r.clientHolder.WorkClient, r.recorder,
				clusterName, klusterletCRDWorkName)
		}

		// If WorkDeleting is not true yet and not timed out, just trigger deletion (don't force)
		// and let the controller retry
		if crdWork.DeletionTimestamp.IsZero() {
			if err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(clusterName).Delete(
				ctx, klusterletCRDWorkName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (r *ReconcileResourceCleanup) deleteHostingManifestWorks(ctx context.Context,
	hostingCluster, hostedCluster string) error {
	hostingWorksSelector := labels.SelectorFromSet(map[string]string{constants.HostedClusterLabel: hostedCluster})
	hostingManifestWorks, err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).List(
		ctx, metav1.ListOptions{LabelSelector: hostingWorksSelector.String()})
	if err != nil || len(hostingManifestWorks.Items) == 0 {
		return err
	}

	var errs []error
	var hostingWorkNames []string
	var klusterletHostingWorkName, kubeconfigHostingWorkName string
	// the work deletion order for hosted cluster:
	// 1. all addon works in hosted and hosting cluster ns
	// 2. klusterlet work in hosting cluster ns
	// 3. hosted kubeconfig work in hosting cluster ns
	for _, manifestWork := range hostingManifestWorks.Items {
		if manifestWork.Name == helpers.HostedKlusterletManifestWorkName(hostedCluster) {
			klusterletHostingWorkName = manifestWork.Name
			continue
		}

		if manifestWork.Name == helpers.HostedManagedKubeConfigManifestWorkName(hostedCluster) {
			kubeconfigHostingWorkName = manifestWork.Name
			continue
		}

		hostingWorkNames = append(hostingWorkNames, manifestWork.Name)
	}

	if len(hostingWorkNames) == 0 {
		if klusterletHostingWorkName != "" {
			return r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).
				Delete(ctx, klusterletHostingWorkName, metav1.DeleteOptions{})
		}

		if kubeconfigHostingWorkName != "" {
			return r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).
				Delete(ctx, kubeconfigHostingWorkName, metav1.DeleteOptions{})
		}

		return nil
	}

	for _, workName := range hostingWorkNames {
		if err = r.clientHolder.WorkClient.WorkV1().ManifestWorks(hostingCluster).
			Delete(ctx, workName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

func (r *ReconcileResourceCleanup) orphanCleanup(ctx context.Context, clusterName string) error {
	var errs []error
	exists, err := r.namespaceExists(ctx, clusterName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	errs = appendIfErr(errs, helpers.ForceDeleteAllManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, clusterName, r.recorder))
	errs = appendIfErr(errs, r.forceDeleteManifestWorks(ctx, clusterName))
	errs = appendIfErr(errs, helpers.ForceDeleteWorkRoleBinding(ctx, r.clientHolder.KubeClient, clusterName, r.recorder))
	errs = appendIfErr(errs, r.deleteOrphanedKlusterlet(ctx, clusterName))
	return utilerrors.NewAggregate(errs)
}

// deleteOrphanedKlusterlet deletes the orphaned Klusterlet CR for the given cluster.
// This handles the case where ManifestWork uses DeleteOption: Orphan, leaving the Klusterlet CR
// without a DeletionTimestamp when the ManagedCluster is deleted before the work-agent finishes cleanup.
// For default/singleton mode, the klusterlet name is "klusterlet".
// For hosted mode, the klusterlet name is "klusterlet-{clusterName}".
func (r *ReconcileResourceCleanup) deleteOrphanedKlusterlet(ctx context.Context, clusterName string) error {
	// Try hosted mode klusterlet name first (klusterlet-{clusterName})
	hostedKlusterletName := fmt.Sprintf("%s-%s", constants.KlusterletSuffix, clusterName)
	klusterlet := &operatorv1.Klusterlet{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: hostedKlusterletName}, klusterlet)
	if err == nil {
		// Found the klusterlet, delete it
		log.Info(fmt.Sprintf("Deleting orphaned klusterlet %s for cluster %s", hostedKlusterletName, clusterName))
		if err := r.clientHolder.RuntimeClient.Delete(ctx, klusterlet); err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	// Try default mode klusterlet name ("klusterlet")
	defaultKlusterletName := constants.KlusterletSuffix
	err = r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: defaultKlusterletName}, klusterlet)
	if err == nil {
		// Verify the klusterlet belongs to this cluster
		if klusterlet.Spec.ClusterName == clusterName {
			log.Info(fmt.Sprintf("Deleting orphaned klusterlet %s for cluster %s", defaultKlusterletName, clusterName))
			if err := r.clientHolder.RuntimeClient.Delete(ctx, klusterlet); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *ReconcileResourceCleanup) forceCleanup(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	var errs []error
	exists, err := r.namespaceExists(ctx, cluster.Name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	errs = appendIfErr(errs, helpers.ForceDeleteAllManagedClusterAddons(ctx, r.clientHolder.RuntimeClient, cluster.Name, r.recorder))
	errs = appendIfErr(errs, r.forceDeleteManifestWorks(ctx, cluster.Name))

	// will not go to the cleanup process and go to forceCleanup directly when we delete an unavailable hosted cluster,
	// so need to delete the works in hosting cluster if there is no addon since the hosting addon is not force deleted.
	// but do not need to force delete the works in hosting cluster because we assume the hosting cluster is always available.
	hostingCluster, _ := helpers.GetHostingCluster(cluster)
	if helpers.IsHostedCluster(cluster) && hostingCluster != "" {
		if addons, err := helpers.ListManagedClusterAddons(ctx,
			r.clientHolder.RuntimeClient, cluster.Name); err != nil || len(addons.Items) != 0 {
			appendIfErr(errs, err)
			return utilerrors.NewAggregate(errs)
		}

		errs = appendIfErr(errs, r.deleteHostingManifestWorks(ctx, hostingCluster, cluster.Name))
	}

	errs = appendIfErr(errs, helpers.ForceDeleteWorkRoleBinding(ctx, r.clientHolder.KubeClient, cluster.Name, r.recorder))
	errs = appendIfErr(errs, r.deleteOrphanedKlusterlet(ctx, cluster.Name))

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
		crdWork := &works.Items[0]

		// If the ManifestWork doesn't have a DeletionTimestamp yet, trigger deletion and wait
		if crdWork.DeletionTimestamp.IsZero() {
			if err := r.clientHolder.WorkClient.WorkV1().ManifestWorks(cluster.Name).Delete(
				ctx, klusterletCRDWorkName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				return err
			}
			return nil // Wait for next reconcile
		}

		// ManifestWork is being deleted. Check if we should force-delete.
		// WorkDeleting=True means work-agent has STARTED processing, but the CRD content
		// may not be deleted yet. We should wait for work-agent to complete naturally.
		// Only force-delete after a timeout (30 seconds) to handle unresponsive work-agent.
		shouldForceDelete := false
		if time.Since(crdWork.DeletionTimestamp.Time) > 30*time.Second {
			log.Info(fmt.Sprintf("CRD manifestwork %s has been deleting for over 30s, force deleting",
				klusterletCRDWorkName))
			shouldForceDelete = true
		}

		if shouldForceDelete {
			if err = helpers.ForceDeleteManifestWork(ctx, r.clientHolder.WorkClient, r.recorder,
				cluster.Name, klusterletCRDWorkName); err != nil {
				return err
			}
		}
		// Otherwise, wait for work-agent to complete naturally
		return nil
	}

	hostingCluster, _ := helpers.GetHostingCluster(cluster)
	if !helpers.IsHostedCluster(cluster) || hostingCluster == "" {
		return nil
	}

	// delete works in hosting cluster after there is no works in hosted cluster ns
	if len(works.Items) > 0 {
		return nil
	}

	return r.deleteHostingManifestWorks(ctx, hostingCluster, cluster.Name)
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

	// Wait for Klusterlet CR to be fully deleted before considering cleanup complete.
	// This ensures klusterlet-operator has finished its cleanup (including CRD deletion).
	if exists, err := r.klusterletExists(ctx, cluster.Name); err != nil || exists {
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

func (r *ReconcileResourceCleanup) namespaceExists(ctx context.Context, name string) (bool, error) {
	_, err := r.clientHolder.KubeClient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// klusterletExists checks if any Klusterlet CR exists for the given cluster.
// For self-managed clusters, we need to wait for klusterlet-operator to finish cleanup.
func (r *ReconcileResourceCleanup) klusterletExists(ctx context.Context, clusterName string) (bool, error) {
	// Check hosted mode klusterlet name (klusterlet-{clusterName})
	hostedKlusterletName := fmt.Sprintf("%s-%s", constants.KlusterletSuffix, clusterName)
	klusterlet := &operatorv1.Klusterlet{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: hostedKlusterletName}, klusterlet)
	if err == nil {
		log.Info(fmt.Sprintf("Klusterlet %s still exists, waiting for cleanup to complete", hostedKlusterletName))
		return true, nil
	}
	if !errors.IsNotFound(err) {
		return false, err
	}

	// Check default mode klusterlet name ("klusterlet")
	defaultKlusterletName := constants.KlusterletSuffix
	err = r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: defaultKlusterletName}, klusterlet)
	if err == nil {
		// Verify the klusterlet belongs to this cluster
		if klusterlet.Spec.ClusterName == clusterName {
			log.Info(fmt.Sprintf("Klusterlet %s still exists for cluster %s, waiting for cleanup to complete",
				defaultKlusterletName, clusterName))
			return true, nil
		}
		return false, nil
	}
	if !errors.IsNotFound(err) {
		return false, err
	}

	return false, nil
}

func clusterNeedForceDelete(cluster *clusterv1.ManagedCluster) bool {
	// need to do force deletion when cluster is deleting but not accepted or not available
	if !cluster.Spec.HubAcceptsClient {
		return true
	}
	return helpers.IsClusterUnavailable(cluster)
}

func appendIfErr(errs []error, err error) []error {
	if err != nil {
		errs = append(errs, err)
	}
	return errs
}
