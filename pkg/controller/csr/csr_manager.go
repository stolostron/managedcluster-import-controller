// Copyright (c) Red Hat, Inc.
package csr

import (
	libgoclient "github.com/open-cluster-management/library-go/pkg/client"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new ManagedCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	kubeClient, err := libgoclient.NewDefaultKubeClient("")
	if err != nil {
		kubeClient = nil
	}
	return &ReconcileCSR{client: mgr.GetClient(), kubeClient: kubeClient, scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("csr-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	csrPredicateFuncs := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return csrPredicate(e.ObjectNew.(*certificatesv1beta1.CertificateSigningRequest))
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return csrPredicate(e.Object.(*certificatesv1beta1.CertificateSigningRequest))
		},
	}

	// Watch for changes to primary resource ManagedCluster
	err = c.Watch(
		&source.Kind{Type: &certificatesv1beta1.CertificateSigningRequest{}},
		&handler.EnqueueRequestForObject{},
		csrPredicateFuncs,
	)

	if err != nil {
		return err
	}

	return nil
}
