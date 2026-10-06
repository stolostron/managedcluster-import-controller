// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workv1 "open-cluster-management.io/api/work/v1"
)

func TestForceDeleteManifestWork(t *testing.T) {
	recent := metav1.Now()
	expired := metav1.NewTime(time.Now().Add(-ManifestWorkForceDeleteGracePeriod - time.Second))

	cases := []struct {
		name             string
		work             *workv1.ManifestWork
		expectDeleted    bool
		expectFinalizers []string
	}{
		{
			name: "no finalizers",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{Name: "work", Namespace: "test"},
			},
			expectDeleted: true,
		},
		{
			name: "other finalizers are removed immediately",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "work", Namespace: "test", Finalizers: []string{"test"},
				},
			},
			expectDeleted: true,
		},
		{
			name: "agent finalizer is kept when deletion just started",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "work", Namespace: "test",
					Finalizers: []string{workv1.ManifestWorkFinalizer, "test"},
				},
			},
			expectFinalizers: []string{workv1.ManifestWorkFinalizer},
		},
		{
			name: "agent finalizer is kept within the grace period",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "work", Namespace: "test",
					DeletionTimestamp: &recent,
					Finalizers:        []string{workv1.ManifestWorkFinalizer},
				},
			},
			expectFinalizers: []string{workv1.ManifestWorkFinalizer},
		},
		{
			name: "agent finalizer is removed after the grace period",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "work", Namespace: "test",
					DeletionTimestamp: &expired,
					Finalizers:        []string{workv1.ManifestWorkFinalizer, "test"},
				},
			},
			expectDeleted: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.TODO()
			workClient := workfake.NewSimpleClientset(c.work)
			withGracefulManifestWorkDeletion(workClient)

			err := ForceDeleteManifestWork(ctx, workClient, eventstesting.NewTestingEventRecorder(t),
				c.work.Namespace, c.work.Name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			work, err := workClient.WorkV1().ManifestWorks(c.work.Namespace).Get(ctx, c.work.Name, metav1.GetOptions{})
			if c.expectDeleted {
				if !errors.IsNotFound(err) {
					t.Fatalf("expected work to be deleted, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected work to remain, got %v", err)
			}
			if !slices.Equal(work.Finalizers, c.expectFinalizers) {
				t.Fatalf("expected finalizers %v, got %v", c.expectFinalizers, work.Finalizers)
			}
			if work.DeletionTimestamp.IsZero() {
				t.Fatal("expected deletionTimestamp to be set")
			}
		})
	}
}

// withGracefulManifestWorkDeletion makes the fake work client keep objects that
// still have finalizers, matching apiserver delete behavior.
func withGracefulManifestWorkDeletion(clientset *workfake.Clientset) {
	clientset.PrependReactor("delete", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		del := action.(clienttesting.DeleteAction)
		obj, err := clientset.Tracker().Get(del.GetResource(), del.GetNamespace(), del.GetName())
		if err != nil {
			return true, nil, err
		}
		work, ok := obj.(*workv1.ManifestWork)
		if !ok {
			return false, nil, nil
		}
		if len(work.Finalizers) == 0 {
			err = clientset.Tracker().Delete(del.GetResource(), del.GetNamespace(), del.GetName())
			return true, nil, err
		}
		if work.DeletionTimestamp.IsZero() {
			now := metav1.Now()
			work.DeletionTimestamp = &now
			if err = clientset.Tracker().Update(del.GetResource(), work, del.GetNamespace()); err != nil {
				return true, nil, err
			}
		}
		return true, work, nil
	})
	clientset.PrependReactor("patch", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		patch := action.(clienttesting.PatchAction)
		var body struct {
			Metadata struct {
				Finalizers []string `json:"finalizers"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(patch.GetPatch(), &body); err != nil {
			return true, nil, err
		}
		obj, err := clientset.Tracker().Get(patch.GetResource(), patch.GetNamespace(), patch.GetName())
		if err != nil {
			return true, nil, err
		}
		work, ok := obj.(*workv1.ManifestWork)
		if !ok {
			return false, nil, nil
		}
		work.Finalizers = body.Metadata.Finalizers
		if len(work.Finalizers) == 0 {
			err = clientset.Tracker().Delete(patch.GetResource(), patch.GetNamespace(), patch.GetName())
			return true, nil, err
		}
		if err = clientset.Tracker().Update(patch.GetResource(), work, patch.GetNamespace()); err != nil {
			return true, nil, err
		}
		return true, work, nil
	})
}
