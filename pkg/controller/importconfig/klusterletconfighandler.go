package importconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
)

var _ handler.EventHandler = &enqueueManagedClusterInKlusterletConfigAnnotation{}

type enqueueManagedClusterInKlusterletConfigAnnotation struct {
	managedclusterIndexer cache.Indexer
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.enqueue(evt.ObjectNew.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) enqueue(klusterletconfigName string, q workqueue.RateLimitingInterface) {
	objs, err := e.managedclusterIndexer.ByIndex(ManagedClusterKlusterletConfigAnnotationIndexKey, klusterletconfigName)
	if err != nil {
		klog.Error(err, "Failed to get managed clusters by klusterletconfig annotation by indexer", "klusterletconfig", klusterletconfigName)
		return
	}
	for _, obj := range objs {
		mc := obj.(*clusterv1.ManagedCluster)
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name: mc.GetName(),
		}})
	}
}

const (
	ManagedClusterKlusterletConfigAnnotationIndexKey = "annotation-klusterletconfig"
)

func IndexManagedClusterByKlusterletconfigAnnotation(obj interface{}) ([]string, error) {
	managedCluster, ok := obj.(*clusterv1.ManagedCluster)
	if !ok {
		return nil, fmt.Errorf("not a managedcluster object")
	}
	klusterletconfig, ok := managedCluster.GetAnnotations()[apiconstants.AnnotationKlusterletConfig]
	if ok && klusterletconfig != "" {
		return []string{klusterletconfig}, nil
	}
	return nil, nil
}
