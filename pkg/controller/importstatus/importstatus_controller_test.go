// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importstatus

import (
	"context"
	"testing"
	"time"

	operatorv1 "open-cluster-management.io/api/operator/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
}

func TestReconcile(t *testing.T) {
	managedClusterName := "test"
	cases := []struct {
		name                             string
		objs                             []client.Object
		works                            []runtime.Object
		expectedErr                      bool
		expectedConditionStatus          metav1.ConditionStatus
		expectedConditionReason          string
		expectedImmediateImportCompleted bool
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
						Name: managedClusterName,
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
						},
					},
				},
			},
			works:       []runtime.Object{},
			expectedErr: false,
		},
		{
			name: "deletion managed cluster",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              managedClusterName,
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						Finalizers:        []string{"test"},
					},
				},
			},
			works:       []runtime.Object{},
			expectedErr: false,
		},
		{
			name: "managed cluster import condition not exist",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: managedClusterName,
					},
				},
			},
			works:       []runtime.Object{},
			expectedErr: false,
		},
		{
			name: "managed cluster import condition not running",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: managedClusterName,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
								constants.ConditionReasonManagedClusterImporting, "test"),
						},
					},
				},
			},
			works:       []runtime.Object{},
			expectedErr: false,
		},
		{
			name: "manifestwork not available",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: managedClusterName,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
								constants.ConditionReasonManagedClusterImporting, "test"),
						},
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name: "manifestwork available",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: managedClusterName,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
								constants.ConditionReasonManagedClusterImporting, "test"),
						},
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
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
						Namespace: managedClusterName,
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
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: constants.ConditionReasonManagedClusterImported,
		},
		{
			name: "with immediate-import annotation",
			objs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: managedClusterName,
						Annotations: map[string]string{
							apiconstants.AnnotationImmediateImport: "",
						},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
								constants.ConditionReasonManagedClusterImporting, "test"),
						},
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
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
						Namespace: managedClusterName,
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
			expectedErr:                      false,
			expectedConditionStatus:          metav1.ConditionTrue,
			expectedConditionReason:          constants.ConditionReasonManagedClusterImported,
			expectedImmediateImportCompleted: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset()

			workClient := workfake.NewSimpleClientset(c.works...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			ctx := context.TODO()
			r := NewReconcileImportStatus(
				fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.objs...).WithStatusSubresource(c.objs...).Build(),
				kubeClient,
				workClient,
				helpers.NewManagedClusterEventRecorder(ctx, kubeClient),
			)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: managedClusterName,
				},
			}

			_, err := r.Reconcile(ctx, req)
			if c.expectedErr && err == nil {
				t.Errorf("expected error, but failed")
			}
			if !c.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if c.expectedConditionReason != "" {
				managedCluster := &clusterv1.ManagedCluster{}
				err = r.client.Get(ctx,
					types.NamespacedName{
						Name: managedClusterName,
					},
					managedCluster)
				if err != nil {
					t.Errorf("get managed cluster error: %v", err)
				}

				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions,
					constants.ConditionManagedClusterImportSucceeded,
				)
				if condition.Status != c.expectedConditionStatus {
					t.Errorf("Expect condition status %s, got %s", c.expectedConditionStatus, condition.Status)
				}
				if condition.Reason != c.expectedConditionReason {
					t.Errorf("Expect condition reason %s, got %s, message: %s",
						c.expectedConditionReason, condition.Reason, condition.Message)
				}
			}

			if c.expectedImmediateImportCompleted {
				managedCluster := &clusterv1.ManagedCluster{}
				err = r.client.Get(ctx,
					types.NamespacedName{
						Name: managedClusterName,
					},
					managedCluster)
				if err != nil {
					t.Errorf("get managed cluster error: %v", err)
				}
				immediateImportValue := managedCluster.Annotations[apiconstants.AnnotationImmediateImport]
				if immediateImportValue != apiconstants.AnnotationValueImmediateImportCompleted {
					t.Errorf("Expect immediate-import completed, got %q", immediateImportValue)
				}
			}
		})
	}
}

// TestReconcileStaleImportReImportGate reproduces the ACM-49455 live finding: during a
// stale-import-success re-import the klusterlet ManifestWork can still report Available=True from
// the previous agent incarnation. The import-status controller must not complete the re-import on
// that stale availability; it must wait until the managed cluster becomes available again (fresh
// agent), then complete and clear the re-import stamp.
func TestReconcileStaleImportReImportGate(t *testing.T) {
	const name = "test"
	stamp := metav1.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	staleAvailable := metav1.NewTime(stamp.Add(-10 * time.Minute)) // old agent's ack, predates re-import
	freshAvailable := metav1.NewTime(stamp.Add(2 * time.Minute))   // new agent after re-import

	// klusterlet works reporting Available=True (stale ack never resets on the hub)
	staleWorks := func() []runtime.Object {
		mkWork := func(suffix string) *workv1.ManifestWork {
			return &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name + "-" + suffix,
					Namespace: name,
					Labels:    map[string]string{constants.KlusterletWorksLabel: "true"},
				},
				Status: workv1.ManifestWorkStatus{
					Conditions: []metav1.Condition{{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue}},
				},
			}
		}
		return []runtime.Object{mkWork("klusterlet-crds"), mkWork("klusterlet")}
	}

	newCluster := func(availableStatus metav1.ConditionStatus, availableTime metav1.Time) *clusterv1.ManagedCluster {
		return &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Annotations: map[string]string{
					apiconstants.AnnotationImmediateImport:   "",
					constants.StaleImportReferenceAnnotation: stamp.UTC().Format(time.RFC3339),
				},
			},
			Status: clusterv1.ManagedClusterStatus{
				Conditions: []metav1.Condition{
					helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
						constants.ConditionReasonManagedClusterImporting, "test"),
					{
						Type:               clusterv1.ManagedClusterConditionAvailable,
						Status:             availableStatus,
						LastTransitionTime: availableTime,
					},
				},
			},
		}
	}

	leaseRenewedAt := func(t metav1.Time) *coordinationv1.Lease {
		mt := metav1.NewMicroTime(t.Time)
		return &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: helpers.ManagedClusterLeaseName, Namespace: name},
			Spec:       coordinationv1.LeaseSpec{RenewTime: &mt},
		}
	}

	cases := []struct {
		name               string
		cluster            *clusterv1.ManagedCluster
		lease              *coordinationv1.Lease
		expectCompleted    bool // immediate-import flipped to Completed
		expectStampPresent bool
	}{
		{
			name:               "stale work available, cluster unknown, lease frozen - gate holds",
			cluster:            newCluster(metav1.ConditionUnknown, staleAvailable),
			lease:              leaseRenewedAt(staleAvailable),
			expectCompleted:    false,
			expectStampPresent: true,
		},
		{
			name:               "stale work available, cluster available but lease frozen before re-import - gate holds",
			cluster:            newCluster(metav1.ConditionTrue, staleAvailable),
			lease:              leaseRenewedAt(staleAvailable),
			expectCompleted:    false,
			expectStampPresent: true,
		},
		{
			name:               "cluster became available after re-import (fallback, no lease) - completes and clears stamp",
			cluster:            newCluster(metav1.ConditionTrue, freshAvailable),
			lease:              nil,
			expectCompleted:    true,
			expectStampPresent: false,
		},
		{
			// CodeRabbit case: healthy ImportOnly cluster, Available never re-transitions
			// (old LastTransitionTime) but the live agent keeps renewing the lease.
			name:               "continuously available cluster with fresh lease - completes and clears stamp",
			cluster:            newCluster(metav1.ConditionTrue, staleAvailable),
			lease:              leaseRenewedAt(freshAvailable),
			expectCompleted:    true,
			expectStampPresent: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			works := staleWorks()
			kubeObjs := []runtime.Object{}
			if c.lease != nil {
				kubeObjs = append(kubeObjs, c.lease)
			}
			kubeClient := kubefake.NewSimpleClientset(kubeObjs...)
			workClient := workfake.NewSimpleClientset(works...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, w := range works {
				workInformer.GetStore().Add(w)
			}

			ctx := context.TODO()
			r := NewReconcileImportStatus(
				fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.cluster).WithStatusSubresource(c.cluster).Build(),
				kubeClient,
				workClient,
				helpers.NewManagedClusterEventRecorder(ctx, kubeClient),
			)

			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// When the gate holds (not completed), the controller must requeue so a later lease
			// renewal is re-checked even without a watched ManagedCluster/ManifestWork event.
			if !c.expectCompleted && result.RequeueAfter <= 0 {
				t.Errorf("expected a bounded requeue while the gate holds, got %+v", result)
			}

			mc := &clusterv1.ManagedCluster{}
			if err := r.client.Get(ctx, types.NamespacedName{Name: name}, mc); err != nil {
				t.Fatalf("get managed cluster error: %v", err)
			}

			completed := mc.Annotations[apiconstants.AnnotationImmediateImport] == apiconstants.AnnotationValueImmediateImportCompleted
			if completed != c.expectCompleted {
				t.Errorf("expected immediate-import completed=%v, got %v (annotations: %v)",
					c.expectCompleted, completed, mc.Annotations)
			}

			_, stampPresent := mc.Annotations[constants.StaleImportReferenceAnnotation]
			if stampPresent != c.expectStampPresent {
				t.Errorf("expected stale-import-reference present=%v, got %v", c.expectStampPresent, stampPresent)
			}

			importCond := meta.FindStatusCondition(mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
			if c.expectCompleted {
				if importCond == nil || importCond.Status != metav1.ConditionTrue {
					t.Errorf("expected ImportSucceeded=True on completion, got %+v", importCond)
				}
			} else {
				// gate held: must not have been flipped to a success
				if importCond != nil && importCond.Status == metav1.ConditionTrue {
					t.Errorf("gate should have held, but ImportSucceeded was set True: %+v", importCond)
				}
			}
		})
	}
}

// TestReconcileStaleImportReImportLeaseRenewalConverges covers the race where the gate first sees a
// stale lease and holds, and later only the lease is renewed (no ManagedCluster or ManifestWork
// event). The controller must requeue so the renewal is re-checked and the re-import converges.
func TestReconcileStaleImportReImportLeaseRenewalConverges(t *testing.T) {
	const name = "test"
	stamp := metav1.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	staleRenew := metav1.NewTime(stamp.Add(-5 * time.Minute))
	freshRenew := metav1.NewTime(stamp.Add(2 * time.Minute))

	mkWork := func(suffix string) *workv1.ManifestWork {
		return &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name: name + "-" + suffix, Namespace: name,
				Labels: map[string]string{constants.KlusterletWorksLabel: "true"},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue}},
			},
		}
	}
	works := []runtime.Object{mkWork("klusterlet-crds"), mkWork("klusterlet")}

	// cluster is available (True) but from before the re-import; only the lease will prove freshness
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				apiconstants.AnnotationImmediateImport:   "",
				constants.StaleImportReferenceAnnotation: stamp.UTC().Format(time.RFC3339),
			},
		},
		Status: clusterv1.ManagedClusterStatus{
			Conditions: []metav1.Condition{
				helpers.NewManagedClusterImportSucceededCondition(metav1.ConditionFalse,
					constants.ConditionReasonManagedClusterImporting, "test"),
				{
					Type:               clusterv1.ManagedClusterConditionAvailable,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: staleRenew,
				},
			},
		},
	}

	staleMicro := metav1.NewMicroTime(staleRenew.Time)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: helpers.ManagedClusterLeaseName, Namespace: name},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &staleMicro},
	}

	ctx := context.TODO()
	kubeClient := kubefake.NewSimpleClientset(lease)
	workClient := workfake.NewSimpleClientset(works...)
	r := NewReconcileImportStatus(
		fake.NewClientBuilder().WithScheme(testscheme).WithObjects(cluster).WithStatusSubresource(cluster).Build(),
		kubeClient,
		workClient,
		helpers.NewManagedClusterEventRecorder(ctx, kubeClient),
	)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}

	// First reconcile: stale lease -> gate holds and requeues.
	result, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected a requeue while the lease is stale, got %+v", result)
	}
	mc := &clusterv1.ManagedCluster{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: name}, mc); err != nil {
		t.Fatal(err)
	}
	if mc.Annotations[apiconstants.AnnotationImmediateImport] == apiconstants.AnnotationValueImmediateImportCompleted {
		t.Fatalf("gate should have held on the stale lease, but the re-import completed")
	}

	// Only the lease is renewed (no ManagedCluster or ManifestWork change). The requeue re-runs
	// the reconcile, which must now complete the re-import.
	freshMicro := metav1.NewMicroTime(freshRenew.Time)
	lease.Spec.RenewTime = &freshMicro
	if _, err := kubeClient.CoordinationV1().Leases(name).Update(ctx, lease, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update lease error: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("unexpected error on second reconcile: %v", err)
	}
	if err := r.client.Get(ctx, types.NamespacedName{Name: name}, mc); err != nil {
		t.Fatal(err)
	}
	if mc.Annotations[apiconstants.AnnotationImmediateImport] != apiconstants.AnnotationValueImmediateImportCompleted {
		t.Errorf("expected re-import to complete after lease renewal, annotations: %v", mc.Annotations)
	}
	if _, present := mc.Annotations[constants.StaleImportReferenceAnnotation]; present {
		t.Errorf("expected stale-import-reference cleared after completion, annotations: %v", mc.Annotations)
	}
}
