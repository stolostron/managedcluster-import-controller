// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package selfmanagedcluster

import (
	"context"
	"strings"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kevents "k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const ControllerName = "selfmanagedcluster-controller"

// Add creates a new self managed cluster controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	mcRecorder kevents.EventRecorder,
	componentNamespace string) error {

	err := ctrl.NewControllerManagedBy(mgr).Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
		}).
		WatchesRawSource( // watch the import-secret
			source.NewImportSecretSource(informerHolder.ImportSecretInformer,
				&source.ManagedClusterResourceEventHandler{},
				predicate.Predicate(
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
					},
				)),
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
						// case 1: handle the label changed and new self managed label is true
						newLabels := e.ObjectNew.GetLabels()
						if !equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), newLabels) &&
							strings.EqualFold(newLabels[constants.SelfManagedLabel], "true") {
							return true
						}

						// case 2: handle the case where the ImmediateImport annotation is added with empty value
						if helpers.IsImmediateImport(e.ObjectNew.GetAnnotations()) {
							return true
						}

						// case 3: handle the removal of the disable-auto-import annotation
						_, oldAutoImportDisabled := e.ObjectOld.GetAnnotations()[apiconstants.DisableAutoImportAnnotation]
						_, newAutoImportDisabled := e.ObjectNew.GetAnnotations()[apiconstants.DisableAutoImportAnnotation]
						return oldAutoImportDisabled && !newAutoImportDisabled
					},
				},
			),
		).
		WatchesRawSource( // watch the klusterlet manifest works
			source.NewKlusterletWorkSource(informerHolder.KlusterletWorkInformer,
				&source.ManagedClusterResourceEventHandler{},
				predicate.Predicate(
					predicate.Funcs{
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
					},
				)),
		).
		Complete(NewReconcileLocalCluster(
			clientHolder,
			informerHolder,
			mgr.GetRESTMapper(),
			helpers.NewEventRecorder(clientHolder.KubeClient, ControllerName),
			mcRecorder,
			helpers.NewImportControllerConfig(componentNamespace, informerHolder.ControllerConfigLister, log),
		))
	return err
}
