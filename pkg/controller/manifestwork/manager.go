// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "manifestwork-controller"

const (
	klusterletSuffix     = "klusterlet"
	klusterletCRDsSuffix = "klusterlet-crds"
)

// Add creates a new manifestwork controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder,
	importSecretInformer, autoImportSecretInformer cache.SharedIndexInformer) (string, error) {
	return controllerName, add(importSecretInformer, mgr, newReconciler(mgr, clientHolder))
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
func add(importSecretInformer cache.SharedIndexInformer, mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return err
	}

	if err := c.Watch(
		&runtimesource.Kind{Type: &workv1.ManifestWork{}},
		handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: o.GetNamespace(),
					},
				},
			}
		}),
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				workName := e.ObjectNew.GetName()
				// for update event, only watch klusterlet manifest works
				if !strings.HasSuffix(workName, klusterletCRDsSuffix) &&
					!strings.HasSuffix(workName, klusterletSuffix) {
					return false
				}

				new, okNew := e.ObjectNew.(*workv1.ManifestWork)
				old, okOld := e.ObjectOld.(*workv1.ManifestWork)
				if okNew && okOld {
					return !helpers.ManifestsEqual(new.Spec.Workload.Manifests, old.Spec.Workload.Manifests) ||
						!equality.Semantic.DeepEqual(new.Status, old.Status)
				}

				return false
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&runtimesource.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return isDefaultModeObject(e.Object) },
			DeleteFunc:  func(e event.DeleteEvent) bool { return isDefaultModeObject(e.Object) },
			CreateFunc:  func(e event.CreateEvent) bool { return isDefaultModeObject(e.Object) },
			UpdateFunc:  func(e event.UpdateEvent) bool { return isDefaultModeObject(e.ObjectNew) },
		})); err != nil {
		return err
	}

	if err := c.Watch(
		source.NewImportSecretSource(importSecretInformer),
		&source.ManagedClusterSecretEventHandler{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
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

	if err := c.Watch(
		&runtimesource.Kind{Type: &addonv1alpha1.ManagedClusterAddOn{}},
		handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: o.GetNamespace(),
					},
				},
			}
		}),
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			UpdateFunc:  func(e event.UpdateEvent) bool { return false },
		})); err != nil {
		return err
	}

	return nil
}

func isDefaultModeObject(object client.Object) bool {
	return !strings.EqualFold(object.GetAnnotations()[constants.KlusterletDeployModeAnnotation], constants.KlusterletDeployModeHosted)
}
