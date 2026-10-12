package resourcecleanup

import (
	"context"

	clustercontroller "github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const ControllerName = "resourcecleanup-controller"

// fromNamespaceWatch marks reconcile requests enqueued by the cluster namespace
// watch. ManagedCluster requests leave Namespace empty. Reconcile uses this to
// confirm the cluster with the live API reader before orphan cleanup; a cache
// miss on that watch is not proof the cluster was deleted.
const fromNamespaceWatch = "from-namespace-watch"

// Add creates resource cleanup controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	mcRecorder kevents.EventRecorder) error {

	err := ctrl.NewControllerManagedBy(mgr).Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
		}).
		Watches(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return !e.Object.GetDeletionTimestamp().IsZero() },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				CreateFunc:  func(e event.CreateEvent) bool { return !e.Object.GetDeletionTimestamp().IsZero() },
				UpdateFunc: func(e event.UpdateEvent) bool {
					// prevent losing the event when cluster is deleting
					return !e.ObjectNew.GetDeletionTimestamp().IsZero()
				},
			}),
		).
		// Existing cluster namespaces are replayed as create events when this controller
		// starts, including after a restart or leader change. That resumes orphan cleanup
		// once the ManagedCluster is already gone and the grace-period requeue was lost.
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(enqueueOrphanedClusterNamespace(clientHolder.RuntimeAPIReader)),
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(event.GenericEvent) bool { return false },
				DeleteFunc:  func(event.DeleteEvent) bool { return false },
				UpdateFunc:  func(event.UpdateEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return isManagedClusterNamespace(e.Object) },
			}),
		).
		Complete(NewReconcileResourceCleanup(
			clientHolder,
			helpers.NewEventRecorder(clientHolder.KubeClient, ControllerName),
			mcRecorder,
		))

	return err
}

// isManagedClusterNamespace reports whether the namespace belongs to a managed cluster.
// Either label is enough; both are applied to cluster namespaces.
func isManagedClusterNamespace(obj client.Object) bool {
	if obj == nil {
		return false
	}
	labels := obj.GetLabels()
	if labels[clusterv1.ClusterNameLabelKey] != "" {
		return true
	}
	_, ok := labels[clustercontroller.ClusterLabel]
	return ok
}

// enqueueOrphanedClusterNamespace queues cleanup for a cluster namespace whose
// ManagedCluster is already gone. c must be the live API reader: the cache can
// miss a cluster that was just created. Namespaces for clusters that still
// exist are skipped; those clusters are reconciled from the ManagedCluster watch.
func enqueueOrphanedClusterNamespace(c client.Reader) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		cluster := &clusterv1.ManagedCluster{}
		err := c.Get(ctx, types.NamespacedName{Name: obj.GetName()}, cluster)
		if err == nil {
			return nil
		}
		// NotFound starts orphan cleanup. Any other lookup error is retried by reconcile,
		// which confirms against the live reader again before deleting anything.
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Namespace: fromNamespaceWatch,
				Name:      obj.GetName(),
			},
		}}
	}
}
