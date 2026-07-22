// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"testing"
	"time"

	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	operatorfake "open-cluster-management.io/api/client/operator/clientset/versioned/fake"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	testscheme = scheme.Scheme
	now        = v1.Now()
)

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(addonv1alpha1.SchemeGroupVersion, &addonv1alpha1.ManagedClusterAddOn{})
	testscheme.AddKnownTypes(addonv1alpha1.SchemeGroupVersion, &addonv1alpha1.ManagedClusterAddOnList{})
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		startObjs    []client.Object
		works        []runtime.Object
		secrets      []runtime.Object
		request      reconcile.Request
		validateFunc func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface)
	}{
		{
			name:      "no managed clusters",
			startObjs: []client.Object{},
			works:     []runtime.Object{},
			secrets:   []runtime.Object{},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				// do nothing
			},
		},
		{
			name: "manifest works are created",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{
						Name: "test",
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
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
			name: "apply klusterlet manifest works",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{
						Name:       "test",
						Finalizers: []string{constants.ManifestWorkFinalizer},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []v1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionJoined,
								Status: v1.ConditionTrue,
							},
						},
						Version: clusterv1.ManagedClusterVersion{Kubernetes: "v1.18.0"},
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				manifestWorks, err := workClient.WorkV1().ManifestWorks("test").List(context.TODO(), v1.ListOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(manifestWorks.Items) != 2 {
					t.Errorf("expected one work, but failed %d", len(manifestWorks.Items))
				}
			},
		},
		{
			name: "disable-auto-import annotation set - ManifestWorks have ReadOnly configs",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							"import.open-cluster-management.io/disable-auto-import": "",
						},
						Finalizers: []string{constants.ManifestWorkFinalizer},
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				// Check klusterlet ManifestWork
				klusterletWork, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), "test-klusterlet", v1.GetOptions{})
				if err != nil {
					t.Errorf("failed to get klusterlet ManifestWork: %v", err)
					return
				}

				if len(klusterletWork.Spec.ManifestConfigs) == 0 {
					t.Errorf("expected ManifestConfigs to be set, got none")
					return
				}

				// Verify the number of configs matches the number of manifests — every
				// manifest must have a corresponding ReadOnly config.
				if len(klusterletWork.Spec.ManifestConfigs) != len(klusterletWork.Spec.Workload.Manifests) {
					t.Errorf("expected %d ManifestConfigs (one per manifest), got %d",
						len(klusterletWork.Spec.Workload.Manifests), len(klusterletWork.Spec.ManifestConfigs))
				}

				// Verify all configs have ReadOnly strategy and a non-empty Resource field.
				// A non-empty Resource field is required for the work-agent's resourceMatch
				// function to correctly identify and apply the ReadOnly strategy.
				for i, config := range klusterletWork.Spec.ManifestConfigs {
					if config.UpdateStrategy == nil {
						t.Errorf("ManifestConfig %d has nil UpdateStrategy", i)
						continue
					}
					if config.UpdateStrategy.Type != workv1.UpdateStrategyTypeReadOnly {
						t.Errorf("ManifestConfig %d has UpdateStrategy %s, expected ReadOnly",
							i, config.UpdateStrategy.Type)
					}
					if config.ResourceIdentifier.Resource == "" {
						t.Errorf("ManifestConfig %d has empty Resource field — work-agent resourceMatch will never match this config",
							i)
					}
				}

				// Check CRDs ManifestWork
				crdsWork, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), "test-klusterlet-crds", v1.GetOptions{})
				if err != nil {
					t.Errorf("failed to get CRDs ManifestWork: %v", err)
					return
				}

				if len(crdsWork.Spec.ManifestConfigs) == 0 {
					t.Errorf("expected CRDs ManifestConfigs to be set, got none")
					return
				}

				// Verify the number of configs matches the number of manifests.
				if len(crdsWork.Spec.ManifestConfigs) != len(crdsWork.Spec.Workload.Manifests) {
					t.Errorf("expected %d CRDs ManifestConfigs (one per manifest), got %d",
						len(crdsWork.Spec.Workload.Manifests), len(crdsWork.Spec.ManifestConfigs))
				}

				// Verify CRDs configs have ReadOnly strategy and a non-empty Resource field.
				for i, config := range crdsWork.Spec.ManifestConfigs {
					if config.UpdateStrategy == nil {
						t.Errorf("CRDs ManifestConfig %d has nil UpdateStrategy", i)
						continue
					}
					if config.UpdateStrategy.Type != workv1.UpdateStrategyTypeReadOnly {
						t.Errorf("CRDs ManifestConfig %d has UpdateStrategy %s, expected ReadOnly",
							i, config.UpdateStrategy.Type)
					}
					if config.ResourceIdentifier.Resource == "" {
						t.Errorf("CRDs ManifestConfig %d has empty Resource field — work-agent resourceMatch will never match this config",
							i)
					}
				}
			},
		},
		{
			name: "no disable-auto-import annotation - ManifestWorks have no ManifestConfigs",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{
						Name:       "test",
						Finalizers: []string{constants.ManifestWorkFinalizer},
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				// Check klusterlet ManifestWork
				klusterletWork, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), "test-klusterlet", v1.GetOptions{})
				if err != nil {
					t.Errorf("failed to get klusterlet ManifestWork: %v", err)
					return
				}

				if len(klusterletWork.Spec.ManifestConfigs) != 0 {
					t.Errorf("expected no ManifestConfigs, got %d", len(klusterletWork.Spec.ManifestConfigs))
				}

				// Check CRDs ManifestWork
				crdsWork, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), "test-klusterlet-crds", v1.GetOptions{})
				if err != nil {
					t.Errorf("failed to get CRDs ManifestWork: %v", err)
					return
				}

				if len(crdsWork.Spec.ManifestConfigs) != 0 {
					t.Errorf("expected no CRDs ManifestConfigs, got %d", len(crdsWork.Spec.ManifestConfigs))
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

			workClient := workfake.NewSimpleClientset(c.works...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			ctx := context.TODO()
			r := NewReconcileManifestWork(
				&helpers.ClientHolder{
					RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).
						WithObjects(c.startObjs...).WithStatusSubresource(c.startObjs...).Build(),
					OperatorClient: operatorfake.NewSimpleClientset(),
					KubeClient:     kubeClient,
					WorkClient:     workClient,
				},
				&source.InformerHolder{
					ImportSecretLister:   kubeInformerFactory.Core().V1().Secrets().Lister(),
					KlusterletWorkLister: workInformerFactory.Work().V1().ManifestWorks().Lister(),
				},
				testscheme,
				eventstesting.NewTestingEventRecorder(t),
				helpers.NewManagedClusterEventRecorder(ctx, kubeClient),
			)

			_, err := r.Reconcile(ctx, c.request)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c.validateFunc(t, r.clientHolder.RuntimeClient, r.clientHolder.WorkClient)
		})
	}
}

