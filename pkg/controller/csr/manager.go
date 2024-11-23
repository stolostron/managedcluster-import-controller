// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package csr

import (
	"context"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	certificatesv1 "k8s.io/api/certificates/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	ControllerName = "csr-controller"
)

// Add creates a new CSR Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder,
	extraApprovalConditions []func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error)) error {

	err := ctrl.NewControllerManagedBy(mgr).Named(ControllerName).
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
					return isValidUnapprovedBootstrapCSR(e.ObjectNew.(*certificatesv1.CertificateSigningRequest))
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return isValidUnapprovedBootstrapCSR(e.Object.(*certificatesv1.CertificateSigningRequest))
				},
			})).
		Complete(&ReconcileCSR{
			clientHolder: clientHolder,
			recorder:     helpers.NewEventRecorder(clientHolder.KubeClient, ControllerName),
			approvalConditions: append([]func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error){
				// The DEFAULT approval condition: if a CSR comes from a managed cluster, and the managed cluster already exists, approve it
				func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error) {
					return approveExistingManagedClusterCSR(ctx, csr, clientHolder)
				},
			}, extraApprovalConditions...),
		})

	return err
}
