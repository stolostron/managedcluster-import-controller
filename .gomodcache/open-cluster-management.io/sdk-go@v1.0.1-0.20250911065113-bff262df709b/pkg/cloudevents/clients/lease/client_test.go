package lease

import (
	"context"
	"testing"

	"github.com/cloudevents/sdk-go/v2/protocol/gochan"

	coordv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/statushash"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
)

func TestUpdate(t *testing.T) {
	cases := []struct {
		name        string
		clusterName string
		lease       *coordv1.Lease
	}{
		{
			name:        "update lease",
			clusterName: "cluster1",
			lease: &coordv1.Lease{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "cluster1"},
				Spec: coordv1.LeaseSpec{
					RenewTime: &metav1.MicroTime{Time: metav1.Now().Time},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			leaseWatchStore := store.NewSimpleStore[*coordv1.Lease]()
			ceClient, err := generic.NewCloudEventAgentClient(
				context.Background(),
				fake.NewAgentOptions(gochan.New(), nil, c.clusterName, c.clusterName+"agent"),
				store.NewAgentWatcherStoreLister(leaseWatchStore),
				statushash.StatusHash,
				NewLeaseCodec())
			if err != nil {
				t.Error(err)
			}

			leaseClient := &LeaseClient{
				cloudEventsClient: ceClient,
				watcherStore:      leaseWatchStore,
				namespace:         c.clusterName,
			}

			if _, err := leaseClient.Update(context.Background(), c.lease, metav1.UpdateOptions{}); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestGet(t *testing.T) {
	cases := []struct {
		name        string
		clusterName string
		leases      []*coordv1.Lease
	}{
		{
			name:        "get lease",
			clusterName: "cluster1",
			leases: []*coordv1.Lease{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "cluster1"},
					Spec: coordv1.LeaseSpec{
						RenewTime: &metav1.MicroTime{Time: metav1.Now().Time},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			leaseWatchStore := store.NewSimpleStore[*coordv1.Lease]()
			for _, lease := range c.leases {
				if err := leaseWatchStore.Add(lease); err != nil {
					t.Fatal(err)
				}
			}

			ceClient, err := generic.NewCloudEventAgentClient(
				context.Background(),
				fake.NewAgentOptions(gochan.New(), nil, "cluster1", "cluster1-agent"),
				store.NewAgentWatcherStoreLister(leaseWatchStore),
				statushash.StatusHash,
				NewLeaseCodec())
			if err != nil {
				t.Error(err)
			}

			leaseClient := &LeaseClient{
				cloudEventsClient: ceClient,
				watcherStore:      leaseWatchStore,
				namespace:         "cluster1",
			}

			if _, err := leaseClient.Get(context.Background(), "test", metav1.GetOptions{}); err != nil {
				t.Error(err)
			}
		})
	}
}
