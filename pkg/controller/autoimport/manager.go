// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"strings"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"

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

const controllerName = "autoimport-controller"

// Add creates a new autoimport controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder) (string, error) {
	return controllerName, add(mgr, newReconciler(clientHolder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(clientHolder *helpers.ClientHolder) reconcile.Reconciler {
	return &ReconcileAutoImport{
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
		&source.Kind{Type: &corev1.Secret{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				secretName := e.Meta.GetName()
				// only handle the auto import secret and managed cluster import secret
				if secretName == constants.AutoImportSecretName ||
					strings.HasSuffix(secretName, constants.ImportSecretNameSuffix) {
					return true
				}

				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				secretName := e.MetaNew.GetName()
				// only handle the auto import secret and managed cluster import secret
				if secretName != constants.AutoImportSecretName &&
					!strings.HasSuffix(secretName, constants.ImportSecretNameSuffix) {
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
