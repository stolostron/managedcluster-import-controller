package flightctl

import (
	"context"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func AddManagedClusterController(ctx context.Context, mgr manager.Manager, flightctlManager *FlightCtlManager, clientHolder *helpers.ClientHolder) error {
	return ctrl.NewControllerManagedBy(mgr).Named(ManagedClusterControllerName).
		Watches(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				UpdateFunc: func(e event.UpdateEvent) bool {
					return !e.ObjectNew.(*clusterv1.ManagedCluster).Spec.HubAcceptsClient
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return !e.Object.(*clusterv1.ManagedCluster).Spec.HubAcceptsClient
				},
			})).
		Complete(&ManagedClusterReconciler{
			clientHolder:                     clientHolder,
			isManagedClusterAFlightctlDevice: flightctlManager.IsManagedClusterAFlightctlDevice,
			recorder:                         helpers.NewEventRecorder(clientHolder.KubeClient, ManagedClusterControllerName),
		})
}
