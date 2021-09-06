// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncset

import (
	"context"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/library-go/pkg/operator/events"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(controllerName)

// ReconcileSyncSet removes the klusterlet syncsets as we are now using manifestworks
type ReconcileSyncSet struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	recorder events.Recorder
}

// Reconcile SyncSets to remove the klusterlet syncsets as we are now using manifestworks.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
//func (r *ReconcileSyncSet) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
func (r *ReconcileSyncSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling syncsets")

	ctx := context.TODO()
	managedClusterName := request.Namespace

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		// the manged cluster could be deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	existing := &hivev1.SyncSet{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: managedClusterName, Name: request.Name}, existing)
	if errors.IsNotFound(err) {
		// the syncset could be deleted, do nothing
		return reconcile.Result{}, nil
	}

	if err != nil {
		return reconcile.Result{}, err
	}

	if existing.Spec.ResourceApplyMode != hivev1.UpsertResourceApplyMode {
		existing.Spec.ResourceApplyMode = hivev1.UpsertResourceApplyMode
		if err := r.client.Update(context.TODO(), existing); err != nil {
			return reconcile.Result{}, err
		}

		r.recorder.Eventf("KlusterletSyncSetUpdated", "Updated syncset %s/%s to upsert mode", request.Namespace, request.Name)
		return reconcile.Result{}, nil
	}

	err = r.client.Delete(ctx, existing)
	if err != nil {
		return reconcile.Result{}, err
	}

	r.recorder.Eventf("KlusterletSyncSetDeleted", "Syncset %s/%s is deleted", request.Namespace, request.Name)
	return reconcile.Result{}, nil
}
