// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package selfmanagedcluster

import (
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "selfmanagedcluster-controller"

// Add creates a new self managed cluster controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder, informerHolder *source.InformerHolder) (string, error) {
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: &ReconcileLocalCluster{
			clientHolder:   clientHolder,
			restMapper:     mgr.GetRESTMapper(),
			informerHolder: informerHolder,
			recorder:       helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
		},
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return controllerName, err
	}

	// watch the import-secret
	if err := c.Watch(
		source.NewImportSecretSource(informerHolder.ImportSecretInformer),
		&source.ManagedClusterResourceEventHandler{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
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
		return controllerName, err
	}

	// watch the managed cluster
	if err := c.Watch(
		&runtimesource.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
				// only handle the label changed and new self managed label is true
				newLabels := e.ObjectNew.GetLabels()
				return !equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), newLabels) &&
					strings.EqualFold(newLabels[constants.SelfManagedLabel], "true")
			},
		}),
	); err != nil {
		return controllerName, err
	}

	// watch the klusterlet manifest works
	if err := c.Watch(
		source.NewKlusterletWorkSource(informerHolder.KlusterletWorkInformer),
		&source.ManagedClusterResourceEventHandler{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				workName := e.Object.GetName()
				// only watch klusterlet manifest works
				if !strings.HasSuffix(workName, constants.KlusterletCRDsSuffix) &&
					!strings.HasSuffix(workName, constants.KlusterletSuffix) {
					return false
				}

				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				workName := e.ObjectNew.GetName()
				// only watch klusterlet manifest works
				if !strings.HasSuffix(workName, constants.KlusterletCRDsSuffix) &&
					!strings.HasSuffix(workName, constants.KlusterletSuffix) {
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
		return controllerName, err
	}

	return controllerName, nil
}
