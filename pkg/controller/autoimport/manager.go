// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"strings"

	workv1 "open-cluster-management.io/api/work/v1"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const controllerName = "autoimport-controller"

// Add creates a new autoimport controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	mcRecorder kevents.EventRecorder) (string, error) {

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: NewReconcileAutoImport(
			clientHolder.RuntimeClient,
			clientHolder.KubeClient,
			informerHolder,
			helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
			mcRecorder,
		),
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return controllerName, err
	}

	// watch the import secrets
	if err := c.Watch(
		source.NewImportSecretSource(informerHolder.ImportSecretInformer,
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
		),
	); err != nil {
		return controllerName, err
	}

	// watch the auto-import secrets
	if err := c.Watch(
		source.NewAutoImportSecretSource(informerHolder.AutoImportSecretInformer,
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
		),
	); err != nil {
		return controllerName, err
	}

	// watch the managed clusters
	if err := c.Watch(
		source.NewManagedClusterSource(informerHolder.ManagedClusterInformer,
			&source.ManagedClusterResourceEventHandler{
				MapFunc: func(obj client.Object) reconcile.Request {
					return reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: obj.GetName(),
							Name:      obj.GetName(),
						},
					}
				},
			},
			predicate.Predicate(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				UpdateFunc: func(e event.UpdateEvent) bool {
					// handle the removal of the disable-auto-import annotation
					_, oldAutoImportDisabled := e.ObjectOld.GetAnnotations()[apiconstants.DisableAutoImportAnnotation]
					_, newAutoImportDisabled := e.ObjectNew.GetAnnotations()[apiconstants.DisableAutoImportAnnotation]
					return oldAutoImportDisabled && !newAutoImportDisabled
				},
			}),
		),
	); err != nil {
		return controllerName, err
	}

	// watch the klusterlet manifest works
	if err := c.Watch(
		source.NewKlusterletWorkSource(informerHolder.KlusterletWorkInformer,
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
		),
	); err != nil {
		return controllerName, err
	}

	return controllerName, nil
}
