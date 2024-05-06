// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package csr

import (
	"context"

	certificatesv1 "k8s.io/api/certificates/v1"
	kevents "k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

const controllerName = "csr-controller"

// Add creates a new CSR Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	_ *source.InformerHolder,
	mcRecorder kevents.EventRecorder) (string, error) {

	err := ctrl.NewControllerManagedBy(mgr).Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
		}).
		Watches(
			&certificatesv1.CertificateSigningRequest{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
				UpdateFunc: func(e event.UpdateEvent) bool {
					return csrPredicate(e.ObjectNew.(*certificatesv1.CertificateSigningRequest))
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return csrPredicate(e.Object.(*certificatesv1.CertificateSigningRequest))
				},
			})).
		Complete(&ReconcileCSR{
			clientHolder: clientHolder,
			recorder:     helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
		})

	return controllerName, err
}
