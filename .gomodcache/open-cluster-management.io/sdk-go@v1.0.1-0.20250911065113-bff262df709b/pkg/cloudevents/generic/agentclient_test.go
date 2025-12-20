package generic

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubetypes "k8s.io/apimachinery/pkg/types"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/payload"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

const testAgentName = "mock-agent"

var mockEventDataType = types.CloudEventsDataType{
	Group:    "resources.test",
	Version:  "v1",
	Resource: "mockresources",
}

func TestAgentResync(t *testing.T) {
	cases := []struct {
		name          string
		clusterName   string
		resources     []*mockResource
		eventType     types.CloudEventsType
		expectedItems int
	}{
		{
			name:          "no cached resources",
			clusterName:   "cluster1",
			resources:     []*mockResource{},
			eventType:     types.CloudEventsType{SubResource: types.SubResourceSpec},
			expectedItems: 0,
		},
		{
			name:        "has cached resources",
			clusterName: "cluster2",
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "2"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "3"},
			},
			eventType:     types.CloudEventsType{SubResource: types.SubResourceSpec},
			expectedItems: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			lister := newMockResourceLister(c.resources...)
			agent, err := NewCloudEventAgentClient[*mockResource](ctx, fake.NewAgentOptions(gochan.New(), nil, c.clusterName, testAgentName), lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			// start a cloudevents receiver client go to receive the event
			eventChan := make(chan receiveEvent)
			stop := make(chan bool)

			go func() {
				err = agent.cloudEventsClient.StartReceiver(ctx, func(event cloudevents.Event) {
					eventChan <- receiveEvent{event: event}
				})
				if err != nil {
					eventChan <- receiveEvent{err: err}
				}
				stop <- true
			}()

			err = agent.Resync(ctx, types.SourceAll)
			require.NoError(t, err)

			receivedEvent := <-eventChan
			require.NoError(t, receivedEvent.err)
			require.NotNil(t, receivedEvent.event)

			eventOut := receivedEvent.event
			clusterName, err := eventOut.Context.GetExtension("clustername")
			require.NoError(t, err)
			require.Equal(t, c.clusterName, clusterName)

			resourceList, err := payload.DecodeSpecResyncRequest(eventOut)
			require.NoError(t, err)
			require.Equal(t, c.expectedItems, len(resourceList.Versions))

			cancel()
			<-stop
		})
	}
}

func TestAgentPublish(t *testing.T) {
	cases := []struct {
		name        string
		clusterName string
		resources   *mockResource
		eventType   types.CloudEventsType
	}{
		{
			name:        "publish status",
			clusterName: "cluster1",
			resources: &mockResource{
				UID:             kubetypes.UID("1234"),
				ResourceVersion: "2",
				Status:          "test-status",
				Namespace:       "cluster1",
			},
			eventType: types.CloudEventsType{
				CloudEventsDataType: mockEventDataType,
				SubResource:         types.SubResourceStatus,
				Action:              "test_update_request",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			agentOptions := fake.NewAgentOptions(gochan.New(), nil, c.clusterName, testAgentName)
			lister := newMockResourceLister()
			agent, err := NewCloudEventAgentClient[*mockResource](context.TODO(), agentOptions, lister, statusHash, newMockResourceCodec())
			require.Nil(t, err)

			// start a cloudevents receiver client go to receive the event
			eventChan := make(chan receiveEvent)
			stop := make(chan bool)
			go func() {
				err = agent.cloudEventsClient.StartReceiver(ctx, func(event cloudevents.Event) {
					eventChan <- receiveEvent{event: event}
				})
				if err != nil {
					eventChan <- receiveEvent{err: err}
				}
				stop <- true
			}()

			err = agent.Publish(ctx, c.eventType, c.resources)
			require.Nil(t, err)

			receivedEvent := <-eventChan
			require.NoError(t, receivedEvent.err)
			require.NotNil(t, receivedEvent.event)

			eventOut := receivedEvent.event
			resourceID, err := eventOut.Context.GetExtension("resourceid")
			require.Equal(t, c.resources.UID, kubetypes.UID(fmt.Sprintf("%s", resourceID)))

			resourceVersion, err := eventOut.Context.GetExtension("resourceversion")
			require.NoError(t, err)
			require.Equal(t, c.resources.ResourceVersion, resourceVersion)

			clusterName, err := eventOut.Context.GetExtension("clustername")
			require.NoError(t, err)
			require.Equal(t, c.clusterName, clusterName)

			cancel()
			<-stop
		})
	}
}

func TestStatusResyncResponse(t *testing.T) {
	cases := []struct {
		name         string
		clusterName  string
		requestEvent cloudevents.Event
		resources    []*mockResource
		validate     func([]cloudevents.Event)
	}{
		{
			name:        "unsupported event type",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				evt := cloudevents.NewEvent()
				evt.SetType("unsupported")
				return evt
			}(),
			validate: func(pubEvents []cloudevents.Event) {
				if len(pubEvents) != 0 {
					t.Errorf("unexpected publish events %v", pubEvents)
				}
			},
		},
		{
			name:        "unsupported resync event type",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              types.ResyncRequestAction,
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				return evt
			}(),
			validate: func(pubEvents []cloudevents.Event) {
				if len(pubEvents) != 0 {
					t.Errorf("unexpected publish events %v", pubEvents)
				}
			},
		},
		{
			name:        "resync all status",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              types.ResyncRequestAction,
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				if err := evt.SetData(cloudevents.ApplicationJSON, &payload.ResourceStatusHashList{}); err != nil {
					t.Fatal(err)
				}
				return evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "2", Status: "test1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "3", Status: "test2"},
			},
			validate: func(pubEvents []cloudevents.Event) {
				if len(pubEvents) != 2 {
					t.Errorf("expected all publish events, but got %v", pubEvents)
				}
			},
		},
		{
			name:        "resync status",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              types.ResyncRequestAction,
				}

				statusHashes := &payload.ResourceStatusHashList{
					Hashes: []payload.ResourceStatusHash{
						{ResourceID: "test1", StatusHash: "test1"},
						{ResourceID: "test2", StatusHash: "test2"},
					},
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				if err := evt.SetData(cloudevents.ApplicationJSON, statusHashes); err != nil {
					t.Fatal(err)
				}
				return evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test0"), ResourceVersion: "2", Status: "test0"},
				{UID: kubetypes.UID("test1"), ResourceVersion: "2", Status: "test1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "3", Status: "test2-updated"},
			},
			validate: func(pubEvents []cloudevents.Event) {
				if len(pubEvents) != 1 {
					t.Errorf("expected one publish events, but got %v", pubEvents)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			agentOptions := fake.NewAgentOptions(gochan.New(), nil, c.clusterName, testAgentName)
			lister := newMockResourceLister(c.resources...)
			agent, err := NewCloudEventAgentClient[*mockResource](ctx, agentOptions, lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			// start receiver
			receivedEvents := []cloudevents.Event{}
			stop := make(chan bool)
			mutex := &sync.Mutex{}

			go func() {
				_ = agent.cloudEventsClient.StartReceiver(ctx, func(event cloudevents.Event) {
					mutex.Lock()
					defer mutex.Unlock()
					receivedEvents = append(receivedEvents, event)
				})
				stop <- true
			}()

			// receive resync request and publish associated resources
			agent.receive(ctx, c.requestEvent)
			// wait 1 seconds to receive the response resources
			time.Sleep(1 * time.Second)

			mutex.Lock()
			c.validate(receivedEvents)
			mutex.Unlock()

			cancel()
			<-stop
		})
	}
}

func TestReceiveResourceSpec(t *testing.T) {
	cases := []struct {
		name         string
		clusterName  string
		requestEvent cloudevents.Event
		resources    []*mockResource
		validate     func(event types.ResourceAction, resource *mockResource)
	}{
		{
			name:        "unsupported sub resource",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              "test_create_request",
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				return evt
			}(),
			validate: func(event types.ResourceAction, resource *mockResource) {
				if len(event) != 0 {
					t.Errorf("should not be invoked")
				}
			},
		},
		{
			name:        "no registered codec for the resource",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					SubResource: types.SubResourceSpec,
					Action:      "test_create_request",
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				return evt
			}(),
			validate: func(event types.ResourceAction, resource *mockResource) {
				if len(event) != 0 {
					t.Errorf("should not be invoked")
				}
			},
		},
		{
			name:        "create a resource",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test_create_request",
				}

				evt, _ := newMockResourceCodec().Encode(testAgentName, eventType, &mockResource{UID: kubetypes.UID("test1"), ResourceVersion: "1", Namespace: "cluster1"})
				return *evt
			}(),
			validate: func(event types.ResourceAction, resource *mockResource) {
				if event != types.Added {
					t.Errorf("expected added, but get %s", event)
				}
			},
		},
		{
			name:        "update a resource",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test_update_request",
				}

				evt, _ := newMockResourceCodec().Encode(testAgentName, eventType, &mockResource{UID: kubetypes.UID("test1"), ResourceVersion: "2", Namespace: "cluster1"})
				return *evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "1", Namespace: "cluster1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "1", Namespace: "cluster1"},
			},
			validate: func(event types.ResourceAction, resource *mockResource) {
				if event != types.Modified {
					t.Errorf("expected modified, but get %s", event)
				}
				if resource.UID != "test1" {
					t.Errorf("unexpected resource %v", resource)
				}
				if resource.ResourceVersion != "2" {
					t.Errorf("unexpected resource %v", resource)
				}
			},
		},
		{
			name:        "delete a resource",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test_delete_request",
				}
				now := metav1.Now()
				evt, _ := newMockResourceCodec().Encode(testAgentName, eventType, &mockResource{UID: kubetypes.UID("test2"), ResourceVersion: "2", DeletionTimestamp: &now, Namespace: "cluster1"})
				return *evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "1", Namespace: "cluster1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "1", Namespace: "cluster1"},
			},
			validate: func(event types.ResourceAction, resource *mockResource) {
				if event != types.Deleted {
					t.Errorf("expected deleted, but get %s", event)
				}
				if resource.UID != "test2" {
					t.Errorf("unexpected resource %v", resource)
				}
			},
		},
		{
			name:        "no change resource",
			clusterName: "cluster1",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test_create_request",
				}

				evt, _ := newMockResourceCodec().Encode(testAgentName, eventType, &mockResource{UID: kubetypes.UID("test1"), ResourceVersion: "2", Namespace: "cluster1"})
				return *evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "2", Namespace: "cluster1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "1", Namespace: "cluster1"},
			},
			validate: func(event types.ResourceAction, resource *mockResource) {
				if len(event) != 0 {
					t.Errorf("expected no change, but get %s", event)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			agentOptions := fake.NewAgentOptions(gochan.New(), nil, c.clusterName, testAgentName)
			lister := newMockResourceLister(c.resources...)
			agent, err := NewCloudEventAgentClient[*mockResource](context.TODO(), agentOptions, lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			var actualEvent types.ResourceAction
			var actualRes *mockResource
			agent.receive(context.TODO(), c.requestEvent, func(event types.ResourceAction, resource *mockResource) error {
				actualEvent = event
				actualRes = resource
				return nil
			})

			c.validate(actualEvent, actualRes)
		})
	}
}

type receiveEvent struct {
	event cloudevents.Event
	err   error
}

type mockResource struct {
	UID               kubetypes.UID `json:"uid"`
	ResourceVersion   string        `json:"resourceVersion"`
	DeletionTimestamp *metav1.Time  `json:"deletionTimestamp,omitempty"`
	Namespace         string
	Spec              string `json:"spec"`
	Status            string `json:"status"`
}

func (r *mockResource) GetUID() kubetypes.UID {
	return r.UID
}

func (r *mockResource) GetResourceVersion() string {
	return r.ResourceVersion
}

func (r *mockResource) GetDeletionTimestamp() *metav1.Time {
	return r.DeletionTimestamp
}

type mockResourceLister struct {
	resources []*mockResource
}

func newMockResourceLister(resources ...*mockResource) *mockResourceLister {
	return &mockResourceLister{
		resources: resources,
	}
}

func (l *mockResourceLister) List(opt types.ListOptions) ([]*mockResource, error) {
	return l.resources, nil
}

func statusHash(r *mockResource) (string, error) {
	return r.Status, nil
}

type mockResourceCodec struct{}

func newMockResourceCodec() *mockResourceCodec {
	return &mockResourceCodec{}
}

func (c *mockResourceCodec) EventDataType() types.CloudEventsDataType {
	return mockEventDataType
}

func (c *mockResourceCodec) Encode(source string, eventType types.CloudEventsType, obj *mockResource) (*cloudevents.Event, error) {
	evt := cloudevents.NewEvent()
	evt.SetID(uuid.New().String())
	evt.SetSource(source)
	evt.SetType(eventType.String())
	evt.SetTime(time.Now())
	evt.SetExtension("resourceid", string(obj.UID))
	evt.SetExtension("resourceversion", obj.ResourceVersion)
	evt.SetExtension("clustername", obj.Namespace)
	if obj.GetDeletionTimestamp() != nil {
		evt.SetExtension("deletiontimestamp", obj.DeletionTimestamp.Time)
	}
	if err := evt.SetData(cloudevents.TextPlain, obj.Status); err != nil {
		return nil, err
	}
	return &evt, nil
}

func (c *mockResourceCodec) Decode(evt *cloudevents.Event) (*mockResource, error) {
	resourceID, err := evt.Context.GetExtension("resourceid")
	if err != nil {
		return nil, fmt.Errorf("failed to get resource ID: %v", err)
	}

	resourceVersion, err := evt.Context.GetExtension("resourceversion")
	if err != nil {
		return nil, fmt.Errorf("failed to get resource version: %v", err)
	}

	res := &mockResource{
		UID:             kubetypes.UID(fmt.Sprintf("%s", resourceID)),
		ResourceVersion: fmt.Sprintf("%s", resourceVersion),
		Status:          string(evt.Data()),
	}

	deletionTimestamp, err := evt.Context.GetExtension("deletiontimestamp")
	if err == nil {
		timestamp, err := time.Parse(time.RFC3339, fmt.Sprintf("%s", deletionTimestamp))
		if err != nil {
			return nil, fmt.Errorf("failed to parse deletiontimestamp - %v to time.Time", deletionTimestamp)
		}
		res.DeletionTimestamp = &metav1.Time{Time: timestamp}
	}

	return res, nil
}
