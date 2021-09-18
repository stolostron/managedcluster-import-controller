// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"strings"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "manifestwork-controller"

const (
	klusterletSuffix     = "klusterlet"
	klusterletCRDsSuffix = "klusterlet-crds"
)

// Add creates a new manifestwork controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder) (string, error) {
	return controllerName, add(mgr, newReconciler(mgr, clientHolder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, clientHolder *helpers.ClientHolder) reconcile.Reconciler {
	return &ReconcileManifestWork{
		clientHolder: clientHolder,
		scheme:       mgr.GetScheme(),
		recorder:     helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
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
		&source.Kind{Type: &workv1.ManifestWork{}},
		// handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		// 	return []reconcile.Request{
		// 		{
		// 			NamespacedName: types.NamespacedName{
		// 				Name: o.GetNamespace(),
		// 			},
		// 		},
		// 	}
		// }),
		nil,
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				workName := e.MetaNew.GetName()
				// for update event, only watch klusterlet manifest works
				if !strings.HasSuffix(workName, klusterletCRDsSuffix) &&
					!strings.HasSuffix(workName, klusterletSuffix) {
					return false
				}

				new, okNew := e.ObjectNew.(*workv1.ManifestWork)
				old, okOld := e.ObjectOld.(*workv1.ManifestWork)
				if okNew && okOld {
					return !helpers.ManifestsEqual(new.Spec.Workload.Manifests, old.Spec.Workload.Manifests)
				}

				return false
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &corev1.Secret{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
				secretName := e.MetaNew.GetName()
				// only handles the import secret changes
				if !strings.HasSuffix(secretName, constants.ImportSecretNameSuffix) {
					return false
				}

				new, okNew := e.ObjectNew.(*corev1.Secret)
				old, okOld := e.ObjectOld.(*corev1.Secret)
				if okNew && okOld {
					return !equality.Semantic.DeepEqual(old.Data, new.Data)
				}

				return false
			},
		}),
	); err != nil {
		return err
	}

	return nil
}
