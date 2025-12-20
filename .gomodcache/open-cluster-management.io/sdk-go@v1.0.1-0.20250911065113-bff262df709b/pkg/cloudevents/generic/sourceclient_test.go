package generic

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/stretchr/testify/require"
	kubetypes "k8s.io/apimachinery/pkg/types"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/payload"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

const testSourceName = "mock-source"

func TestSourceResync(t *testing.T) {
	cases := []struct {
		name          string
		resources     []*mockResource
		eventType     types.CloudEventsType
		expectedItems int
	}{
		{
			name:          "no cached resources",
			resources:     []*mockResource{},
			eventType:     types.CloudEventsType{SubResource: types.SubResourceStatus},
			expectedItems: 0,
		},
		{
			name: "has cached resources",
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "2", Status: "test1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "3", Status: "test2"},
			},
			eventType:     types.CloudEventsType{SubResource: types.SubResourceStatus},
			expectedItems: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			sourceOptions := fake.NewSourceOptions(gochan.New(), testSourceName)
			lister := newMockResourceLister(c.resources...)
			source, err := NewCloudEventSourceClient[*mockResource](ctx, sourceOptions, lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			eventChan := make(chan receiveEvent)
			stop := make(chan bool)
			go func() {
				err = source.cloudEventsClient.StartReceiver(ctx, func(event cloudevents.Event) {
					eventChan <- receiveEvent{event: event}
				})
				if err != nil {
					eventChan <- receiveEvent{err: err}
				}
				stop <- true
			}()

			err = source.Resync(ctx, "cluster1")
			require.NoError(t, err)

			receivedEvent := <-eventChan
			require.NoError(t, receivedEvent.err)
			require.NotNil(t, receivedEvent.event)

			resourceList, err := payload.DecodeStatusResyncRequest(receivedEvent.event)
			require.NoError(t, err)
			require.Equal(t, c.expectedItems, len(resourceList.Hashes))

			cancel()
			<-stop
		})
	}
}

func TestSourcePublish(t *testing.T) {
	cases := []struct {
		name      string
		resources *mockResource
		eventType types.CloudEventsType
	}{
		{
			name: "publish specs",
			resources: &mockResource{
				UID:             kubetypes.UID("1234"),
				ResourceVersion: "2",
				Spec:            "test-spec",
			},
			eventType: types.CloudEventsType{
				CloudEventsDataType: mockEventDataType,
				SubResource:         types.SubResourceSpec,
				Action:              "test_create_request",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			sourceOptions := fake.NewSourceOptions(gochan.New(), testSourceName)
			lister := newMockResourceLister()
			source, err := NewCloudEventSourceClient[*mockResource](ctx, sourceOptions, lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			eventChan := make(chan receiveEvent)
			stop := make(chan bool)
			go func() {
				err = source.cloudEventsClient.StartReceiver(ctx, func(event cloudevents.Event) {
					eventChan <- receiveEvent{event: event}
				})
				if err != nil {
					eventChan <- receiveEvent{err: err}
				}
				stop <- true
			}()

			err = source.Publish(ctx, c.eventType, c.resources)
			require.NoError(t, err)

			receivedEvent := <-eventChan
			require.NoError(t, receivedEvent.err)
			require.NotNil(t, receivedEvent.event)

			eventOut := receivedEvent.event

			resourceID, err := eventOut.Context.GetExtension("resourceid")
			require.NoError(t, err)
			require.Equal(t, c.resources.UID, kubetypes.UID(fmt.Sprintf("%s", resourceID)))

			resourceVersion, err := eventOut.Context.GetExtension("resourceversion")
			require.NoError(t, err)
			require.Equal(t, c.resources.ResourceVersion, resourceVersion)

			cancel()
			<-stop
		})
	}
}

func TestSpecResyncResponse(t *testing.T) {
	cases := []struct {
		name         string
		requestEvent cloudevents.Event
		resources    []*mockResource
		validate     func([]cloudevents.Event)
	}{
		{
			name: "unsupported event type",
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
			name: "unsupported resync event type",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
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
			name: "resync all specs",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              types.ResyncRequestAction,
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", "cluster1")
				if err := evt.SetData(cloudevents.ApplicationJSON, &payload.ResourceVersionList{}); err != nil {
					t.Fatal(err)
				}
				return evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "2", Spec: "test1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "3", Spec: "test2"},
			},
			validate: func(pubEvents []cloudevents.Event) {
				if len(pubEvents) != 2 {
					t.Errorf("expected all publish events, but got %v", pubEvents)
				}
			},
		},
		{
			name: "resync specs",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              types.ResyncRequestAction,
				}

				versions := &payload.ResourceVersionList{
					Versions: []payload.ResourceVersion{
						{ResourceID: "test1", ResourceVersion: 1},
						{ResourceID: "test2", ResourceVersion: 2},
					},
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", "cluster1")
				if err := evt.SetData(cloudevents.ApplicationJSON, versions); err != nil {
					t.Fatal(err)
				}
				return evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "2", Spec: "test1-updated"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "2", Spec: "test2"},
			},
			validate: func(pubEvents []cloudevents.Event) {
				if len(pubEvents) != 1 {
					t.Errorf("expected one publish events, but got %v", pubEvents)
				}
			},
		},
		{
			name: "resync specs - deletion",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              types.ResyncRequestAction,
				}

				versions := &payload.ResourceVersionList{
					Versions: []payload.ResourceVersion{
						{ResourceID: "test1", ResourceVersion: 1},
						{ResourceID: "test2", ResourceVersion: 2},
					},
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", "cluster1")
				if err := evt.SetData(cloudevents.ApplicationJSON, versions); err != nil {
					t.Fatal(err)
				}
				return evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "1", Spec: "test1"},
			},
			validate: func(pubEvents []cloudevents.Event) {
				if len(pubEvents) != 1 {
					t.Errorf("expected one publish events, but got %v", pubEvents)
				}

				if _, err := pubEvents[0].Context.GetExtension("deletiontimestamp"); err != nil {
					t.Errorf("expected deletion events, but got %v", pubEvents)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			sourceOptions := fake.NewSourceOptions(gochan.New(), testSourceName)
			lister := newMockResourceLister(c.resources...)
			source, err := NewCloudEventSourceClient[*mockResource](ctx, sourceOptions, lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			// start receiver
			receivedEvents := []cloudevents.Event{}
			stop := make(chan bool)
			mutex := &sync.Mutex{}
			go func() {
				_ = source.cloudEventsClient.StartReceiver(ctx, func(event cloudevents.Event) {
					mutex.Lock()
					defer mutex.Unlock()
					receivedEvents = append(receivedEvents, event)
				})
				stop <- true
			}()

			// receive resync request and publish associated resources
			source.receive(ctx, c.requestEvent)
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

func TestReceiveResourceStatus(t *testing.T) {
	cases := []struct {
		name         string
		requestEvent cloudevents.Event
		resources    []*mockResource
		validate     func(event types.ResourceAction, resource *mockResource)
	}{
		{
			name: "unsupported sub resource",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
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
			name: "no registered codec for the resource",
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
			name: "update status",
			requestEvent: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              "test_update_request",
				}

				evt, _ := newMockResourceCodec().Encode(testAgentName, eventType, &mockResource{UID: kubetypes.UID("test1"), ResourceVersion: "1", Status: "update-test1"})
				evt.SetExtension("clustername", "cluster1")
				return *evt
			}(),
			resources: []*mockResource{
				{UID: kubetypes.UID("test1"), ResourceVersion: "1", Status: "test1"},
				{UID: kubetypes.UID("test2"), ResourceVersion: "1", Status: "test2"},
			},
			validate: func(event types.ResourceAction, resource *mockResource) {
				if event != types.StatusModified {
					t.Errorf("expected modified, but get %s", event)
				}
				if resource.UID != "test1" {
					t.Errorf("unexpected resource %v", resource)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sourceOptions := fake.NewSourceOptions(gochan.New(), testSourceName)
			lister := newMockResourceLister(c.resources...)
			source, err := NewCloudEventSourceClient[*mockResource](context.TODO(), sourceOptions, lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			var actualEvent types.ResourceAction
			var actualRes *mockResource
			source.receive(context.TODO(), c.requestEvent, func(event types.ResourceAction, resource *mockResource) error {
				actualEvent = event
				actualRes = resource
				return nil
			})

			c.validate(actualEvent, actualRes)
		})
	}
}
