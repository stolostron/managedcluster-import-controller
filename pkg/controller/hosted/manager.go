// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hosted

import (
	"context"
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const controllerName = "hosted-manifestwork-controller"

// Add creates a new manifestwork controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager, clientHolder *helpers.ClientHolder, informerHolder *source.InformerHolder) (string, error) {

	err := ctrl.NewControllerManagedBy(mgr).Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
		}).
		WatchesRawSource(
			source.NewHostedWorkSource(informerHolder.HostedWorkInformer),
			&source.ManagedClusterResourceEventHandler{
				MapFunc: func(o client.Object) reconcile.Request {
					managedClusterName := o.GetNamespace()
					workName := o.GetName()
					if strings.HasSuffix(workName, constants.HostedKlusterletManifestworkSuffix) {
						managedClusterName = strings.TrimSuffix(workName, "-"+constants.HostedKlusterletManifestworkSuffix)
					}
					if strings.HasSuffix(workName, constants.HostedManagedKubeconfigManifestworkSuffix) {
						managedClusterName = strings.TrimSuffix(workName, "-"+constants.HostedManagedKubeconfigManifestworkSuffix)
					}
					return reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: managedClusterName,
							Name:      managedClusterName,
						},
					}
				},
			},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return true },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool {
					workName := e.ObjectNew.GetName()
					// for update event, only watch hosted mode manifest works
					if !strings.HasSuffix(workName, constants.HostedKlusterletManifestworkSuffix) &&
						!strings.HasSuffix(workName, constants.HostedManagedKubeconfigManifestworkSuffix) {
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
		).
		Watches(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return isHostedModeObject(e.Object) },
				DeleteFunc:  func(e event.DeleteEvent) bool { return isHostedModeObject(e.Object) },
				CreateFunc:  func(e event.CreateEvent) bool { return isHostedModeObject(e.Object) },
				UpdateFunc:  func(e event.UpdateEvent) bool { return isHostedModeObject(e.ObjectNew) },
			}),
		).
		WatchesRawSource(
			source.NewImportSecretSource(informerHolder.ImportSecretInformer),
			&source.ManagedClusterResourceEventHandler{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				CreateFunc: func(e event.CreateEvent) bool {
					// only handle the hosted mode import secret
					return isHostedModeObject(e.Object)
				},
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
		WatchesRawSource( // watch the auto-import secrets
			source.NewAutoImportSecretSource(informerHolder.AutoImportSecretInformer),
			&source.ManagedClusterResourceEventHandler{},
			builder.WithPredicates(predicate.Funcs{
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
			helpers.NewManagedClusterEventRecorder(ctx, clientHolder.KubeClient, controllerName),
		))
	return controllerName, err
}

func isHostedModeObject(object client.Object) bool {
	return strings.EqualFold(object.GetAnnotations()[constants.KlusterletDeployModeAnnotation], string(operatorv1.InstallModeHosted))
}
