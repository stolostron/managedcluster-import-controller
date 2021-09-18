// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"strings"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "importconfig-controller"

// Add creates a new importconfig controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder) (string, error) {
	return controllerName, add(mgr, newReconciler(mgr, clientHolder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, clientHolder *helpers.ClientHolder) reconcile.Reconciler {
	return &ReconcileImportConfig{
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
		&source.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				// only handle the labels changes for image registry
				return !equality.Semantic.DeepEqual(e.MetaOld.GetLabels(), e.MetaNew.GetLabels())
			},
		}),
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &rbacv1.ClusterRole{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &rbacv1.ClusterRoleBinding{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
	); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &corev1.ServiceAccount{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
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
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool {
				// only handle the import secret
				return strings.HasSuffix(e.Meta.GetName(), constants.ImportSecretNameSuffix)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				secretName := e.MetaNew.GetName()
				// only handle the import secret data chanages
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
