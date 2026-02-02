// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hosted

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

const controllerName = "hosted-manifestwork-controller"

// Add creates a new manifestwork controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	mcRecorder kevents.EventRecorder) (string, error) {

	err := ctrl.NewControllerManagedBy(mgr).Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
		}).
		WatchesRawSource(
			source.NewHostedWorkSource(informerHolder.HostedWorkInformer,
				&source.ManagedClusterResourceEventHandler{
					MapFunc: func(o client.Object) reconcile.Request {
						return reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: o.GetNamespace(),
								Name:      o.GetNamespace(),
							},
						}
					},
				},
				predicate.Funcs{
					GenericFunc: func(e event.GenericEvent) bool { return false },
					DeleteFunc: func(e event.DeleteEvent) bool {
						workName := e.Object.GetName()
						// only watch hosted manifest works
						if !strings.HasSuffix(workName, constants.HostedKlusterletManifestworkSuffix) {
							return false
						}
						return true
					},
					CreateFunc: func(e event.CreateEvent) bool {
						workName := e.Object.GetName()
						// only watch hosted manifest works
						if !strings.HasSuffix(workName, constants.HostedKlusterletManifestworkSuffix) {
							return false
						}
						return true
					},
					UpdateFunc: func(e event.UpdateEvent) bool {
						workName := e.ObjectNew.GetName()
						// only watch hosted manifest works
						if !strings.HasSuffix(workName, constants.HostedKlusterletManifestworkSuffix) {
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
		).
		Watches(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				UpdateFunc: func(e event.UpdateEvent) bool {
					// handle the change of the hosted annotation
					_, oldHosted := e.ObjectOld.GetAnnotations()[constants.KlusterletDeployModeAnnotation]
					_, newHosted := e.ObjectNew.GetAnnotations()[constants.KlusterletDeployModeAnnotation]
					if oldHosted != newHosted {
						return true
					}

					// handle the change of the klusterletconfig annotation
					oldKlusterletConfig := e.ObjectOld.GetAnnotations()[apiconstants.AnnotationKlusterletConfig]
					newKlusterletConfig := e.ObjectNew.GetAnnotations()[apiconstants.AnnotationKlusterletConfig]
					return oldKlusterletConfig != newKlusterletConfig
				},
			}),
		).
		WatchesRawSource(
			source.NewImportSecretSource(informerHolder.ImportSecretInformer,
				&source.ManagedClusterResourceEventHandler{},
				predicate.Funcs{
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
		).
		WatchesRawSource(
			source.NewAutoImportSecretSource(informerHolder.AutoImportSecretInformer,
				&source.ManagedClusterResourceEventHandler{},
				predicate.Funcs{
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
		).
		Complete(NewReconcileHosted(
			clientHolder,
			informerHolder,
			mgr.GetScheme(),
			helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
			mcRecorder,
		))
	return controllerName, err
}
