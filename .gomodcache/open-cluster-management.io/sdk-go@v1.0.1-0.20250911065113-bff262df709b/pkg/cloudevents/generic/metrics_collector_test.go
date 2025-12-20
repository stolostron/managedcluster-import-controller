package generic

import (
	"context"
	"fmt"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	kubetypes "k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/payload"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

type testResyncType string

const (
	testSpecResync   testResyncType = "spec"
	testStatusResync testResyncType = "status"
)

func TestCloudEventsMetrics(t *testing.T) {
	cases := []struct {
		name        string
		clusterName string
		sourceID    string
		resources   []*mockResource
		dataType    types.CloudEventsDataType
		subresource types.EventSubResource
		action      types.EventAction
	}{
		{
			name:        "receive single resource",
			clusterName: "cluster1",
			sourceID:    "source1",
			resources: []*mockResource{
				{Namespace: "cluster1", UID: kubetypes.UID("test1"), ResourceVersion: "2", Status: "test1"},
			},
			dataType:    mockEventDataType,
			subresource: types.SubResourceSpec,
			action:      types.EventAction("test_create_request"),
		},
		{
			name:        "receive multiple resources",
			clusterName: "cluster1",
			sourceID:    "source1",
			resources: []*mockResource{
				{Namespace: "cluster1", UID: kubetypes.UID("test1"), ResourceVersion: "2", Status: "test1"},
				{Namespace: "cluster1", UID: kubetypes.UID("test2"), ResourceVersion: "3", Status: "test2"},
			},
			dataType:    mockEventDataType,
			subresource: types.SubResourceSpec,
			action:      types.EventAction("test_create_request"),
		},
	}
	for _, c := range cases {
		// reset metrics
		ResetCloudEventsMetrics()
		// run test
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			sendReceiver := gochan.New()

			// initialize source client
			sourceOptions := fake.NewSourceOptions(sendReceiver, c.sourceID)
			lister := newMockResourceLister([]*mockResource{}...)
			source, err := NewCloudEventSourceClient[*mockResource](ctx, sourceOptions, lister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			// initialize agent client
			agentOptions := fake.NewAgentOptions(sendReceiver, nil, c.clusterName, testAgentName)
			agentLister := newMockResourceLister([]*mockResource{}...)
			agent, err := NewCloudEventAgentClient[*mockResource](ctx, agentOptions, agentLister, statusHash, newMockResourceCodec())
			require.NoError(t, err)

			// start agent subscription
			agent.subscribe(ctx, func(ctx context.Context, evt cloudevents.Event) {
				agent.receive(ctx, evt)
			})

			eventType := types.CloudEventsType{
				CloudEventsDataType: c.dataType,
				SubResource:         c.subresource,
				Action:              c.action,
			}

			// publish resources to agent
			for _, resource := range c.resources {
				err = source.Publish(ctx, eventType, resource)
				require.NoError(t, err)
			}

			// wait 1 second for agent receive the resources
			time.Sleep(time.Second)

			// ensure metrics are updated
			sentTotal := cloudeventsSentCounterMetric.WithLabelValues(c.sourceID, noneOriginalSource, c.clusterName, c.dataType.String(), string(c.subresource), string(c.action))
			require.Equal(t, len(c.resources), int(toFloat64Counter(sentTotal)))
			receivedTotal := cloudeventsReceivedCounterMetric.WithLabelValues(c.sourceID, c.clusterName, c.dataType.String(), string(c.subresource), string(c.action))
			require.Equal(t, len(c.resources), int(toFloat64Counter(receivedTotal)))

			cancel()
		})
	}
}

func TestReconnectMetrics(t *testing.T) {
	// reset metrics
	ResetCloudEventsMetrics()
	ctx, cancel := context.WithCancel(context.Background())

	originalDelayFn := DelayFn
	// override DelayFn to avoid waiting for backoff
	DelayFn = func() time.Duration { return 0 }
	defer func() {
		// reset DelayFn
		DelayFn = originalDelayFn
	}()

	errChan := make(chan error)
	agentOptions := fake.NewAgentOptions(gochan.New(), errChan, "cluster1", testAgentName)
	agentLister := newMockResourceLister([]*mockResource{}...)
	_, err := NewCloudEventAgentClient[*mockResource](ctx, agentOptions, agentLister, statusHash, newMockResourceCodec())
	require.NoError(t, err)

	// mimic agent disconnection by sending an error
	errChan <- fmt.Errorf("test error")
	// sleep second to wait for the agent to reconnect
	time.Sleep(time.Second)

	reconnectTotal := clientReconnectedCounterMetric.WithLabelValues(testAgentName)
	require.Equal(t, 1.0, toFloat64Counter(reconnectTotal))

	cancel()
}

// toFloat64Counter returns the count of a counter metric
func toFloat64Counter(c prometheus.Counter) float64 {
	var (
		m      prometheus.Metric
		mCount int
		mChan  = make(chan prometheus.Metric)
		done   = make(chan struct{})
	)

	go func() {
		for m = range mChan {
			mCount++
		}
		close(done)
	}()

	c.Collect(mChan)
	close(mChan)
	<-done

	if mCount != 1 {
		panic(fmt.Errorf("collected %d metrics instead of exactly 1", mCount))
	}

	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		panic(fmt.Errorf("metric write failed, err=%v", err))
	}

	if pb.Counter != nil {
		return pb.Counter.GetValue()
	}
	panic(fmt.Errorf("collected a non-counter metric: %s", pb))
}

func TestResyncMetrics(t *testing.T) {
	cases := []struct {
		name        string
		resyncType  testResyncType
		clusterName string
		sourceID    string
		resources   []*mockResource
		dataType    types.CloudEventsDataType
	}{
		{
			name:        "resync spec",
			resyncType:  testSpecResync,
			clusterName: "cluster1",
			sourceID:    "source1",
			resources: []*mockResource{
				{Namespace: "cluster1", UID: kubetypes.UID("test1"), ResourceVersion: "2", Status: "test1"},
				{Namespace: "cluster1", UID: kubetypes.UID("test2"), ResourceVersion: "3", Status: "test2"},
			},
			dataType: mockEventDataType,
		},
		{
			name:        "resync status",
			resyncType:  testStatusResync,
			clusterName: "cluster1",
			sourceID:    "source1",
			resources: []*mockResource{
				{Namespace: "cluster1", UID: kubetypes.UID("test1"), ResourceVersion: "2", Status: "test1"},
			},
			dataType: mockEventDataType,
		},
	}

	for _, c := range cases {
		// reset metrics
		ResetCloudEventsMetrics()
		// run test
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			if c.resyncType == testSpecResync {
				sourceOptions := fake.NewSourceOptions(gochan.New(), c.sourceID)
				lister := newMockResourceLister(c.resources...)
				source, err := NewCloudEventSourceClient[*mockResource](ctx, sourceOptions, lister, statusHash, newMockResourceCodec())
				require.NoError(t, err)

				eventType := types.CloudEventsType{
					CloudEventsDataType: c.dataType,
					SubResource:         types.SubResourceSpec,
					Action:              types.ResyncRequestAction,
				}
				evt := cloudevents.NewEvent()
				evt.SetSource(c.clusterName)
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", c.clusterName)
				if err := evt.SetData(cloudevents.ApplicationJSON, &payload.ResourceVersionList{}); err != nil {
					t.Errorf("failed to set data for event: %v", err)
				}

				// receive resync request and publish associated resources
				source.receive(ctx, evt)

				receivedTotal := cloudeventsReceivedCounterMetric.WithLabelValues(c.clusterName, c.clusterName, c.dataType.String(), string(types.SubResourceSpec), string(types.ResyncRequestAction))
				require.Equal(t, 1, int(toFloat64Counter(receivedTotal)))

				// wait 1 seconds to respond to the spec resync request
				time.Sleep(1 * time.Second)

				// check spec resync duration metric as a histogram
				h := resourceSpecResyncDurationMetric.WithLabelValues(c.sourceID, c.clusterName, c.dataType.String())
				count, sum := toFloat64HistCountAndSum(h)
				require.Equal(t, uint64(1), count)
				require.Greater(t, sum, 0.0)
				require.Less(t, sum, 1.0)

				sentTotal := cloudeventsSentCounterMetric.WithLabelValues(c.sourceID, noneOriginalSource, c.clusterName, c.dataType.String(), string(types.SubResourceSpec), string(types.ResyncResponseAction))
				require.Equal(t, len(c.resources), int(toFloat64Counter(sentTotal)))
			}

			if c.resyncType == testStatusResync {
				agentOptions := fake.NewAgentOptions(gochan.New(), nil, c.clusterName, testAgentName)
				lister := newMockResourceLister(c.resources...)
				agent, err := NewCloudEventAgentClient[*mockResource](ctx, agentOptions, lister, statusHash, newMockResourceCodec())
				require.NoError(t, err)

				eventType := types.CloudEventsType{
					CloudEventsDataType: c.dataType,
					SubResource:         types.SubResourceStatus,
					Action:              types.ResyncRequestAction,
				}
				evt := cloudevents.NewEvent()
				evt.SetSource(c.sourceID)
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", c.clusterName)
				if err := evt.SetData(cloudevents.ApplicationJSON, &payload.ResourceStatusHashList{}); err != nil {
					t.Errorf("failed to set data for event: %v", err)
				}

				// receive resync request and publish associated resources
				agent.receive(ctx, evt)

				receivedTotal := cloudeventsReceivedCounterMetric.WithLabelValues(c.sourceID, c.clusterName, c.dataType.String(), string(types.SubResourceStatus), string(types.ResyncRequestAction))
				require.Equal(t, 1, int(toFloat64Counter(receivedTotal)))

				// wait 1 seconds to respond to the resync request
				time.Sleep(1 * time.Second)

				// check status resync duration metric as a histogram
				h := resourceStatusResyncDurationMetric.WithLabelValues(c.sourceID, c.clusterName, c.dataType.String())
				count, sum := toFloat64HistCountAndSum(h)
				require.Equal(t, uint64(1), count)
				require.Greater(t, sum, 0.0)
				require.Less(t, sum, 1.0)

				sentTotal := cloudeventsSentCounterMetric.WithLabelValues(testAgentName, noneOriginalSource, c.clusterName, c.dataType.String(), string(types.SubResourceStatus), string(types.ResyncResponseAction))
				require.Equal(t, len(c.resources), int(toFloat64Counter(sentTotal)))
			}

			cancel()
		})
	}
}

// toFloat64HistCountAndSum returns the count and sum of a histogram metric
func toFloat64HistCountAndSum(h prometheus.Observer) (uint64, float64) {
	var (
		m      prometheus.Metric
		mCount int
		mChan  = make(chan prometheus.Metric)
		done   = make(chan struct{})
	)

	go func() {
		for m = range mChan {
			mCount++
		}
		close(done)
	}()

	c, ok := h.(prometheus.Collector)
	if !ok {
		panic(fmt.Errorf("observer is not a collector; got: %T", h))
	}

	c.Collect(mChan)
	close(mChan)
	<-done

	if mCount != 1 {
		panic(fmt.Errorf("collected %d metrics instead of exactly 1", mCount))
	}

	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		panic(fmt.Errorf("metric write failed, err=%v", err))
	}

	if pb.Histogram != nil {
		return pb.Histogram.GetSampleCount(), pb.Histogram.GetSampleSum()
	}
	panic(fmt.Errorf("collected a non-histogram metric: %s", pb))
}
