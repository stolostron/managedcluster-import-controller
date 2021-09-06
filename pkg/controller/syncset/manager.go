// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncset

import (
	"strings"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "syncset-controller"

const (
	klusterletSyncsetPostfix     = "klusterlet"
	klusterletCRDsSyncsetPostfix = "klusterlet-crds"
)

// Add creates a new importconfig controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder) (string, error) {
	return controllerName, add(mgr, newReconciler(clientHolder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(clientHolder *helpers.ClientHolder) reconcile.Reconciler {
	return &ReconcileSyncSet{
		client:   clientHolder.RuntimeClient,
		recorder: helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
	}
}

// adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &hivev1.SyncSet{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				if strings.HasSuffix(e.Meta.GetName(), klusterletCRDsSyncsetPostfix) ||
					strings.HasSuffix(e.Meta.GetName(), klusterletSyncsetPostfix) {
					return true
				}

				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				name := e.MetaNew.GetName()
				if !strings.HasSuffix(name, klusterletCRDsSyncsetPostfix) &&
					!strings.HasSuffix(name, klusterletSyncsetPostfix) {
					return false
				}

				old, okOld := e.ObjectOld.(*hivev1.SyncSet)
				new, okNew := e.ObjectNew.(*hivev1.SyncSet)
				if okOld && okNew {
					return !equality.Semantic.DeepEqual(old.Spec, new.Spec)
				}

				return false
			},
		}),
	); err != nil {
		return err
	}

	return nil
}
