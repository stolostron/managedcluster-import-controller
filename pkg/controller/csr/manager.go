// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package csr

import (
	"context"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const ControllerName = "csr-controller"

// Add creates a new CSR Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context,
	mgr manager.Manager,
	clientHolder *helpers.ClientHolder) error {

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
					return csrPredicate(e.ObjectNew.(*certificatesv1.CertificateSigningRequest))
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return csrPredicate(e.Object.(*certificatesv1.CertificateSigningRequest))
				},
			})).
		Complete(&ReconcileCSR{
			clientHolder: clientHolder,
			recorder:     helpers.NewEventRecorder(clientHolder.KubeClient, ControllerName),
			checks: []func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error){
				// Case 1: if a CSR comes from a managed cluster that already exists, approve it
				func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error) {
					clusterName := getClusterName(csr)
					cluster := clusterv1.ManagedCluster{}
					err := clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: clusterName}, &cluster)
					if errors.IsNotFound(err) {
						return false, nil
					}
					if err != nil {
						return false, err
					}
					return true, nil
				},
			},
		})

	return err
}
