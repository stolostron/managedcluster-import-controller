// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusternamespacedeletion

import (
	"context"
	"testing"
	"time"

	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	clustercontroller "github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileWaitsForManifestWorks(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		clusterv1.AddToScheme,
		addonv1alpha1.AddToScheme,
		hyperv1beta1.AddToScheme,
		hivev1.AddToScheme,
		asv1beta1.AddToScheme,
		capiv1beta1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}

	manifestWorkRequeuePeriod = 10 * time.Second

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "cluster1",
		Labels: map[string]string{clustercontroller.ClusterLabel: "cluster1"},
	}}
	runtimeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
	workClient := workfake.NewSimpleClientset(&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
		Name:      "work1",
		Namespace: "cluster1",
	}})
	r := &ReconcileClusterNamespaceDeletion{
		client:     runtimeClient,
		apiReader:  runtimeClient,
		workClient: workClient,
		recorder:   eventstesting.NewTestingEventRecorder(t),
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.RequeueAfter != manifestWorkRequeuePeriod {
		t.Fatalf("expected requeue while manifestworks remain, got %v", result)
	}
	if err := runtimeClient.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, &corev1.Namespace{}); err != nil {
		t.Fatalf("expected namespace to remain, got %v", err)
	}

	if err := workClient.WorkV1().ManifestWorks("cluster1").Delete(context.Background(), "work1", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	result, err = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}})
	if err != nil {
		t.Fatalf("reconcile after manifestwork deletion: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("expected namespace deletion once manifestworks are gone, got %v", result)
	}
	err = runtimeClient.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, &corev1.Namespace{})
	if !errors.IsNotFound(err) {
		t.Fatalf("expected namespace to be deleted, got %v", err)
	}
}
