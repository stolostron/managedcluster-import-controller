// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hosted

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"open-cluster-management.io/api/addon/v1alpha1"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

var (
	testscheme = scheme.Scheme
)

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.ManagedClusterAddOnList{})
	testscheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.ManagedClusterAddOn{})
}

func TestReconcile(t *testing.T) {
	trueString := "True"
	cases := []struct {
		name         string
		runtimeObjs  []client.Object  // used by clientHolder.RuntimeClient
		kubeObjs     []runtime.Object // used by clientHolder.KubeClient
		workObjs     []runtime.Object
		request      reconcile.Request
		vaildateFunc func(t *testing.T, reconcileResult reconcile.Result, reconcoleErr error, clientHolder *helpers.ClientHolder)
	}{
		// managedcluster is not found, expect to do nothing
		{
			name:        "managedcluster is not found",
			runtimeObjs: []client.Object{},
			kubeObjs:    []runtime.Object{},
			workObjs:    []runtime.Object{},
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
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeDefault),
						},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{},
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
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
						},
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      "manifest1",
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
				managedCluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionFalse {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterWaitForImporting {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
				}
			},
		},
		{
			name: "managedcluster is Hosted mode, but hosting cluster is not a managed cluster of the hub",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1", // hosting cluster name
						},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{},
			request:  reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(
				t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				managedcluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedcluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				condition := meta.FindStatusCondition(
					managedcluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionFalse {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterImporting {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
				}
			},
		},
		{
			name: "managedcluster is Hosted mode, but hosting cluster is being deleted",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1", // hosting cluster name
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "cluster1",
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						Finalizers:        []string{"test"},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{},
			request:  reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(
				t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				managedcluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedcluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				condition := meta.FindStatusCondition(
					managedcluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionFalse {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterImportFailed {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
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
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1", // hosting cluster name
						},
						Finalizers: []string{constants.ManifestWorkFinalizer},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{},
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
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
						DeletionTimestamp: &metav1.Time{Time: time.Now()}, // managedCluster is deleted
						// need the test finalizer, otherwise the managedcluster can't be created since the deletion timestamp is set
						Finalizers: []string{"test"},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-klusterlet",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				managedcluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedcluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				// expect finalizer is removed
				if len(managedcluster.Finalizers) != 0 && managedcluster.Finalizers[0] != "test" {
					t.Errorf("expect no finalizer added, but get %v", managedcluster.Finalizers)
				}

				// expect hosted manifestworks are deleted
				manifestworks, err := ch.WorkClient.WorkV1().ManifestWorks("cluster1").List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if len(manifestworks.Items) != 0 {
					t.Errorf("expect no manifestwork, but get %v", manifestworks.Items)
				}
			},
		},
		{
			name: "managedcluster is Hosted mode, and managedCluster is deleting, the non-addon manifestworks are deleted",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
						DeletionTimestamp: &metav1.Time{Time: time.Now()}, // managedCluster is deleted
						Finalizers:        []string{"test"},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      "manifestwork",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-klusterlet",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				managedcluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedcluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				// expect hosted manifestworks are deleted
				manifestworks, err := ch.WorkClient.WorkV1().ManifestWorks("test").List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if len(manifestworks.Items) != 0 {
					t.Errorf("expect no manifestwork, but get %v", manifestworks.Items)
				}
			},
		},
		{
			name: "managedcluster is Hosted mode, and is deleting" +
				"the hosting klusterlet manifestworks are not force deleted",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
						DeletionTimestamp: &metav1.Time{Time: time.Now()}, // managedCluster is deleted
						Finalizers:        []string{"test"},
					},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []metav1.Condition{
							{
								Type:    clusterv1.ManagedClusterConditionAvailable,
								Status:  metav1.ConditionFalse,
								Message: "unavailable",
								Reason:  "Test",
							},
						},
					},
				},
				&addonapiv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  "test",
						Finalizers: []string{addonv1alpha1.AddonHostingManifestFinalizer},
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{
				// manifestworks
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-klusterlet",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error,
				ch *helpers.ClientHolder) {
				managedcluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedcluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				// expect hosting manifestworks are not deleted
				hosgintManifestworks, err := ch.WorkClient.WorkV1().ManifestWorks("cluster1").List(
					context.TODO(), metav1.ListOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(hosgintManifestworks.Items) != 2 {
					t.Errorf("expect 2 hosint manifestwork, but get %v", hosgintManifestworks.Items)
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
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
				},
			},
			kubeObjs: []runtime.Object{},
			workObjs: []runtime.Object{
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
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}

				managedCluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionFalse {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterImporting {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
				}
				if !strings.Contains(condition.Message, "Wait for import secret to be created") {
					t.Errorf("unexpected condition message: %v", condition.Message)
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
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
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
			workObjs: []runtime.Object{
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
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result,
				reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
				managedCluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionFalse {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterImportFailed {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
				}
				if !strings.Contains(condition.Message, "Import secret is invalid") {
					t.Errorf("unexpected condition message: %v", condition.Message)
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
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
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
			workObjs: []runtime.Object{
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
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
				managedCluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionFalse {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterImporting {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
				}
				if !strings.Contains(condition.Message,
					"Wait for importing resources to be available on the hosting cluster") {
					t.Errorf("unexpected condition message: %v", condition.Message)
				}
			},
		},
		// managedcluster is Hosted mode, klusterlet available
		{
			name: "managedcluster is Hosted mode, klusterlet available",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
				},
			},
			kubeObjs: []runtime.Object{
				testinghelpers.GetHostedImportSecret("test"),
			},
			workObjs: []runtime.Object{
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
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
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
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
				managedCluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionFalse {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterImporting {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
				}
				if !strings.Contains(condition.Message,
					"Wait for the user to provide the external managed kubeconfig") {
					t.Errorf("unexpected condition message: %v", condition.Message)
				}
			},
		},
		{
			name: "managedcluster is Hosted mode, no auto import secret, but external managed kubeconfig is created",
			runtimeObjs: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
							constants.HostingClusterNameAnnotation:   "cluster1",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster1",
					},
				},
			},
			kubeObjs: []runtime.Object{
				testinghelpers.GetHostedImportSecret("test"),
			},
			workObjs: []runtime.Object{
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
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkAvailable,
								Status: metav1.ConditionTrue,
							},
						},
						ResourceStatus: workv1.ManifestResourceStatus{
							Manifests: []workv1.ManifestCondition{
								{
									StatusFeedbacks: workv1.StatusFeedbackResult{
										Values: []workv1.FeedbackValue{
											{
												Name: "ReadyToApply-status",
												Value: workv1.FieldValue{
													Type:   workv1.String,
													String: &trueString,
												},
											},
										},
									},
									ResourceMeta: workv1.ManifestResourceMeta{
										Group: operatorv1.GroupName,
										Kind:  "Klusterlet",
										Name:  hostedKlusterletCRName("test"),
									},
								},
							},
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cluster1",
						Name:      "test-hosted-kubeconfig",
						Labels: map[string]string{
							constants.HostedClusterLabel: "test",
						},
					},
				},
			},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}, // managedcluster name
			vaildateFunc: func(t *testing.T, reconcileResult reconcile.Result, reconcileErr error, ch *helpers.ClientHolder) {
				if reconcileErr != nil {
					t.Errorf("unexpected error: %v", reconcileErr)
				}
				managedCluster := &clusterv1.ManagedCluster{}
				err := ch.RuntimeClient.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
				if condition.Status != metav1.ConditionTrue {
					t.Errorf("unexpected condition status: %v", condition.Status)
				}
				if condition.Reason != constants.ConditionReasonManagedClusterImported {
					t.Errorf("unexpected condition reason: %v", condition.Reason)
				}
			},
		},
		// TODO: add auto import secret test cases
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(c.kubeObjs...)
			kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
			secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
			for _, secret := range c.kubeObjs {
				secretInformer.GetStore().Add(secret)
			}

			workClient := workfake.NewSimpleClientset(c.workObjs...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.workObjs {
				workInformer.GetStore().Add(work)
			}

			r := &ReconcileHosted{
				clientHolder: &helpers.ClientHolder{
					RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).
						WithObjects(c.runtimeObjs...).WithStatusSubresource(c.runtimeObjs...).Build(),
					KubeClient: kubeClient,
					WorkClient: workClient,
				},
				informerHolder: &source.InformerHolder{
					ImportSecretLister:     kubeInformerFactory.Core().V1().Secrets().Lister(),
					AutoImportSecretLister: kubeInformerFactory.Core().V1().Secrets().Lister(),
					HostedWorkLister:       workInformerFactory.Work().V1().ManifestWorks().Lister(),
				},
				recorder: eventstesting.NewTestingEventRecorder(t),
				scheme:   testscheme,
			}
			response, err := r.Reconcile(context.Background(), c.request)
			c.vaildateFunc(t, response, err, r.clientHolder)
		})
	}
}
