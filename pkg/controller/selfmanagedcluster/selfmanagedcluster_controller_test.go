// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package selfmanagedcluster

import (
	"context"
	"testing"

	operatorfake "github.com/open-cluster-management/api/client/operator/clientset/versioned/fake"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers/testing"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	corev1 "k8s.io/api/core/v1"
	crdv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var apiGroupResources = []*restmapper.APIGroupResources{
	{
		Group: metav1.APIGroup{
			Name: "apiextensions.k8s.io",
			Versions: []metav1.GroupVersionForDiscovery{
				{Version: "v1beta1"},
			},
			PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1beta1"},
		},
		VersionedResources: map[string][]metav1.APIResource{
			"v1beta1": {
				{Name: "customresourcedefinitions", Namespaced: false, Kind: "CustomResourceDefinition"},
			},
		},
	},
}

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(crdv1beta1.SchemeGroupVersion, &crdv1beta1.CustomResourceDefinition{})
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		objs         []runtime.Object
		validateFunc func(t *testing.T, runtimeClient client.Client)
	}{
		{
			name: "no managed clusters",
			objs: []runtime.Object{},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				// do nothing
			},
		},
		{
			name: "self managed label is false",
			objs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "false",
						},
					},
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				cluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "local-cluster"}, cluster)
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
				if len(cluster.Status.Conditions) != 0 {
					t.Errorf("unexpected condistions")
				}
			},
		},
		{
			name: "has auto-import-secret",
			objs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "local-cluster",
					},
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				cluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "local-cluster"}, cluster)
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
				if len(cluster.Status.Conditions) != 0 {
					t.Errorf("unexpected condistions")
				}
			},
		},
		{
			name: "no import secret",
			objs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				cluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "local-cluster"}, cluster)
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
				if len(cluster.Status.Conditions) != 0 {
					t.Errorf("unexpected condistions")
				}
			},
		},
		{
			name: "import cluster",
			objs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
				testinghelpers.GetImportSecret("local-cluster"),
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				cluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: "local-cluster"}, cluster)
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
				if len(cluster.Status.Conditions) == 0 {
					t.Errorf("unexpected condistions")
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &ReconcileLocalCluster{
				clientHolder: &helpers.ClientHolder{
					KubeClient:          kubefake.NewSimpleClientset(),
					APIExtensionsClient: apiextensionsfake.NewSimpleClientset(),
					OperatorClient:      operatorfake.NewSimpleClientset(),
					RuntimeClient:       fake.NewFakeClientWithScheme(testscheme, c.objs...),
				},
				scheme:     testscheme,
				recorder:   eventstesting.NewTestingEventRecorder(t),
				restMapper: restmapper.NewDiscoveryRESTMapper(apiGroupResources),
			}

			_, err := r.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Name: "local-cluster"}})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c.validateFunc(t, r.clientHolder.RuntimeClient)
		})
	}
}
