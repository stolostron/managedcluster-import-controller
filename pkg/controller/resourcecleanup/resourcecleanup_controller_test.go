package resourcecleanup

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	testScheme = scheme.Scheme
	now        = metav1.Now()
)

func init() {
	testScheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testScheme.AddKnownTypes(addonv1alpha1.SchemeGroupVersion, &addonv1alpha1.ManagedClusterAddOn{})
	testScheme.AddKnownTypes(addonv1alpha1.SchemeGroupVersion, &addonv1alpha1.ManagedClusterAddOnList{})
	testScheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	testScheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWorkList{})
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name           string
		request        reconcile.Request
		runtimeObjects []client.Object
		kubeObjects    []runtime.Object
		works          []runtime.Object
		requeue        bool
		validateFunc   func(t *testing.T, clientHolder *helpers.ClientHolder)
	}{
		{
			name:    "default cluster is deleting and no resources",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("unexpected no cluster,but got error: %v", err)
				}
			},
		},
		{
			name:    "default cluster is deleting and klustereletCRD work is not deleting and no finalizer",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
					},
				},
			},
			requeue: true,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); errors.IsNotFound(err) {
					t.Errorf("unexpected no cluster,but got error: %v", err)
				}
			},
		},
		{
			name:    "default cluster is deleting and klustereletCRD work is deleting",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-klusterlet-crds",
						Namespace:         "test",
						DeletionTimestamp: &now,
						Finalizers:        []string{workv1.ManifestWorkFinalizer},
					},
				},
			},
			requeue: true,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); errors.IsNotFound(err) {
					t.Errorf("unexpected no cluster,but got error: %v", err)
				}
			},
		},
		{
			name:    "default cluster is deleting and klustereletCRD work is deleting and have deleting condition",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-klusterlet-crds",
						Namespace:         "test",
						DeletionTimestamp: &now,
						Finalizers:        []string{workv1.ManifestWorkFinalizer},
					},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							metav1.Condition{
								Type:   workv1.WorkDeleting,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("expected no cluster,but got error: %v", err)
				}
			},
		},
		{
			name:    "default cluster is deleting and have work",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "work1", Namespace: "test"}},
			},
			requeue: true,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); err != nil {
					t.Errorf("unexpected cluster,but got error: %v", err)
				}
				if len(managedCluster.Finalizers) != 2 {
					t.Errorf("expected 2 managed cluster finalizers,but got %v", len(managedCluster.Finalizers))
				}
				ic := meta.FindStatusCondition(managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if ic == nil {
					t.Errorf("expected ManagedClusterImportSucceeded condition, but got nil")
				}
				if ic.Reason != constants.ConditionReasonManagedClusterDetaching {
					t.Errorf("expected deataching condition reason, but got %v", ic.Reason)
				}
			},
		},
		{
			name:    "default cluster is deleting and have addon",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{Name: "addon1", Namespace: "test"}},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			},
			requeue: true,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); err != nil {
					t.Errorf("unexpected cluster,but got error: %v", err)
				}
				if len(managedCluster.Finalizers) != 2 {
					t.Errorf("expected 2 managed cluster finalizers,but got %v", len(managedCluster.Finalizers))
				}
				ic := meta.FindStatusCondition(managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if ic == nil {
					t.Errorf("expected ManagedClusterImportSucceeded condition, but got nil")
				}
				if ic.Reason != constants.ConditionReasonManagedClusterDetaching {
					t.Errorf("expected deataching condition reason, but got %v", ic.Reason)
				}
			},
		},
		{
			name:    "cluster is not found",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon1", Namespace: "test", Finalizers: []string{"test"}},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
					Name:       "open-cluster-management:managedcluster:test:work",
					Namespace:  "test",
					Finalizers: []string{workv1.ManifestWorkFinalizer},
				}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work1", Namespace: "test", Finalizers: []string{"test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				addons, _ := helpers.ListManagedClusterAddons(context.TODO(), clientHolder.RuntimeClient, "test")
				if len(addons.Items) != 0 {
					t.Errorf("expected no addon,but got %v", len(addons.Items))
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("test").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected no work,but got %v", len(works.Items))
				}
				workRoleBinding, _ := helpers.GetWorkRoleBinding(context.TODO(), clientHolder.RuntimeClient, "test")
				if workRoleBinding != nil {
					t.Errorf("expected no workRolebinding,but got %v", workRoleBinding)
				}
			},
		},
		{
			name:    "cluster is Unavailable",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionUnknown,
							}},
					},
				},
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon1", Namespace: "test", Finalizers: []string{"test"}},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
					Name:       "open-cluster-management:managedcluster:test:work",
					Namespace:  "test",
					Finalizers: []string{workv1.ManifestWorkFinalizer},
				}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work1", Namespace: "test", Finalizers: []string{"test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("unexpected no cluster,but got error: %v", err)
				}
				addons, _ := helpers.ListManagedClusterAddons(context.TODO(), clientHolder.RuntimeClient, "test")
				if len(addons.Items) != 0 {
					t.Errorf("expected no addon,but got %v", len(addons.Items))
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("test").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected no work,but got %v", len(works.Items))
				}
				workRoleBinding, _ := helpers.GetWorkRoleBinding(context.TODO(), clientHolder.RuntimeClient, "test")
				if workRoleBinding != nil {
					t.Errorf("expected no workRolebinding,but got %v", workRoleBinding)
				}
			},
		},
		{
			name:    "cluster is not accepted",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: false,
					},
				},
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon1", Namespace: "test", Finalizers: []string{"test"}},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
					Name:       "open-cluster-management:managedcluster:test:work",
					Namespace:  "test",
					Finalizers: []string{workv1.ManifestWorkFinalizer},
				}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work1", Namespace: "test", Finalizers: []string{"test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("unexpected no cluster,but got error: %v", err)
				}
				addons, _ := helpers.ListManagedClusterAddons(context.TODO(), clientHolder.RuntimeClient, "test")
				if len(addons.Items) != 0 {
					t.Errorf("expected no addon,but got %v", len(addons.Items))
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("test").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected no work,but got %v", len(works.Items))
				}
				workRoleBinding, _ := helpers.GetWorkRoleBinding(context.TODO(), clientHolder.RuntimeClient, "test")
				if workRoleBinding != nil {
					t.Errorf("expected no workRolebinding,but got %v", workRoleBinding)
				}
			},
		},
		{
			name:    "hosted cluster is deleting and no resources",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						}, Finalizers: []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("unexpected no cluster,but got error: %v", err)
				}
			},
		},
		{
			name:    "hosted cluster is deleting and have work",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "work1", Namespace: "test"}},
			},
			requeue: true,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); err != nil {
					t.Errorf("unexpected cluster,but got error: %v", err)
				}
				if len(managedCluster.Finalizers) != 2 {
					t.Errorf("expected 2 managed cluster finalizers,but got %v", len(managedCluster.Finalizers))
				}
				ic := meta.FindStatusCondition(managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if ic == nil {
					t.Errorf("expected ManagedClusterImportSucceeded condition, but got nil")
				}
				if ic.Reason != constants.ConditionReasonManagedClusterDetaching {
					t.Errorf("expected deataching condition reason, but got %v", ic.Reason)
				}
			},
		},
		{
			name:    "hosted cluster is deleting and have hosting work",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hosting"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "work1", Namespace: "hosting"}},
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "work2",
					Namespace: "hosting", Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("unexpected no cluster,but got error: %v", err)
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("hosting").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 1 {
					t.Errorf("expected 1 work in hosting cluster ns,but got %v", len(works.Items))
				}
			},
		},
		{
			name:    "hosted cluster is deleting and have klusterlet work and other work",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hosting"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "work1", Namespace: "hosting",
					Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: helpers.HostedKlusterletManifestWorkName("test"),
					Namespace: "hosting", Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: helpers.HostedManagedKubeConfigManifestWorkName("test"),
					Namespace: "hosting", Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
			},
			requeue: true,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); errors.IsNotFound(err) {
					t.Errorf("the cluster should not be deleted")
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("hosting").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 2 {
					t.Errorf("expected 2 works in hosting cluster ns,but got %v", len(works.Items))
				}
			},
		},
		{
			name:    "hosted cluster is deleting and only have klusterlet and kubeconfig work",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hosting"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: helpers.HostedKlusterletManifestWorkName("test"),
					Namespace: "hosting", Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: helpers.HostedManagedKubeConfigManifestWorkName("test"),
					Namespace: "hosting", Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
			},
			requeue: true,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); errors.IsNotFound(err) {
					t.Errorf("the cluster should not be deleted")
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("hosting").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 1 {
					t.Errorf("expected no works in hosting cluster ns,but got %v", len(works.Items))
				}
			},
		},
		{
			name:    "hosted cluster is deleting and only have klusterlet work",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hosting"}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: helpers.HostedKlusterletManifestWorkName("test"),
					Namespace: "hosting", Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("the cluster should be deleted")
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("hosting").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected no works in hosting cluster ns,but got %v", len(works.Items))
				}
			},
		},
		{
			name:    "hosted cluster is Unavailable and there is work on hosting cluster",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionUnknown,
							}},
					},
				},
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon1", Namespace: "test", Finalizers: []string{"test"}},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
					Name:       "open-cluster-management:managedcluster:test:work",
					Namespace:  "test",
					Finalizers: []string{workv1.ManifestWorkFinalizer},
				}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work1", Namespace: "test", Finalizers: []string{"test"}}},
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work2", Namespace: "hosting", Finalizers: []string{"test"},
					Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("expected no cluster,but got error: %v", err)
				}
				addons, _ := helpers.ListManagedClusterAddons(context.TODO(), clientHolder.RuntimeClient, "test")
				if len(addons.Items) != 0 {
					t.Errorf("expected no addon,but got %v", len(addons.Items))
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("test").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected no work,but got %v", len(works.Items))
				}
				works, _ = clientHolder.WorkClient.WorkV1().ManifestWorks("hosting").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected 0 work,but got %v", len(works.Items))
				}
				workRoleBinding, _ := helpers.GetWorkRoleBinding(context.TODO(), clientHolder.RuntimeClient, "test")
				if workRoleBinding != nil {
					t.Errorf("expected no workRolebinding,but got %v", workRoleBinding)
				}
			},
		},
		{
			name:    "hosted cluster is Unavailable and no work on hosting cluster",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionUnknown,
							}},
					},
				},
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon1", Namespace: "test", Finalizers: []string{"test"}},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
					Name:       "open-cluster-management:managedcluster:test:work",
					Namespace:  "test",
					Finalizers: []string{workv1.ManifestWorkFinalizer},
				}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work1", Namespace: "test", Finalizers: []string{"test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("expected no cluster,but got error: %v", err)
				}
				addons, _ := helpers.ListManagedClusterAddons(context.TODO(), clientHolder.RuntimeClient, "test")
				if len(addons.Items) != 0 {
					t.Errorf("expected no addon,but got %v", len(addons.Items))
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("test").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected no work,but got %v", len(works.Items))
				}

				workRoleBinding, _ := helpers.GetWorkRoleBinding(context.TODO(), clientHolder.RuntimeClient, "test")
				if workRoleBinding != nil {
					t.Errorf("expected no workRolebinding,but got %v", workRoleBinding)
				}
			},
		},
		{
			name:    "hosted cluster is not accepted",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}},
			runtimeObjects: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "hosting",
						},
						Finalizers:        []string{constants.ImportFinalizer, constants.ManifestWorkFinalizer},
						DeletionTimestamp: &now,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: false,
					},
				},
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon1", Namespace: "test", Finalizers: []string{"test"}},
				},
			},
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
					Name:       "open-cluster-management:managedcluster:test:work",
					Namespace:  "test",
					Finalizers: []string{workv1.ManifestWorkFinalizer},
				}},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work1", Namespace: "test", Finalizers: []string{"test"}}},
				&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
					Name: "work2", Namespace: "hosting", Finalizers: []string{"test"},
					Labels: map[string]string{constants.HostedClusterLabel: "test"}}},
			},
			requeue: false,
			validateFunc: func(t *testing.T, clientHolder *helpers.ClientHolder) {
				managedCluster := &clusterv1.ManagedCluster{}
				if err := clientHolder.RuntimeClient.Get(context.TODO(),
					types.NamespacedName{Name: "test"}, managedCluster); !errors.IsNotFound(err) {
					t.Errorf("expected no cluster,but got error: %v", err)
				}
				addons, _ := helpers.ListManagedClusterAddons(context.TODO(), clientHolder.RuntimeClient, "test")
				if len(addons.Items) != 0 {
					t.Errorf("expected no addon,but got %v", len(addons.Items))
				}
				works, _ := clientHolder.WorkClient.WorkV1().ManifestWorks("test").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected no work,but got %v", len(works.Items))
				}
				works, _ = clientHolder.WorkClient.WorkV1().ManifestWorks("hosting").List(context.TODO(), metav1.ListOptions{})
				if len(works.Items) != 0 {
					t.Errorf("expected 0 work,but got %v", len(works.Items))
				}
				workRoleBinding, _ := helpers.GetWorkRoleBinding(context.TODO(), clientHolder.RuntimeClient, "test")
				if workRoleBinding != nil {
					t.Errorf("expected no workRolebinding,but got %v", workRoleBinding)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			var kubeClient kubernetes.Interface
			if len(c.kubeObjects) != 0 {
				kubeClient = kubefake.NewSimpleClientset(c.kubeObjects...)
			} else {
				kubeClient = kubefake.NewSimpleClientset()
			}

			runtimeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(c.runtimeObjects...).
				WithStatusSubresource(c.runtimeObjects...).Build()

			workClient := workfake.NewSimpleClientset(c.works...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			ctx := context.TODO()
			clientHolder := &helpers.ClientHolder{
				RuntimeClient: runtimeClient,
				KubeClient:    kubeClient,
				WorkClient:    workClient,
			}
			r := NewReconcileResourceCleanup(
				clientHolder,
				eventstesting.NewTestingEventRecorder(t),
				helpers.NewManagedClusterEventRecorder(ctx, kubeClient),
			)

			rst, err := r.Reconcile(ctx, c.request)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if rst.RequeueAfter == 0 && c.requeue {
				t.Errorf("expect requeue, but got nothing")
			}

			if rst.RequeueAfter != 0 && !c.requeue {
				t.Errorf("expect no requeue, but got requeue")
			}

			c.validateFunc(t, clientHolder)
		})
	}
}
