// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importstatus

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kevents "k8s.io/client-go/tools/events"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
)

var log = logf.Log.WithName(ControllerName)

// staleImportFreshnessRequeue is how often the import-status controller re-checks a pending
// stale-import re-import for agent freshness. The controller watches ManagedClusters and
// ManifestWorks but not the managed-cluster lease, so a later lease renewal alone would not
// re-trigger the gate; this bounded requeue guarantees the pending re-import converges.
const staleImportFreshnessRequeue = 30 * time.Second

// ReconcileImportStatus reconciles the klusterlet manifestworks to judge whether the cluster is imported successfully
type ReconcileImportStatus struct {
	client     client.Client
	kubeClient kubernetes.Interface
	workClient workclient.Interface
	mcRecorder kevents.EventRecorder
}

// blank assignment to verify that ReconcileImportStatus implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImportStatus{}

func NewReconcileImportStatus(
	client client.Client,
	kubeClient kubernetes.Interface,
	workClient workclient.Interface,
	mcRecorder kevents.EventRecorder,
) *ReconcileImportStatus {
	return &ReconcileImportStatus{
		client:     client,
		kubeClient: kubeClient,
		workClient: workClient,
		mcRecorder: mcRecorder,
	}
}

// Reconcile sets the manged cluster import condition according to the klusterlet manifestwork status
func (r *ReconcileImportStatus) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	managedClusterName := request.Name
	managedCluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if helpers.DetermineKlusterletMode(managedCluster) == operatorv1.InstallModeHosted {
		return reconcile.Result{}, nil
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	reqLogger.V(5).Info("Reconciling managedcluster", "managedCluster", managedClusterName)

	// This controller will only add/update the ImportSucceededCondition condition in following 2 cases:
	// - Add the condition when it does not exist
	// - Set the condition status to True when manifestworks are available
	//
	// Will NOT change the condition in other situation, otherwise there will be a changing loop on the
	// condition with other controllers
	existedCondition := meta.FindStatusCondition(
		managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if existedCondition == nil {
		reqLogger.V(5).Info("Add ImportSucceededCondition with WaitForImporting reason")
		return reconcile.Result{Requeue: true}, helpers.UpdateManagedClusterImportCondition(
			r.client,
			managedCluster,
			helpers.NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterWaitForImporting,
				"Wait for importing",
			),
			r.mcRecorder,
		)
	}

	available, err := helpers.IsManifestWorksAvailable(ctx, r.workClient, managedClusterName,
		// only check if the klusterlet is available, check the klusterlet-crd will break the no-operator mode
		fmt.Sprintf("%s-%s", managedClusterName, constants.KlusterletSuffix))
	if err != nil {
		reqLogger.V(5).Info("Check klusterlet manifestworks availability failed", "error", err)
		return reconcile.Result{}, err
	}
	if !available {
		reqLogger.V(5).Info("Klusterlet manifestworks are not available")
		return reconcile.Result{}, nil
	}

	// When a stale-import-success re-import is in progress (ACM-49455), the klusterlet
	// ManifestWork can still report Available=True from the previous agent incarnation: that
	// condition is set by the spoke work agent and is never reset when the old agent is wiped.
	// Trusting it would complete the re-import against a dead spoke. Require a live agent to have
	// proven itself since the re-import request (a lease renewal at/after the request, or the
	// managed cluster becoming available at/after it) before completing.
	reference, reImporting := helpers.StaleImportReference(managedCluster)
	if reImporting {
		lease, err := r.kubeClient.CoordinationV1().Leases(managedClusterName).Get(
			ctx, helpers.ManagedClusterLeaseName, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		if errors.IsNotFound(err) {
			lease = nil
		}
		if !helpers.ClusterAgentIsFresh(managedCluster, lease, reference) {
			reqLogger.Info("Re-import in progress, waiting for a live agent to prove freshness",
				"managedCluster", managedClusterName, "staleImportReference", reference)
			// Requeue: a subsequent lease renewal alone does not generate a watched event, so
			// poll the freshness signal until the agent proves itself (or a watched event fires).
			return reconcile.Result{RequeueAfter: staleImportFreshnessRequeue}, nil
		}
	}

	if helpers.IsImmediateImport(managedCluster.Annotations) {
		// update the ImmediateImport annotation value to `Completed`
		if err := helpers.SetImmediateImportCompleted(r.client, managedCluster.Name); err != nil {
			return reconcile.Result{}, err
		}
	}

	// The re-import has genuinely completed (the cluster is available again), so clear the
	// re-import stamp. Leaving it would make the freshness gate above block a later, unrelated
	// availability blip from being re-affirmed as imported. Clear only the stamp this reconcile
	// evaluated, so a newer request armed concurrently keeps its gate.
	if reImporting {
		if err := helpers.ClearStaleImportReference(r.client, managedCluster.Name, reference); err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.V(5).Info("Klusterlet manifestworks are available")
	return reconcile.Result{}, helpers.UpdateManagedClusterImportCondition(
		r.client,
		managedCluster,
		helpers.NewManagedClusterImportSucceededCondition(
			metav1.ConditionTrue,
			constants.ConditionReasonManagedClusterImported,
			"Import succeeded",
		),
		r.mcRecorder,
	)
}
