// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importstatus

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
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
	recorder   events.Recorder
}

// blank assignment to verify that ReconcileImportStatus implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImportStatus{}

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

	if helpers.DetermineKlusterletMode(managedCluster) != constants.KlusterletDeployModeDefault {
		return reconcile.Result{}, nil
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	existedCondition := meta.FindStatusCondition(
		managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if existedCondition == nil || existedCondition.Status != metav1.ConditionFalse ||
		existedCondition.Reason != constants.ConditionReasonManagedClusterImporting {
		// Wait for autoimport/clusterdeployment/selfmanaged controller to apply importing resources first
		return reconcile.Result{}, nil
	}

	reqLogger.V(5).Info("Reconciling the klusterlet manifest works to set condition of the managed cluster")

	condition, err := r.checkManifestWorksAvailability(ctx, managedClusterName)
	reqLogger.V(5).Info("Check manifest works availability", "condition", condition, "error", err)
	if err := helpers.UpdateManagedClusterStatus(
		r.client,
		r.recorder,
		managedClusterName,
		condition,
	); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, err
}

func (r *ReconcileImportStatus) checkManifestWorksAvailability(ctx context.Context, managedClusterName string) (
	metav1.Condition, error) {

	available, err := helpers.IsManifestWorksAvailable(ctx, r.workClient, managedClusterName,
		fmt.Sprintf("%s-%s", managedClusterName, constants.KlusterletCRDsSuffix),
		fmt.Sprintf("%s-%s", managedClusterName, constants.KlusterletSuffix))
	if err != nil {
		return helpers.NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImporting,
			fmt.Sprintf("Check klusterlet manifestworks availability failed: %v", err),
		), err
	}
	if !available {
		return helpers.NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImporting,
			"Wait for the klusterlet manifestworks to be available",
		), nil
	}

	return helpers.NewManagedClusterImportSucceededCondition(
		metav1.ConditionTrue,
		constants.ConditionReasonManagedClusterImported,
		"Import succeeded",
	), nil
}
