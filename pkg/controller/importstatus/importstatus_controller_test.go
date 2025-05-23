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
