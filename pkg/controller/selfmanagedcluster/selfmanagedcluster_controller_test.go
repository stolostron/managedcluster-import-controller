// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package selfmanagedcluster

import (
	"context"
	"testing"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	operatorfake "open-cluster-management.io/api/client/operator/clientset/versioned/fake"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	corev1 "k8s.io/api/core/v1"
	crdv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
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
	testscheme.AddKnownTypes(operatorv1.SchemeGroupVersion, &operatorv1.Klusterlet{})
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		objs         []client.Object
		works        []runtime.Object
		secrets      []runtime.Object
		validateFunc func(t *testing.T, runtimeClient client.Client)
	}{
		{
			name:    "no managed clusters",
			objs:    []client.Object{},
			works:   []runtime.Object{},
			secrets: []runtime.Object{},
			validateFunc: func(t *testing.T, runtimeClient client.Client) {
				// do nothing
			},
		},
		{
			name: "self managed label is false",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "false",
						},
					},
				},
			},
			works:   []runtime.Object{},
			secrets: []runtime.Object{},
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
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
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
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
			},
			works:   []runtime.Object{},
			secrets: []runtime.Object{},
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
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-cluster-klusterlet-crds",
						Namespace: "local-cluster",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-cluster-klusterlet",
						Namespace: "local-cluster",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
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
			kubeClient := kubefake.NewSimpleClientset(c.secrets...)
			kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
			secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
			for _, secret := range c.secrets {
				secretInformer.GetStore().Add(secret)
			}

			workClient := workfake.NewSimpleClientset()
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			r := &ReconcileLocalCluster{
				clientHolder: &helpers.ClientHolder{
					KubeClient:          kubeClient,
					APIExtensionsClient: apiextensionsfake.NewSimpleClientset(),
					OperatorClient:      operatorfake.NewSimpleClientset(),
					RuntimeClient:       fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.objs...).Build(),
				},
				informerHolder: &source.InformerHolder{
					AutoImportSecretLister: kubeInformerFactory.Core().V1().Secrets().Lister(),
					ImportSecretLister:     kubeInformerFactory.Core().V1().Secrets().Lister(),
					KlusterletWorkLister:   workInformerFactory.Work().V1().ManifestWorks().Lister(),
				},
				recorder:   eventstesting.NewTestingEventRecorder(t),
				restMapper: restmapper.NewDiscoveryRESTMapper(apiGroupResources),
			}

			_, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "local-cluster",
				},
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c.validateFunc(t, r.clientHolder.RuntimeClient)
		})
	}
}
