// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hosted

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	testscheme = scheme.Scheme
)

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.ManagedClusterAddOnList{})
	testscheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.ManagedClusterAddOn{})

	testscheme.AddKnownTypes(v1alpha1.SchemeGroupVersion, &workv1.ManifestWork{})
	testscheme.AddKnownTypes(v1alpha1.GroupVersion, &workv1.ManifestWorkList{})
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		runtimeObjs  []client.Object  // used by clientHolder.RuntimeClient
		kubeObjs     []runtime.Object // used by clientHolder.KubeClient
		request      reconcile.Request
		vaildateFunc func(t *testing.T, reconcileResult reconcile.Result, reconcoleErr error, clientHolder *helpers.ClientHolder)
	}{
		// managedcluster is not found, expect to do nothing
		{
			name:        "managedcluster is not found",
			runtimeObjs: []client.Object{},
			kubeObjs:    []runtime.Object{},
			request:     reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
			},
		},
		// managedcluster is not Hosted mode, expect to do nothing
		{
			name: "managedcluster is not Hosted mode",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeDefault,
						},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			request:  reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
			},
		},
		// managedcluster is Hosted mode, but annotation hostingClusterName not found
		{
			name: "managedcluster is Hosted mode, but annotation hostingClusterName not found",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
						},
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      "manifest1",
					},
				},
			},
			kubeObjs: []runtime.Object{},
			request:  reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr == nil || !strings.Contains(reconcileErr.Error(), fmt.Sprintf("annotation %s not found", constants.HostingClusterNameAnnotation)) {
					t.Errorf("expect err annotation %s not found, but get %v", constants.HostingClusterNameAnnotation, reconcileErr)
				}
			},
		},
		// managedcluster is Hosted mode but no manifests found
		{
			name: "managedcluster is Hosted mode ",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
							constants.HostingClusterNameAnnotation:   "cluster1", // hosting cluster name
						},
						Finalizers: []string{constants.ManifestWorkFinalizer},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			request:  reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				managedcluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedcluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				// expect finalizer is removed
				if len(managedcluster.Finalizers) > 0 {
					t.Errorf("expect finalizer is removed, but get %v", managedcluster.Finalizers)
				}
			},
		},
		// managedcluster is Hosted mode, and managedCluster is deleting
		{
			name: "managedcluster is Hosted mode, and managedCluster is deleted",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
						DeletionTimestamp: &metav1.Time{Time: time.Now()}, // managedCluster is deleted
					},
				},
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      "manifest1",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-klusterlet",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
					},
				},
			},
			kubeObjs: []runtime.Object{},
			request:  reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				managedcluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedcluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				// expect finalizer is removed
				if len(managedcluster.Finalizers) > 0 {
					t.Errorf("expect no finalizer added, but get %v", managedcluster.Finalizers)
				}

				// expect hosted manifestworks are deleted
				manifestworks := &workv1.ManifestWorkList{}
				err = ch.RuntimeClient.List(context.TODO(), manifestworks, client.InNamespace("cluster1"))
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if len(manifestworks.Items) != 0 {
					t.Errorf("expect no manifestwork, but get %v", manifestworks.Items)
				}
			},
		},
		// managedcluster is Hosted mode, but no import secret
		{
			name: "managedcluster is Hosted mode, and managedCluster is deleting",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      "manifest1",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-klusterlet",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
					},
				},
			},
			kubeObjs: []runtime.Object{},
			request:  reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
			},
		},
		// managedcluster is Hosted mode, but import secret don't have data
		{
			name: "managedcluster is Hosted mode, but import secret don't have data",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      "manifest1",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-klusterlet",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
					},
				},
			},
			kubeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-import",
						Namespace: "test",
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr == nil || strings.Contains(reconcileErr.Error(), "is rquired") {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
			},
		},
		// managedcluster is Hosted mode, and import secret have the data
		{
			name: "managedcluster is Hosted mode, and import secret have the data",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      "manifest1",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-klusterlet",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
					},
				},
			},
			kubeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-import",
						Namespace: "test",
					},
					Data: map[string][]byte{
						constants.ImportSecretImportYamlKey: []byte(`apiVersion: v1
kind: Namespace
metadata:
  name: foo1`),
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
			},
		},
		// TODO: add auto import secret test cases
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &ReconcileHosted{
				clientHolder: &helpers.ClientHolder{
					RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).
						WithObjects(c.runtimeObjs...).Build(),
					KubeClient: kubefake.NewSimpleClientset(c.kubeObjs...),
				},
				recorder: eventstesting.NewTestingEventRecorder(t),
				scheme:   testscheme,
			}
			response, err := r.Reconcile(context.Background(), c.request)
			c.vaildateFunc(t, response, err, r.clientHolder)
		})
	}
}
