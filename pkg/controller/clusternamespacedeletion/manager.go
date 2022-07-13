// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusternamespacedeletion

import (
	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	clustercontroller "github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "clusternamespacedeletion-controller"

// Add creates a new managedcluster controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder,
	importSecretInformer, autoImportSecretInformer cache.SharedIndexInformer) (string, error) {
	return controllerName, add(mgr, newReconciler(clientHolder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(clientHolder *helpers.ClientHolder) reconcile.Reconciler {
	return &ReconcileClusterNamespaceDeletion{
		client:   clientHolder.RuntimeClient,
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
		handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: o.GetName(),
					},
				},
			}
		}),
		// only cares cluster be deleted
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return !e.ObjectNew.GetDeletionTimestamp().IsZero()
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &corev1.Namespace{}},
		handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: o.GetName(),
					},
				},
			}
		}),
		// only cares cluster namespace
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				labels := e.Object.GetLabels()
				if len(labels) == 0 {
					return false
				}
				if _, ok := labels[clustercontroller.ClusterLabel]; ok {
					return true
				}

				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				labels := e.ObjectNew.GetLabels()
				if len(labels) == 0 {
					return false
				}
				if _, ok := labels[clustercontroller.ClusterLabel]; ok {
					return true
				}

				return false
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &hivev1.ClusterDeployment{}},
		handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: o.GetNamespace(),
					},
				},
			}
		}),
		// only cares deletion
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &addonv1alpha1.ManagedClusterAddOn{}},
		handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: o.GetNamespace(),
					},
				},
			}
		}),
		// only cares deletion
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &asv1beta1.InfraEnv{}},
		handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: o.GetNamespace(),
					},
				},
			}
		}),
		// only cares deletion
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
		}),
	); err != nil {
		return err
	}

	return nil
}
