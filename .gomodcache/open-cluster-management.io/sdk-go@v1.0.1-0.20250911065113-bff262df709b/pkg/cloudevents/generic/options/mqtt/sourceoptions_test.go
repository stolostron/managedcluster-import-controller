package mqtt

import (
	"context"
	"os"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cloudeventscontext "github.com/cloudevents/sdk-go/v2/context"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	clienttesting "open-cluster-management.io/sdk-go/pkg/testing"
)

const testSourceConfig = `
brokerHost: test
topics:
  sourceEvents: sources/hub1/clusters/+/sourceevents
  agentEvents: sources/hub1/clusters/+/agentevents
`

func TestSourceContext(t *testing.T) {
	file, err := clienttesting.WriteToTempFile("mqtt-config-test-", []byte(testSourceConfig))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	options, err := BuildMQTTOptionsFromFlags(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name          string
		ctx           context.Context
		event         cloudevents.Event
		expectedTopic string
		assertError   func(error)
	}{
		{
			name: "unsupported event",
			ctx:  context.TODO(),
			event: func() cloudevents.Event {
				evt := cloudevents.NewEvent()
				evt.SetType("unsupported")
				return evt
			}(),
			assertError: func(err error) {
				if err == nil {
					t.Errorf("expected error, but failed")
				}
			},
		},
		{
			name:          "get topic from context",
			ctx:           context.WithValue(context.TODO(), MQTT_SOURCE_PUB_TOPIC_KEY, PubTopic("sources/hub1/clusters/cluster1/sourceevents")),
			event:         cloudevents.NewEvent(),
			expectedTopic: "sources/hub1/clusters/cluster1/sourceevents",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
		{
			name:          "get resync topic from context",
			ctx:           context.WithValue(context.TODO(), MQTT_SOURCE_PUB_TOPIC_KEY, PubTopic("sources/source1/sourcebroadcast")),
			event:         cloudevents.NewEvent(),
			expectedTopic: "sources/source1/sourcebroadcast",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
		{
			name: "resync status",
			ctx:  context.TODO(),
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              types.ResyncRequestAction,
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", "cluster1")
				return evt
			}(),
			expectedTopic: "sources/hub1/clusters/cluster1/sourceevents",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
		{
			name: "unsupported send resource no cluster name",
			ctx:  context.TODO(),
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test",
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				return evt
			}(),
			assertError: func(err error) {
				if err == nil {
					t.Errorf("expected error, but failed")
				}
			},
		},
		{
			name: "send spec",
			ctx:  context.TODO(),
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test",
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", "cluster1")
				return evt
			}(),
			expectedTopic: "sources/hub1/clusters/cluster1/sourceevents",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sourceOptions := &mqttSourceOptions{
				MQTTOptions: *options,
				sourceID:    "hub1",
			}
			ctx, err := sourceOptions.WithContext(c.ctx, c.event.Context)
			c.assertError(err)

			topic := func(ctx context.Context) string {
				if ctx == nil {
					return ""
				}

				return cloudeventscontext.TopicFrom(ctx)
			}(ctx)

			if topic != c.expectedTopic {
				t.Errorf("expected %s, but got %s", c.expectedTopic, topic)
			}
		})
	}
}
