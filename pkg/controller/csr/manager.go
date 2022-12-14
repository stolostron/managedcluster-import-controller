// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package csr

import (
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	certificatesv1 "k8s.io/api/certificates/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "csr-controller"

// Add creates a new CSR Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder, _ *source.InformerHolder) (string, error) {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: &ReconcileCSR{
			clientHolder: clientHolder,
			recorder:     helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
		},
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return controllerName, err
	}

	csrPredicateFuncs := predicate.Funcs{
		GenericFunc: func(e event.GenericEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			return csrPredicate(e.ObjectNew.(*certificatesv1.CertificateSigningRequest))
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return csrPredicate(e.Object.(*certificatesv1.CertificateSigningRequest))
		},
	}

	// Watch for changes to primary resource ManagedCluster
	err = c.Watch(
		&runtimesource.Kind{Type: &certificatesv1.CertificateSigningRequest{}},
		&handler.EnqueueRequestForObject{},
		csrPredicateFuncs,
	)

	if err != nil {
		return controllerName, err
	}

	return controllerName, nil
}
