// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"testing"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
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

	spokeKubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := spokeKubeClient.CoreV1().Namespaces().Create(
		context.TODO(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "open-cluster-management-agent",
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := spokeKubeClient.CoreV1().Secrets("open-cluster-management-agent").Create(
		context.TODO(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bootstrap-hub-kubeconfig",
			},
			Data: map[string][]byte{
				"kubeconfig": []byte("dumb"),
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		objs        []client.Object
		works       []runtime.Object
		secrets     []runtime.Object
		expectedErr bool
	}{
		{
			name:        "no cluster",
			objs:        []client.Object{},
			expectedErr: false,
		},
		{
			name: "hosted cluster",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
						},
					},
				},
			},
			works:       []runtime.Object{},
			secrets:     []runtime.Object{},
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
			works:       []runtime.Object{},
			secrets:     []runtime.Object{},
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
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
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
			name: "no manifest works",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
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
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
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
		{
			name: "only update the bootstrap secret",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
						Labels: map[string]string{
							restoreLabel: "true",
						},
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
		{
			name: "only update the bootstrap secret - works unavailable",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
						Labels: map[string]string{
							restoreLabel: "true",
						},
					},
					Data: map[string][]byte{
						"autoImportRetry": []byte("0"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "only update the bootstrap secret - works available",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
						Labels: map[string]string{
							restoreLabel: "true",
						},
					},
					Data: map[string][]byte{
						"autoImportRetry": []byte("0"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
				},
			},
			expectedErr: false,
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

			workClient := workfake.NewSimpleClientset(c.works...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			r := &ReconcileAutoImport{
				client:     fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.objs...).Build(),
				kubeClient: kubeClient,
				workClient: workClient,
				informerHolder: &source.InformerHolder{
					AutoImportSecretLister: kubeInformerFactory.Core().V1().Secrets().Lister(),
					ImportSecretLister:     kubeInformerFactory.Core().V1().Secrets().Lister(),
					KlusterletWorkLister:   workInformerFactory.Work().V1().ManifestWorks().Lister(),
				},
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
