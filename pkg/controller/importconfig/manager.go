// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kevents "k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

const controllerName = "importconfig-controller"

// Add creates a new importconfig controller and adds it to the Manager.
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
		Watches(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool {
					// handle the labels changes for image registry
					// handle the annotations changes for node placement and klusterletconfig
					// handle the claim changes for priority class
					return !equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels()) ||
						!equality.Semantic.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations()) ||
						helpers.IsKubeVersionChanged(e.ObjectOld, e.ObjectNew)
				},
			}),
		).
		Watches(
			&rbacv1.ClusterRole{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&clusterv1.ManagedCluster{},
				handler.OnlyControllerOwner(),
			),
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			}),
		).
		Watches(
			&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&clusterv1.ManagedCluster{},
				handler.OnlyControllerOwner(),
			),
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			}),
		).
		Watches(
			&corev1.ServiceAccount{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&clusterv1.ManagedCluster{},
				handler.OnlyControllerOwner(),
			),
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			}),
		).
		Watches(
			&klusterletconfigv1alpha1.KlusterletConfig{},
			&enqueueManagedClusterInKlusterletConfigAnnotation{
				managedclusterIndexer: informerHolder.ManagedClusterInformer.GetIndexer(),
			},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return true },
				CreateFunc:  func(e event.CreateEvent) bool { return true },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			}),
		).
		WatchesRawSource(
			source.NewImportSecretSource(informerHolder.ImportSecretInformer),
			&source.ManagedClusterResourceEventHandler{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			}),
		).
		Complete(&ReconcileImportConfig{
			clientHolder:           clientHolder,
			klusterletconfigLister: informerHolder.KlusterletConfigLister,
			scheme:                 mgr.GetScheme(),
			recorder:               helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
		})
	return controllerName, err
}
