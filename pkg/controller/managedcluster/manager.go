// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "managedcluster-controller"

// Add creates a new managedcluster controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder, _ *source.InformerHolder) (string, error) {
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: &ReconcileManagedCluster{
			client:   clientHolder.RuntimeClient,
			recorder: helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
		},
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return controllerName, err
	}

	if err := c.Watch(
		&runtimesource.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				// only handle the finalizers/labels/annotations changes
				return !equality.Semantic.DeepEqual(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers()) ||
					!equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels()) ||
					!equality.Semantic.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
			},
		}),
	); err != nil {
		return controllerName, err
	}

	if err := c.Watch(
		&runtimesource.Kind{Type: &corev1.Namespace{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				// only handle the labels chanages
				return !equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels())
			},
		}),
	); err != nil {
		return controllerName, err
	}

	return controllerName, nil
}
