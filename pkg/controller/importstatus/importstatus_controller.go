// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importstatus

import (
	"context"
	"fmt"

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

var log = logf.Log.WithName(controllerName)

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
