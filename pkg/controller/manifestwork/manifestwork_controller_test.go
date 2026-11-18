// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"strings"
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
	"sigs.k8s.io/controller-runtime/pkg/event"
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
		{
			name: "disable-auto-import with existing works - applies ReadOnly without overwriting manifests",
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
			works: func() []runtime.Object {
				// Build ManifestWorks as if they were created without the annotation (no ManifestConfigs)
				clusterWithoutAnnotation := &clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{Name: "test"},
				}
				return createManifestWorks(clusterWithoutAnnotation, testinghelpers.GetImportSecret("test"))
			}(),
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				klusterletWork, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), "test-klusterlet", v1.GetOptions{})
				if err != nil {
					t.Errorf("failed to get klusterlet ManifestWork: %v", err)
					return
				}

				if len(klusterletWork.Spec.ManifestConfigs) == 0 {
					t.Errorf("expected ReadOnly ManifestConfigs to be applied, got none")
					return
				}

				if len(klusterletWork.Spec.ManifestConfigs) != len(klusterletWork.Spec.Workload.Manifests) {
					t.Errorf("expected %d ManifestConfigs (one per manifest), got %d",
						len(klusterletWork.Spec.Workload.Manifests), len(klusterletWork.Spec.ManifestConfigs))
				}

				for i, config := range klusterletWork.Spec.ManifestConfigs {
					if config.UpdateStrategy == nil || config.UpdateStrategy.Type != workv1.UpdateStrategyTypeReadOnly {
						t.Errorf("ManifestConfig %d expected ReadOnly strategy", i)
					}
					if config.ResourceIdentifier.Resource == "" {
						t.Errorf("ManifestConfig %d has empty Resource field", i)
					}
				}

				crdsWork, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), "test-klusterlet-crds", v1.GetOptions{})
				if err != nil {
					t.Errorf("failed to get CRDs ManifestWork: %v", err)
					return
				}

				if len(crdsWork.Spec.ManifestConfigs) == 0 {
					t.Errorf("expected CRDs ReadOnly ManifestConfigs to be applied, got none")
					return
				}

				for i, config := range crdsWork.Spec.ManifestConfigs {
					if config.UpdateStrategy == nil || config.UpdateStrategy.Type != workv1.UpdateStrategyTypeReadOnly {
						t.Errorf("CRDs ManifestConfig %d expected ReadOnly strategy", i)
					}
				}
			},
		},
		{
			name: "disable-auto-import with existing ReadOnly works - no update needed",
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
			works: func() []runtime.Object {
				// Build ManifestWorks as if they were created WITH the annotation (ReadOnly configs)
				clusterWithAnnotation := &clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							"import.open-cluster-management.io/disable-auto-import": "",
						},
					},
				}
				return createManifestWorks(clusterWithAnnotation, testinghelpers.GetImportSecret("test"))
			}(),
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				klusterletWork, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), "test-klusterlet", v1.GetOptions{})
				if err != nil {
					t.Errorf("failed to get klusterlet ManifestWork: %v", err)
					return
				}

				if len(klusterletWork.Spec.ManifestConfigs) == 0 {
					t.Errorf("expected ManifestConfigs to remain set")
					return
				}

				for i, config := range klusterletWork.Spec.ManifestConfigs {
					if config.UpdateStrategy == nil || config.UpdateStrategy.Type != workv1.UpdateStrategyTypeReadOnly {
						t.Errorf("ManifestConfig %d expected ReadOnly strategy to remain", i)
					}
				}
			},
		},
		{
			name: "allow-manifestwork-update clears ReadOnly and keeps the migration payload",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							"import.open-cluster-management.io/disable-auto-import":       "",
							"import.open-cluster-management.io/allow-manifestwork-update": "",
						},
						Finalizers: []string{constants.ManifestWorkFinalizer},
					},
				},
			},
			works: migrationManifestWorks(true),
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				assertManifestWorkPayloadPreserved(t, workClient, false)
			},
		},
		{
			name: "allow-manifestwork-update with Update strategy works leaves the migration payload unchanged",
			startObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: v1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							"import.open-cluster-management.io/disable-auto-import":       "",
							"import.open-cluster-management.io/allow-manifestwork-update": "",
						},
						Finalizers: []string{constants.ManifestWorkFinalizer},
					},
				},
			},
			works: migrationManifestWorks(false),
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, runtimeClient client.Client, workClient workclient.Interface) {
				assertManifestWorkPayloadPreserved(t, workClient, false)
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

const disableAutoImportAnnotation = "import.open-cluster-management.io/disable-auto-import"

// migrationBootstrapManifests is the payload Global Hub writes into the klusterlet
// ManifestWork during a hive migration. It deliberately differs from the import secret
// content, so a rebuild from that secret is detectable.
func migrationBootstrapManifests() []workv1.Manifest {
	return []workv1.Manifest{
		{
			RawExtension: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"migration-bootstrap","namespace":"open-cluster-management-agent"},"data":{"target-hub":"hub-a"}}`),
			},
		},
	}
}

// migrationManifestWorks builds the klusterlet works as they exist in the middle of a
// migration: the klusterlet work carries the migration payload instead of the import
// secret content, and readOnly selects whether those works still have ReadOnly configs.
func migrationManifestWorks(readOnly bool) []runtime.Object {
	works := createManifestWorks(
		&clusterv1.ManagedCluster{ObjectMeta: v1.ObjectMeta{Name: "test"}},
		testinghelpers.GetImportSecret("test"),
	)
	for _, obj := range works {
		work := obj.(*workv1.ManifestWork)
		if strings.HasSuffix(work.Name, constants.KlusterletSuffix) {
			work.Spec.Workload.Manifests = migrationBootstrapManifests()
		}
		work.Spec.ManifestConfigs = buildManifestConfigs(
			&clusterv1.ManagedCluster{
				ObjectMeta: v1.ObjectMeta{
					Name:        "test",
					Annotations: map[string]string{disableAutoImportAnnotation: ""},
				},
			},
			work.Spec.Workload.Manifests,
		)
		if !readOnly {
			work.Spec.ManifestConfigs = nil
		}
	}
	return works
}

// assertManifestWorkPayloadPreserved verifies that the migration payload stored in the
// klusterlet works was not replaced by the import secret content, and that the ReadOnly
// configs match the expected state.
func assertManifestWorkPayloadPreserved(t *testing.T, workClient workclient.Interface, wantReadOnly bool) {
	t.Helper()
	for _, name := range []string{"test-klusterlet", "test-klusterlet-crds"} {
		work, err := workClient.WorkV1().ManifestWorks("test").Get(context.TODO(), name, v1.GetOptions{})
		if err != nil {
			t.Errorf("failed to get %s: %v", name, err)
			continue
		}

		if name == "test-klusterlet" && !helpers.ManifestsEqual(work.Spec.Workload.Manifests, migrationBootstrapManifests()) {
			t.Errorf("%s workload was rebuilt from the import secret", name)
		}

		if !wantReadOnly {
			if len(work.Spec.ManifestConfigs) != 0 {
				t.Errorf("%s expected no ManifestConfigs, got %d", name, len(work.Spec.ManifestConfigs))
			}
			continue
		}

		if len(work.Spec.ManifestConfigs) != len(work.Spec.Workload.Manifests) {
			t.Errorf("%s expected %d ReadOnly ManifestConfigs (one per manifest), got %d", name,
				len(work.Spec.Workload.Manifests), len(work.Spec.ManifestConfigs))
			continue
		}
		for i, config := range work.Spec.ManifestConfigs {
			if config.UpdateStrategy == nil || config.UpdateStrategy.Type != workv1.UpdateStrategyTypeReadOnly {
				t.Errorf("%s ManifestConfig %d expected ReadOnly strategy", name, i)
			}
		}
	}
}

// TestAllowManifestWorkUpdateLifecycle walks a migrated cluster through the update
// window: ReadOnly, editable, and back to ReadOnly, while the migration payload written
// into the klusterlet works must survive unchanged.
func TestAllowManifestWorkUpdateLifecycle(t *testing.T) {
	ctx := context.TODO()

	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test",
			Annotations: map[string]string{disableAutoImportAnnotation: ""},
			Finalizers:  []string{constants.ManifestWorkFinalizer},
		},
	}
	secrets := []runtime.Object{testinghelpers.GetImportSecret("test")}
	works := migrationManifestWorks(true)

	kubeClient := kubefake.NewSimpleClientset(secrets...)
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
	for _, secret := range secrets {
		if err := secretInformer.GetStore().Add(secret); err != nil {
			t.Fatalf("failed to add import secret to the informer store: %v", err)
		}
	}

	workClient := workfake.NewSimpleClientset(works...)
	workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
	workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
	for _, work := range works {
		if err := workInformer.GetStore().Add(work); err != nil {
			t.Fatalf("failed to add manifest work to the informer store: %v", err)
		}
	}

	runtimeClient := fake.NewClientBuilder().WithScheme(testscheme).
		WithObjects(managedCluster).WithStatusSubresource(managedCluster).Build()

	r := NewReconcileManifestWork(
		&helpers.ClientHolder{
			RuntimeClient:  runtimeClient,
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

	reconcileCluster := func(t *testing.T) {
		t.Helper()
		if _, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "test"},
		}); err != nil {
			t.Fatalf("unexpected reconcile error: %v", err)
		}
	}

	setAnnotations := func(t *testing.T, annotations map[string]string) {
		t.Helper()
		current := &clusterv1.ManagedCluster{}
		if err := runtimeClient.Get(ctx, types.NamespacedName{Name: "test"}, current); err != nil {
			t.Fatalf("failed to get managed cluster: %v", err)
		}
		current.Annotations = annotations
		if err := runtimeClient.Update(ctx, current); err != nil {
			t.Fatalf("failed to update managed cluster annotations: %v", err)
		}
	}

	// Start in the disaster-recovery state: ReadOnly works holding a migration payload.
	reconcileCluster(t)
	assertManifestWorkPayloadPreserved(t, workClient, true)

	// The migration flow opens the update window.
	setAnnotations(t, map[string]string{
		disableAutoImportAnnotation:                 "",
		constants.AnnotationAllowManifestWorkUpdate: "",
	})
	reconcileCluster(t)
	assertManifestWorkPayloadPreserved(t, workClient, false)

	// Close the window before the informer sees the config-only writes. The
	// annotation reconcile reads stale ReadOnly work configs and skips the writes.
	setAnnotations(t, map[string]string{disableAutoImportAnnotation: ""})
	reconcileCluster(t)
	assertManifestWorkPayloadPreserved(t, workClient, false)

	// Deliver the config-only updates to the informer. The watch must enqueue
	// another reconcile now that the cache reflects the actual work strategy.
	list, err := workClient.WorkV1().ManifestWorks("test").List(ctx, v1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list manifest works: %v", err)
	}
	for i := range list.Items {
		current := &list.Items[i]
		previous, err := r.informerHolder.KlusterletWorkLister.ManifestWorks(current.Namespace).Get(current.Name)
		if err != nil {
			t.Fatalf("failed to get cached work %s: %v", current.Name, err)
		}
		if err := workInformer.GetStore().Update(current); err != nil {
			t.Fatalf("failed to refresh %s in the informer store: %v", current.Name, err)
		}
		if !klusterletWorkChanged(event.UpdateEvent{ObjectOld: previous, ObjectNew: current}) {
			t.Fatalf("config-only update for %s did not enqueue reconciliation", current.Name)
		}
	}
	reconcileCluster(t)
	assertManifestWorkPayloadPreserved(t, workClient, true)

	// The restored ReadOnly configs must protect every manifest of every work.
	for _, name := range []string{"test-klusterlet", "test-klusterlet-crds"} {
		work, err := workClient.WorkV1().ManifestWorks("test").Get(ctx, name, v1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get %s: %v", name, err)
		}
		if len(work.Spec.ManifestConfigs) == 0 {
			t.Errorf("%s expected ReadOnly ManifestConfigs after the update window closed", name)
		}
	}
}

func TestManifestWorkAnnotationChanged(t *testing.T) {
	disable := "import.open-cluster-management.io/disable-auto-import"
	allow := constants.AnnotationAllowManifestWorkUpdate
	cases := []struct {
		name string
		old  map[string]string
		new  map[string]string
		want bool
	}{
		{name: "unrelated", old: map[string]string{"a": "b"}, new: map[string]string{"a": "c"}, want: false},
		{name: "disable added", old: nil, new: map[string]string{disable: ""}, want: true},
		{name: "disable removed", old: map[string]string{disable: ""}, new: map[string]string{}, want: true},
		{
			name: "allow added",
			old:  map[string]string{disable: ""},
			new:  map[string]string{disable: "", allow: ""},
			want: true,
		},
		{
			name: "allow removed",
			old:  map[string]string{disable: "", allow: ""},
			new:  map[string]string{disable: ""},
			want: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := manifestWorkAnnotationChanged(c.old, c.new); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
