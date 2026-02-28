// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kevents "k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

const controllerName = "managedcluster-controller"

// Add creates a new managedcluster controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	_ *source.InformerHolder,
	mcRecorder kevents.EventRecorder) (string, error) {

	err := ctrl.NewControllerManagedBy(mgr).Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
		}).
		Watches(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				CreateFunc:  func(e event.CreateEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool {
					// prevent losing the event when cluster is deleting
					if !e.ObjectNew.GetDeletionTimestamp().IsZero() {
						return true
					}
					// only handle the finalizers/labels/annotations changes
					return !equality.Semantic.DeepEqual(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers()) ||
						!equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels()) ||
						!equality.Semantic.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
				},
			}),
		).
		Watches(
			&corev1.Namespace{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool {
					// only handle the labels chanages
					return !equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels())
				},
			}),
		).
		Complete(NewReconcileManagedCluster(
			clientHolder.RuntimeClient,
			helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
			mcRecorder,
		))

	return controllerName, err
}
