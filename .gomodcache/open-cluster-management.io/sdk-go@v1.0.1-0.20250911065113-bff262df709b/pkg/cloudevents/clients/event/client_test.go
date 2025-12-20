package event

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudevents/sdk-go/v2/protocol/gochan"

	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/statushash"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
)

func TestCreate(t *testing.T) {
	cases := []struct {
		name        string
		clusterName string
		event       *eventsv1.Event
	}{
		{
			name:        "create event",
			clusterName: "cluster1",
			event: &eventsv1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ceClient, err := generic.NewCloudEventAgentClient(
				context.Background(),
				fake.NewAgentOptions(gochan.New(), nil, c.clusterName, c.clusterName+"agent"),
				nil,
				statushash.StatusHash,
				NewEventCodec())
			if err != nil {
				t.Error(err)
			}

			evtClient := NewEventClient(ceClient).WithNamespace(c.clusterName)
			if _, err := evtClient.Create(context.Background(), c.event, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
		})
	}

}

func TestPatch(t *testing.T) {
	cases := []struct {
		name        string
		clusterName string
		event       *eventsv1.Event
		patch       []byte
	}{
		{
			name:        "patch event",
			clusterName: "cluster1",
			event: &eventsv1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "cluster1"},
			},
			patch: func() []byte {
				oldData, err := json.Marshal(&eventsv1.Event{ObjectMeta: metav1.ObjectMeta{Name: "test"}})
				if err != nil {
					t.Fatal(err)
				}
				newData, err := json.Marshal(&eventsv1.Event{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Series:     &eventsv1.EventSeries{Count: 2},
				})
				if err != nil {
					t.Fatal(err)
				}
				data, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, eventsv1.Event{})
				if err != nil {
					t.Fatal(err)
				}
				return data
			}(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ceClient, err := generic.NewCloudEventAgentClient(
				context.Background(),
				fake.NewAgentOptions(gochan.New(), nil, c.clusterName, c.clusterName+"agent"),
				nil,
				statushash.StatusHash,
				NewEventCodec())
			if err != nil {
				t.Fatal(err)
			}

			evtClient := NewEventClient(ceClient).WithNamespace(c.clusterName)

			if _, err := evtClient.Patch(context.Background(),
				c.event.Name,
				types.StrategicMergePatchType,
				c.patch,
				metav1.PatchOptions{}); err != nil {
				t.Fatal(err)
			}
		})
	}

}
