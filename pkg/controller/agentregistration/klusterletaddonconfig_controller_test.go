package agentregistration

import (
	"context"
	"testing"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	klusterletaddonconfigv1schema "github.com/stolostron/klusterlet-addon-controller/pkg/apis"
	klusterletaddonconfigv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	// Create a new KlusterletAddonConfig object
	kac := &klusterletaddonconfigv1.KlusterletAddonConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-managed-cluster-with-label",
			Namespace: "test-managed-cluster-with-label",
		},
		Spec: klusterletaddonconfigv1.KlusterletAddonConfigSpec{
			ApplicationManagerConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
				Enabled: true,
			},
			CertPolicyControllerConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
				Enabled: true,
			},
			IAMPolicyControllerConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
				Enabled: true,
			},
			PolicyController: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
				Enabled: true,
			},
			SearchCollectorConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
				Enabled: false,
			},
		},
	}

	// Create a new ManagedCluster object
	managedClusterWithoutLable := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-managed-cluster-without-label",
		},
	}

	managedClusterWithLabel := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-managed-cluster-with-label",
			Annotations: map[string]string{
				AnnotationCreateWithDefaultKlusterletAddonConfig: "",
			},
		},
	}

	// Create a new ReconcileKlusterAddonConfig object
	s := runtime.NewScheme()
	klusterletaddonconfigv1schema.AddToScheme(s)
	clusterv1.AddToScheme(s)
	r := &reconcileKlusterAddonConfig{
		runtimeClient: fake.NewFakeClientWithScheme(s, managedClusterWithoutLable, managedClusterWithLabel),
		recorder:      eventstesting.NewTestingEventRecorder(t),
	}

	// Test case 1: ManagedCluster not found
	t.Run("ManagedClusterNotFound", func(t *testing.T) {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: "non-existent-managed-cluster",
			},
		}

		result, err := r.Reconcile(context.Background(), req)
		if err != nil {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() error = %v", err)
			return
		}

		if result.Requeue {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() requeue = true, want false")
		}
	})

	// Test case 2: KlusterletAddonConfig not found
	t.Run("KlusterletAddonConfigNotFound", func(t *testing.T) {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: "test-managed-cluster-with-label",
			},
		}
		result, err := r.Reconcile(context.Background(), req)
		if err != nil {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() error = %v", err)
			return
		}

		if result.Requeue {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() requeue = true, want false")
		}

		// Verify that the KlusterletAddonConfig object was created
		err = r.runtimeClient.Get(context.Background(), types.NamespacedName{Name: "test-managed-cluster-with-label", Namespace: "test-managed-cluster-with-label"}, kac)
		if err != nil {
			t.Errorf("Failed to get KlusterletAddonConfig: %v", err)
			return
		}
	})

	// Test case 3: KlusterletAddonConfig found
	t.Run("KlusterletAddonConfigFound", func(t *testing.T) {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: "test-managed-cluster-with-label",
			},
		}
		result, err := r.Reconcile(context.Background(), req)
		if err != nil {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() error = %v", err)
			return
		}

		if result.Requeue {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() requeue = true, want false")
		}
	})

	// Test case 4: The managed cluster doesn't have the label LabelCreateWithDefaultKlusterletAddonConfig
	t.Run("ManagedClusterWithoutLabel", func(t *testing.T) {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: "test-managed-cluster-without-label",
			},
		}
		result, err := r.Reconcile(context.Background(), req)
		if err != nil {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() error = %v", err)
			return
		}

		if result.Requeue {
			t.Errorf("reconcileKlusterAddonConfig.Reconcile() requeue = true, want false")
		}
	})
}
