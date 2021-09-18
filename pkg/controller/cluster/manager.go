// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "managedcluster-controller"

// Add creates a new managedcluster controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder) (string, error) {
	return controllerName, add(mgr, newReconciler(mgr, clientHolder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, clientHolder *helpers.ClientHolder) reconcile.Reconciler {
	return &ReconcileManagedCluster{
		client:   clientHolder.RuntimeClient,
		scheme:   mgr.GetScheme(),
		recorder: helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
	}
}

// adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				// TODO: changed this to e.ObjectOld/e.ObjectNew after update the controller-runtime
				objectOld := e.MetaOld
				objectNew := e.MetaNew

				// only handle the finalizers/labels/annotations changes
				return !equality.Semantic.DeepEqual(objectOld.GetFinalizers(), objectNew.GetFinalizers()) ||
					!equality.Semantic.DeepEqual(objectOld.GetLabels(), objectNew.GetLabels()) ||
					!equality.Semantic.DeepEqual(objectOld.GetAnnotations(), objectNew.GetAnnotations())
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &corev1.Namespace{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				// TODO: changed this to e.ObjectOld/e.ObjectNew after update the controller-runtime
				objectOld := e.MetaOld
				objectNew := e.MetaNew

				// only handle the labels chanages
				return !equality.Semantic.DeepEqual(objectOld.GetLabels(), objectNew.GetLabels())
			},
		}),
	); err != nil {
		return err
	}

	return nil
}
