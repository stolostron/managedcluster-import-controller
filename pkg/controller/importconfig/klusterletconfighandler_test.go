package importconfig

import (
	"context"
	"testing"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestEnqueueManagedClusterInKlusterletConfigAnnotation(t *testing.T) {
	mcs := []*clusterv1.ManagedCluster{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:        "test1",
				Annotations: map[string]string{"agent.open-cluster-management.io/klusterlet-config": "test-klusterletconfig1"},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:        "test2",
				Annotations: map[string]string{"agent.open-cluster-management.io/klusterlet-config": "test-klusterletconfig2"},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:        "test3",
				Annotations: map[string]string{"agent.open-cluster-management.io/klusterlet-config": "test-klusterletconfig2"},
			},
		},
	}

	testcases := []struct {
		addEvent func(handler handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request])
		verify   func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request])
	}{
		{
			addEvent: func(h handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				// Create a new create event for the managed cluster
				evt := event.CreateEvent{
					Object: &klusterletconfigv1alpha1.KlusterletConfig{
						ObjectMeta: v1.ObjectMeta{
							Name: "test-klusterletconfig1",
						},
					},
				}
				h.Create(context.Background(), evt, queue)
			},
			verify: func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				if queue.Len() != 1 {
					t.Errorf("Expected queue length to be 1, but got %d", queue.Len())
				}
				expectedRequest := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: "test1",
					},
				}
				// Get the item from the queue
				item, _ := queue.Get()
				// Check that the item is the expected reconcile request
				if item != expectedRequest {
					t.Errorf("Expected item to be %v, but got %v", expectedRequest, item)
				}
			},
		},
		{
			addEvent: func(h handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				// Create a new create event for the managed cluster
				evt := event.DeleteEvent{
					Object: &klusterletconfigv1alpha1.KlusterletConfig{
						ObjectMeta: v1.ObjectMeta{
							Name: "test-klusterletconfig2",
						},
					},
				}
				h.Delete(context.Background(), evt, queue)
			},
			verify: func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				if queue.Len() != 2 {
					t.Errorf("Expected queue length to be 2, but got %d", queue.Len())
				}
			},
		},
		{
			addEvent: func(h handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				// Create a new create event for the managed cluster
				evt := event.CreateEvent{
					Object: &klusterletconfigv1alpha1.KlusterletConfig{
						ObjectMeta: v1.ObjectMeta{
							Name: "global",
						},
					},
				}
				h.Create(context.Background(), evt, queue)
			},
			verify: func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				if queue.Len() != 3 {
					t.Errorf("Expected queue length to be 3, but got %d", queue.Len())
				}
			},
		},
	}

	for _, tc := range testcases {
		// Create a fake clientset and indexer
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			ManagedClusterKlusterletConfigAnnotationIndexKey: IndexManagedClusterByKlusterletconfigAnnotation,
		})

		// Create a new kcHandler
		kcHandler := &enqueueManagedClusterInKlusterletConfigAnnotation{
			managedclusterIndexer: indexer,
		}

		// Create a new queue
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		// Add the managed clusters to the indexer
		for _, mc := range mcs {
			err := indexer.Add(mc)
			if err != nil {
				t.Fatalf("Failed to add managed cluster to indexer: %v", err)
			}
		}

		tc.addEvent(kcHandler, queue)
		tc.verify(t, queue)
	}
}

func TestIndexManagedClusterByKlusterletconfigAnnotation(t *testing.T) {
	// Create a new managed cluster with a klusterletconfig annotation
	mcWithAnnotation := &clusterv1.ManagedCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test1",
			Annotations: map[string]string{"agent.open-cluster-management.io/klusterlet-config": "test-klusterletconfig"},
		},
	}

	// Create a new managed cluster without a klusterletconfig annotation
	mcWithoutAnnotation := &clusterv1.ManagedCluster{
		ObjectMeta: v1.ObjectMeta{
			Name: "test2",
		},
	}

	// Test the function with a managed cluster that has a klusterletconfig annotation
	result, err := IndexManagedClusterByKlusterletconfigAnnotation(mcWithAnnotation)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 2 || result[0] != "global" || result[1] != "test-klusterletconfig" {
		t.Errorf("Expected result to be [\"global, test-klusterletconfig\"], but got %v", result)
	}

	// Test the function with a managed cluster that does not have a klusterletconfig annotation
	result, err = IndexManagedClusterByKlusterletconfigAnnotation(mcWithoutAnnotation)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "global" {
		t.Errorf("Expected result to be [\"global\"], but got %v", result)
	}
}
