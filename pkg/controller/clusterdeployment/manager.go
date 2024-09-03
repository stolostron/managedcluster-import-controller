// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusterdeployment

import (
	"context"
	"strings"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
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
)

const controllerName = "clusterdeployment-controller"

// Add creates a new managedcluster controller and adds it to the Manager.
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
		Watches( // watch the clusterdeployment
			&hivev1.ClusterDeployment{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: o.GetNamespace(),
							Name:      o.GetNamespace(),
						},
					},
				}
			}),
		).
		Watches( // watch the managed cluster
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(
				predicate.Funcs{
					GenericFunc: func(e event.GenericEvent) bool { return false },
					DeleteFunc:  func(e event.DeleteEvent) bool { return false },
					CreateFunc:  func(e event.CreateEvent) bool { return false },
					UpdateFunc: func(e event.UpdateEvent) bool {
						oldAnnotations := e.ObjectOld.GetAnnotations()
						newAnnotations := e.ObjectNew.GetAnnotations()

						// handle the removal of the disable-auto-import annotation
						_, oldAutoImportDisabled := oldAnnotations[apiconstants.DisableAutoImportAnnotation]
						_, newAutoImportDisabled := newAnnotations[apiconstants.DisableAutoImportAnnotation]
						if oldAutoImportDisabled && !newAutoImportDisabled {
							return true
						}

						// handle create-via annotation change
						oldCreateVia := oldAnnotations[constants.CreatedViaAnnotation]
						newCreateVia := newAnnotations[constants.CreatedViaAnnotation]
						return oldCreateVia != newCreateVia
					},
				},
			),
		).
		WatchesRawSource( // watch the import secret
			source.NewImportSecretSource(informerHolder.ImportSecretInformer, &source.ManagedClusterResourceEventHandler{},
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
		WatchesRawSource( // watch the klusterlet manifest works
			source.NewKlusterletWorkSource(informerHolder.KlusterletWorkInformer, &source.ManagedClusterResourceEventHandler{},
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
				})),
		).
		Complete(NewReconcileClusterDeployment(
			clientHolder.RuntimeClient,
			clientHolder.KubeClient,
			informerHolder,
			helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
			mcRecorder,
		))

	return controllerName, err
}
