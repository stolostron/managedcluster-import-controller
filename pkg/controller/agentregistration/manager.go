package agentregistration

import (
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const klusterAddonConfigController = "klusterAddonConfig-controller"

func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder, informerHolder *source.InformerHolder) (string, error) {
	c, err := controller.New(klusterAddonConfigController, mgr, controller.Options{
		Reconciler: &reconcileKlusterAddonConfig{
			runtimeClient: clientHolder.RuntimeClient,
			recorder:      helpers.NewEventRecorder(clientHolder.KubeClient, klusterAddonConfigController),
		},
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return klusterAddonConfigController, err
	}

	// only watch the create of managedcluster
	if err := c.Watch(
		&runtimesource.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc:  func(e event.UpdateEvent) bool { return false },
		}),
	); err != nil {
		return klusterAddonConfigController, err
	}
	return klusterAddonConfigController, nil
}
