// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"testing"

	testinghelpers "github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers/testing"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
}

func TestReconcile(t *testing.T) {
	apiServer := &envtest.Environment{}
	config, err := apiServer.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer apiServer.Stop()

	cases := []struct {
		name        string
		objs        []client.Object
		expectedErr bool
	}{
		{
			name:        "no cluster",
			objs:        []client.Object{},
			expectedErr: false,
		},
		{
			name: "no auto-import-secret",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "no import-secret",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "import cluster with auto-import secret",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"autoImportRetry": []byte("0"),
						"token":           []byte(config.BearerToken),
						"server":          []byte(config.Host),
					},
				},
			},
			expectedErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &ReconcileAutoImport{
				client:   fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.objs...).Build(),
				recorder: eventstesting.NewTestingEventRecorder(t),
			}

			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "test"}}
			_, err := r.Reconcile(context.TODO(), req)
			if c.expectedErr && err == nil {
				t.Errorf("expected error, but failed")
			}
			if !c.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
