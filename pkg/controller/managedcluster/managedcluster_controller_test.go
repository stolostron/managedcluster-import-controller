// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"testing"

	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
)

var (
	testscheme = scheme.Scheme
	now        = metav1.Now()
)

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	testscheme.AddKnownTypes(asv1beta1.GroupVersion, &asv1beta1.InfraEnvList{})
	testscheme.AddKnownTypes(asv1beta1.GroupVersion, &asv1beta1.InfraEnv{})
	testscheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.ManagedClusterAddOnList{})
	testscheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.ManagedClusterAddOn{})
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		startObjs    []client.Object
		request      reconcile.Request
		validateFunc func(t *testing.T, runtimeClient client.Client)
	}{
		{
			name:      "no managed clusters",
			startObjs: []client.Object{},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				// do nothing
			},
		},
		{
			name: "managed cluster is created",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(managedCluster.Finalizers) != 1 {
					t.Errorf("expected one finalizer, but failed")
				}
			},
		},
		{
			name: "managed clusters is deleting, but it has other finalizers",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{"test", constants.ImportFinalizer},
						DeletionTimestamp: &now,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(managedCluster.Finalizers) != 2 {
					t.Errorf("expected two finalizer, but failed")
				}
			},
		},
		{
			name: "managed clusters is deleting, but the finalizers is not right",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{"test"},
						DeletionTimestamp: &now,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				ns := &corev1.Namespace{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, ns); err == nil {
					t.Errorf("expected error, but failed")
				}
			},
		},
		{
			name: "managed clusters is deleting, but there are some pods in its namespace",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer},
						DeletionTimestamp: &now,
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				ns := &corev1.Namespace{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, ns); err != nil {
					t.Errorf("unexpected error, but failed, %v", err)
				}
			},
		},
		{
			name: "managed clusters is deleting, but there are other resource in its namespace",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer},
						DeletionTimestamp: &now,
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&asv1beta1.InfraEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				ns := &corev1.Namespace{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, ns); err != nil {
					t.Errorf("unexpected error, but failed, %v", err)
				}
			},
		},
		{
			name: "managed clusters is deleting, but there are addons in its namespace",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer},
						DeletionTimestamp: &now,
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&v1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{
							"cluster.open-cluster-management.io/addon-pre-delete",
						},
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				addon := &v1alpha1.ManagedClusterAddOn{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Namespace: "test", Name: "test"}, addon); !errors.IsNotFound(err) {
					t.Errorf("unexpected not found, but failed, %v, addon: %v", err, addon)
				}

				managedCluster := &clusterv1.ManagedCluster{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster); err != nil {
					t.Errorf("unexpected error, but failed, %v", err)
				}
			},
		},
		{
			name: "managed clusters is deleting, no addon in the ns",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer},
						DeletionTimestamp: &now,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("expected not found, but failed, %v", managedCluster)
				}

			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.TODO()
			kubeClient := kubefake.NewSimpleClientset()
			runtimeClient := fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.startObjs...).
				WithStatusSubresource(c.startObjs...).Build()
			r := NewReconcileManagedCluster(
				runtimeClient,
				eventstesting.NewTestingEventRecorder(t),
				helpers.NewManagedClusterEventRecorder(ctx, kubeClient),
			)

			_, err := r.Reconcile(ctx, c.request)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c.validateFunc(t, r.client)
		})
	}
}
