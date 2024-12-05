package flightctl

import (
	"context"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const ManagedClusterControllerName = "flightctl-managedcluster-controller"

// ManagedClusterReconciler is responsible to set hubAcceptsClient to true if the managed cluster is a flightctl device.
type ManagedClusterReconciler struct {
	clientHolder                     *helpers.ClientHolder
	recorder                         events.Recorder
	isManagedClusterAFlightctlDevice func(ctx context.Context, managedClusterName string) (bool, error)
}

var _ reconcile.Reconciler = &ManagedClusterReconciler{}

func (r *ManagedClusterReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	cluster := &clusterv1.ManagedCluster{}
	if err := r.clientHolder.RuntimeClient.Get(ctx, request.NamespacedName, cluster); err != nil {
		return reconcile.Result{}, err
	}

	if cluster.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	if cluster.Spec.HubAcceptsClient {
		return reconcile.Result{}, nil
	}

	isDevice, err := r.isManagedClusterAFlightctlDevice(ctx, cluster.Name)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !isDevice {
		return reconcile.Result{}, nil
	}

	cluster.Spec.HubAcceptsClient = true
	if err := r.clientHolder.RuntimeClient.Update(ctx, cluster); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
