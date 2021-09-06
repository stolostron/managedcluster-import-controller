// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncset

import (
	"context"
	"testing"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.SyncSet{})
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		startObjs    []runtime.Object
		request      reconcile.Request
		validateFunc func(t *testing.T, client runtimeclient.Client)
	}{
		{
			name:      "no clusters",
			startObjs: []runtime.Object{},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client) {
				// do nothing
			},
		},
		{
			name: "no syncsets",
			startObjs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client) {
				// do nothing
			},
		},
		{
			name: "delete syncset without upsert mode",
			startObjs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&hivev1.SyncSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
					},
					Spec: hivev1.SyncSetSpec{},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-klusterlet",
					Namespace: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client) {
				cs := &hivev1.SyncSet{}
				if err := client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "test-klusterlet",
						Namespace: "test",
					}, cs); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if cs.Spec.ResourceApplyMode != hivev1.UpsertResourceApplyMode {
					t.Errorf("expected updated, but failed")
				}
			},
		},
		{
			name: "delete syncset with upsert mode",
			startObjs: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&hivev1.SyncSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
					},
					Spec: hivev1.SyncSetSpec{
						SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
							ResourceApplyMode: hivev1.UpsertResourceApplyMode,
						},
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-klusterlet-crds",
					Namespace: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client) {
				cs := &hivev1.SyncSet{}
				if err := client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
					}, cs); !errors.IsNotFound(err) {
					t.Errorf("unexpected error: %v", err)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &ReconcileSyncSet{
				client:   fake.NewFakeClientWithScheme(testscheme, c.startObjs...),
				recorder: eventstesting.NewTestingEventRecorder(t),
			}

			_, err := r.Reconcile(c.request)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c.validateFunc(t, r.client)
		})
	}
}
