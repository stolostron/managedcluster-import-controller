package resourcecleanup

import (
	"context"
	"fmt"
	"testing"

	clustercontroller "github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsManagedClusterNamespace(t *testing.T) {
	cases := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{name: "nil"},
		{
			name: "unlabeled namespace",
			obj:  &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		},
		{
			name: "managed cluster label",
			obj: &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "test",
				Labels: map[string]string{clustercontroller.ClusterLabel: "test"},
			}},
			want: true,
		},
		{
			name: "cluster name label",
			obj: &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "test",
				Labels: map[string]string{clusterv1.ClusterNameLabelKey: "test"},
			}},
			want: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isManagedClusterNamespace(c.obj); got != c.want {
				t.Fatalf("expected %v, got %v", c.want, got)
			}
		})
	}
}

func TestEnqueueOrphanedClusterNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	runtimeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "live"}},
	).Build()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "live"}}
	if requests := enqueueOrphanedClusterNamespace(runtimeClient)(context.Background(), ns); len(requests) != 0 {
		t.Fatalf("expected no request for a namespace whose cluster still exists, got %v", requests)
	}

	ns.Name = "gone"
	requests := enqueueOrphanedClusterNamespace(runtimeClient)(context.Background(), ns)
	if len(requests) != 1 || requests[0].Name != "gone" || requests[0].Namespace != fromNamespaceWatch {
		t.Fatalf("expected orphan cleanup for gone, got %v", requests)
	}

	requests = enqueueOrphanedClusterNamespace(errClient{})(context.Background(), ns)
	if len(requests) != 1 || requests[0].Name != "gone" || requests[0].Namespace != fromNamespaceWatch {
		t.Fatalf("expected a retry request when the cluster lookup fails, got %v", requests)
	}
}

type errClient struct {
	client.Client
}

func (errClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return fmt.Errorf("lookup failed")
}
