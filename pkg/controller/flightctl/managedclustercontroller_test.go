package flightctl

import (
	"context"
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestManagedClusterReconciler(t *testing.T) {
	s := scheme.Scheme
	err := clusterv1.Install(s)
	if err != nil {
		t.Fatalf("Failed to install cluster scheme: %v", err)
	}

	tests := []struct {
		name               string
		existingObjects    []runtime.Object
		isFlightCtlDevice  bool
		flightCtlError     error
		expectedAcceptance bool
		expectError        bool
	}{
		{
			name: "cluster is flightctl device",
			existingObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: false,
					},
				},
			},
			isFlightCtlDevice:  true,
			expectedAcceptance: true,
			expectError:        false,
		},
		{
			name: "cluster is not flightctl device",
			existingObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: false,
					},
				},
			},
			isFlightCtlDevice:  false,
			expectedAcceptance: false,
			expectError:        false,
		},
		{
			name: "cluster already accepted",
			existingObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			isFlightCtlDevice:  true,
			expectedAcceptance: true,
			expectError:        false,
		},
		{
			name: "flightctl error",
			existingObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: false,
					},
				},
			},
			isFlightCtlDevice:  false,
			flightCtlError:     assert.AnError,
			expectedAcceptance: false,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithRuntimeObjects(tt.existingObjects...).
				Build()

			clientHolder := &helpers.ClientHolder{
				RuntimeClient: fakeClient,
			}

			reconciler := &ManagedClusterReconciler{
				clientHolder: clientHolder,
				isManagedClusterAFlightctlDevice: func(ctx context.Context, managedClusterName string) (bool, error) {
					return tt.isFlightCtlDevice, tt.flightCtlError
				},
			}

			// Reconcile
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test-cluster",
				},
			})

			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Get the cluster and check its status
			cluster := &clusterv1.ManagedCluster{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-cluster"}, cluster)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedAcceptance, cluster.Spec.HubAcceptsClient)
		})
	}
}
