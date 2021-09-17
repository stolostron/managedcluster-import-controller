// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusterdeployment

import (
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	testinghelpers "github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
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
		objs        []runtime.Object
		expectedErr bool
	}{
		{
			name: "no clusterdeployment",
			objs: []runtime.Object{},
		},
		{
			name: "no cluster",
			objs: []runtime.Object{
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
					},
				},
			},
		},
		{
			name: "clusterdeployment is not installed",
			objs: []runtime.Object{
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
		},
		{
			name: "import cluster with auto-import secret",
			objs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
					},
				},
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
					},
				},
			},
		},
		{
			name: "import cluster with clusterdeployment secret",
			objs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
						ClusterMetadata: &hivev1.ClusterMetadata{
							AdminKubeconfigSecretRef: corev1.LocalObjectReference{
								Name: "test",
							},
						},
					},
				},
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token":  []byte(config.BearerToken),
						"server": []byte(config.Host),
					},
				},
			},
			expectedErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &ReconcileClusterDeployment{
				//client:   fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.objs...).Build(),
				client:   fake.NewFakeClientWithScheme(testscheme, c.objs...),
				recorder: eventstesting.NewTestingEventRecorder(t),
			}

			_, err := r.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}})
			if c.expectedErr && err == nil {
				t.Errorf("expected error, but failed")
			}
			if !c.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
