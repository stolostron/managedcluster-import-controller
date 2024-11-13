// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kevents "k8s.io/client-go/tools/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const ControllerName = "manifestwork-controller"

// Add creates a new manifestwork controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	mcRecorder kevents.EventRecorder) error {

	err := ctrl.NewControllerManagedBy(mgr).Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
		}).
		WatchesRawSource(
			source.NewKlusterletWorkSource(informerHolder.KlusterletWorkInformer,
				&source.ManagedClusterResourceEventHandler{},
				predicate.Predicate(predicate.Funcs{
					GenericFunc: func(e event.GenericEvent) bool { return false },
					CreateFunc:  func(e event.CreateEvent) bool { return true },
					DeleteFunc:  func(e event.DeleteEvent) bool { return true },
					UpdateFunc: func(e event.UpdateEvent) bool {
						workName := e.ObjectNew.GetName()
						// for update event, only watch klusterlet manifest works
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
				})),
		).
		Watches(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return isDefaultModeObject(e.Object) },
				DeleteFunc:  func(e event.DeleteEvent) bool { return isDefaultModeObject(e.Object) },
				CreateFunc:  func(e event.CreateEvent) bool { return isDefaultModeObject(e.Object) },
				UpdateFunc:  func(e event.UpdateEvent) bool { return isDefaultModeObject(e.ObjectNew) },
			}),
		).
		WatchesRawSource(
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
				})),
		).
		Complete(NewReconcileManifestWork(
			clientHolder,
			informerHolder,
			mgr.GetScheme(),
			helpers.NewEventRecorder(clientHolder.KubeClient, ControllerName),
			mcRecorder,
		))
	return err
}

func isDefaultModeObject(object client.Object) bool {
	return !strings.EqualFold(object.GetAnnotations()[constants.KlusterletDeployModeAnnotation], string(operatorv1.InstallModeHosted))
}
