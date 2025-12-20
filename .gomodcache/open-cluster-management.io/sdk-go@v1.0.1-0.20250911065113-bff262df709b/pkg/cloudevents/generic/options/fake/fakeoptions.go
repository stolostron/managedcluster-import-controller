package fake

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

type CloudEventsFakeOptions struct {
	protocol options.CloudEventsProtocol
	errChan  chan error
}

func NewAgentOptions(protocol options.CloudEventsProtocol, errChan chan error, clusterName, agentID string) *options.CloudEventsAgentOptions {
	return &options.CloudEventsAgentOptions{
		CloudEventsOptions: &CloudEventsFakeOptions{protocol: protocol, errChan: errChan},
		AgentID:            agentID,
		ClusterName:        clusterName,
	}
}

func NewSourceOptions(protocol options.CloudEventsProtocol, sourceID string) *options.CloudEventsSourceOptions {
	return &options.CloudEventsSourceOptions{
		CloudEventsOptions: &CloudEventsFakeOptions{protocol: protocol},
		SourceID:           sourceID,
	}
}

func (o *CloudEventsFakeOptions) WithContext(ctx context.Context, evtCtx cloudevents.EventContext) (context.Context, error) {
	return ctx, nil
}

func (o *CloudEventsFakeOptions) Protocol(ctx context.Context, dataType types.CloudEventsDataType) (options.CloudEventsProtocol, error) {
	return o.protocol, nil
}

func (o *CloudEventsFakeOptions) ErrorChan() <-chan error {
	return o.errChan
}
