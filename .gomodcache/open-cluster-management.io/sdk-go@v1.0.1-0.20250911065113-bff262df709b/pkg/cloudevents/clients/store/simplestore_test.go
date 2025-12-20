package store

import (
	"testing"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

func TestHandleReceivedResource(t *testing.T) {
	// identity := "test"
	store := NewSimpleStore[*coordv1.Lease]()
	if err := store.Add(&coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "update",
			Namespace: "test",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(&coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deletion",
			Namespace: "test",
		},
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		action   types.ResourceAction
		received *coordv1.Lease
		validate func(t *testing.T, namespace, name string)
	}{
		{
			name:   "add resource",
			action: types.Added,
			received: &coordv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new",
					Namespace: "test",
				},
			},
			validate: func(t *testing.T, namespace, name string) {
				_, exists, err := store.Get(namespace, name)
				if err != nil {
					t.Error(err)
				}

				if !exists {
					t.Errorf("expected exits, but failed")
				}
			},
		},
		{
			name:   "update resource",
			action: types.Modified,
			received: &coordv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update",
					Namespace: "test",
				},
				Spec: coordv1.LeaseSpec{
					RenewTime: &metav1.MicroTime{},
				},
			},
			validate: func(t *testing.T, namespace, name string) {
				updated, exists, err := store.Get(namespace, name)
				if err != nil {
					t.Error(err)
				}

				if !exists {
					t.Errorf("expected delete, but failed")
				}

				if updated.Spec.RenewTime == nil {
					t.Errorf("unexpected renew time")
				}
			},
		},
		{
			name:   "delete resource",
			action: types.Deleted,
			received: &coordv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "deletion",
					Namespace:         "test",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			},
			validate: func(t *testing.T, namespace, name string) {
				_, exists, err := store.Get(namespace, name)
				if err != nil {
					t.Error(err)
				}

				if exists {
					t.Errorf("expected delete, but failed")
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := store.HandleReceivedResource(c.action, c.received)
			if err != nil {
				t.Error(err)
			}

			c.validate(t, c.received.Namespace, c.received.Name)
		})
	}
}
