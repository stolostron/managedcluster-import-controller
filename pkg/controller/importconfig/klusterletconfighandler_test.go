package importconfig

import (
	"context"
	"testing"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
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

func TestEnqueueManagedClusterByBootstrapKubeconfigSecret(t *testing.T) {
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

	klusterletconfigs := []*klusterletconfigv1alpha1.KlusterletConfig{
		{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-klusterletconfig1",
			},
			Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
				BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
					Type: operatorv1.LocalSecrets,
					LocalSecrets: operatorv1.LocalSecretsConfig{
						KubeConfigSecrets: []operatorv1.KubeConfigSecret{
							{
								Name: "test-secret1",
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-klusterletconfig2",
			},
			Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
				BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
					Type: operatorv1.LocalSecrets,
					LocalSecrets: operatorv1.LocalSecretsConfig{
						KubeConfigSecrets: []operatorv1.KubeConfigSecret{
							{
								Name: "test-secret2",
							},
						},
					},
				},
			},
		},
	}

	testcases := []struct {
		addEvent func(handler handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request])
		verify   func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request])
	}{
		{
			// create a klusterletconfig with secrets, expect the managed cluster to be enqueued
			addEvent: func(h handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				// Create a new create event for the klusterletconfig
				evt := event.CreateEvent{
					Object: &corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name: "test-secret1",
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
			// create a klusterletconfig with secrets, expect the managed cluster to be enqueued
			addEvent: func(h handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				// Create a new create event for the klusterletconfig
				evt := event.CreateEvent{
					Object: &corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name: "test-secret2",
						},
					},
				}
				h.Create(context.Background(), evt, queue)
			},
			verify: func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				if queue.Len() != 2 {
					t.Errorf("Expected queue length to be 2, but got %d", queue.Len())
				}

				item, _ := queue.Get()
				if item.Name != "test2" &&
					item.Name != "test3" {
					t.Errorf("Expected item to be test2 or test3, but got %v", item.Name)
				}

				item, _ = queue.Get()
				if item.Name != "test2" &&
					item.Name != "test3" {
					t.Errorf("Expected item to be test2 or test3, but got %v", item.Name)
				}
			},
		},
	}

	for _, tc := range testcases {
		// Create fake clientet and indexer
		managedClusterIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			ManagedClusterKlusterletConfigAnnotationIndexKey: IndexManagedClusterByKlusterletconfigAnnotation,
		})

		klusterletconfigIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			KlusterletConfigBootstrapKubeConfigSecretsIndexKey: IndexKlusterletConfigByBootstrapKubeConfigSecrets(),
		})

		// Create a new handler
		handler := &enqueueManagedClusterByBootstrapKubeConfigSecrets{
			managedclusterIndexer:   managedClusterIndexer,
			klusterletconfigIndexer: klusterletconfigIndexer,
		}

		// Create a new queue
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

		// Add the managed clusters to the indexer
		for _, mc := range mcs {
			err := managedClusterIndexer.Add(mc)
			if err != nil {
				t.Fatalf("Failed to add managed cluster to indexer: %v", err)
			}
		}

		for _, kc := range klusterletconfigs {
			err := klusterletconfigIndexer.Add(kc)
			if err != nil {
				t.Fatalf("Failed to add klusterletconfig to indexer: %v", err)
			}
		}

		// Add the event to the handler
		tc.addEvent(handler, queue)
		tc.verify(t, queue)
	}
}

func TestIndexKlusterletConfigByBootstrapKubeConfigSecrets(t *testing.T) {
	// Create a new klusterletconfig with a bootstrap kubeconfig secret
	kc := &klusterletconfigv1alpha1.KlusterletConfig{
		Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
			BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
				Type: operatorv1.LocalSecrets,
				LocalSecrets: operatorv1.LocalSecretsConfig{
					KubeConfigSecrets: []operatorv1.KubeConfigSecret{
						{
							Name: "test-secret1",
						},
						{
							Name: "test-secret2",
						},
					},
				},
			},
		},
	}

	kcWithoutSecrets := &klusterletconfigv1alpha1.KlusterletConfig{}

	// Test the function with a klusterletconfig that has bootstrap kubeconfig secrets
	result, err := IndexKlusterletConfigByBootstrapKubeConfigSecrets()(kc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 2 || result[0] != "test-secret1" || result[1] != "test-secret2" {
		t.Errorf("Expected result to be [\"test-secret1\", \"test-secret2\"], but got %v", result)
	}

	// Test the function with a klusterletconfig that does not have bootstrap kubeconfig secrets
	result, err = IndexKlusterletConfigByBootstrapKubeConfigSecrets()(kcWithoutSecrets)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected result to be [], but got %v", result)
	}
}

func TestEnqueueManagedClusterByCustomizedCAConfigmaps(t *testing.T) {
	mcs := []*clusterv1.ManagedCluster{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:        "test1",
				Annotations: map[string]string{"agent.open-cluster-management.io/klusterlet-config": "test-kc1"},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:        "test2",
				Annotations: map[string]string{"agent.open-cluster-management.io/klusterlet-config": "test-kc2"},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:        "test3",
				Annotations: map[string]string{"agent.open-cluster-management.io/klusterlet-config": "test-kc2"},
			},
		},
	}

	klusterletconfigs := []*klusterletconfigv1alpha1.KlusterletConfig{
		{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-kc1",
			},
			Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
				HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
					ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyDefault,
					TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
						{
							Name: "test-cm1",
							CABundle: &klusterletconfigv1alpha1.ConfigMapReference{
								Namespace: "ns1",
								Name:      "cm1",
							},
						},
						{
							Name: "test-cm2",
							CABundle: &klusterletconfigv1alpha1.ConfigMapReference{
								Namespace: "ns2",
								Name:      "cm2",
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-kc2",
			},
			Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
				HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
					ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyDefault,
					TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
						{
							Name: "test-cm3",
							CABundle: &klusterletconfigv1alpha1.ConfigMapReference{
								Namespace: "ns3",
								Name:      "cm3",
							},
						},
						{
							Name: "test-cm4",
							CABundle: &klusterletconfigv1alpha1.ConfigMapReference{
								Namespace: "ns2",
								Name:      "cm2",
							},
						},
					},
				},
			},
		},
	}

	testcases := []struct {
		addEvent func(handler handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request])
		verify   func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request])
	}{
		{
			// create a klusterletconfig with configmaps, expect the managed cluster to be enqueued
			addEvent: func(h handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				// Create a new configmap event for the klusterletconfig
				evt := event.CreateEvent{
					Object: &corev1.ConfigMap{
						ObjectMeta: v1.ObjectMeta{
							Name:      "cm1",
							Namespace: "ns1",
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
			// create a klusterletconfig with configmaps, expect the managed cluster to be enqueued
			addEvent: func(h handler.EventHandler, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				// Create a new configmap event for the klusterletconfig
				evt := event.CreateEvent{
					Object: &corev1.ConfigMap{
						ObjectMeta: v1.ObjectMeta{
							Name:      "cm2",
							Namespace: "ns2",
						},
					},
				}
				h.Create(context.Background(), evt, queue)
			},
			verify: func(t *testing.T, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				if queue.Len() != 3 {
					t.Errorf("Expected queue length to be 3, but got %d", queue.Len())
				}

				item, _ := queue.Get()
				if item.Name != "test1" && item.Name != "test2" && item.Name != "test3" {
					t.Errorf("Expected item to be test1, test2 or test3, but got %v", item.Name)
				}
				item, _ = queue.Get()
				if item.Name != "test1" && item.Name != "test2" && item.Name != "test3" {
					t.Errorf("Expected item to be test1, test2 or test3, but got %v", item.Name)
				}
				item, _ = queue.Get()
				if item.Name != "test1" && item.Name != "test2" && item.Name != "test3" {
					t.Errorf("Expected item to be test1, test2 or test3, but got %v", item.Name)
				}
			},
		},
	}

	for _, tc := range testcases {
		// Create fake clientet and indexer
		managedClusterIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			ManagedClusterKlusterletConfigAnnotationIndexKey: IndexManagedClusterByKlusterletconfigAnnotation,
		})

		klusterletconfigIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			KlusterletConfigCustomizedCAConfigmapsIndexKey: IndexKlusterletConfigByCustomizedCAConfigmaps(),
		})

		// Create a new handler
		handler := &enqueueManagedClusterByCustomizedCAConfigmaps{
			managedclusterIndexer:   managedClusterIndexer,
			klusterletconfigIndexer: klusterletconfigIndexer,
		}

		// Create a new queue
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

		// Add the managed clusters to the indexer
		for _, mc := range mcs {
			err := managedClusterIndexer.Add(mc)
			if err != nil {
				t.Fatalf("Failed to add managed cluster to indexer: %v", err)
			}
		}

		for _, kc := range klusterletconfigs {
			err := klusterletconfigIndexer.Add(kc)
			if err != nil {
				t.Fatalf("Failed to add klusterletconfig to indexer: %v", err)
			}
		}

		// Add the event to the handler
		tc.addEvent(handler, queue)
		tc.verify(t, queue)
	}
}
