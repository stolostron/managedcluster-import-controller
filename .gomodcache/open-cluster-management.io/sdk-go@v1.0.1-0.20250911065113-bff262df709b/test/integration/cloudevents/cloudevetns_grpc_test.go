package cloudevents

import (
	"context"

	"github.com/onsi/ginkgo"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/constants"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options"
	grpcoptions "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("CloudEvents Clients Test - GRPC", runCloudeventsClientPubSubTest(GetGRPCSourceOptions))

// The GRPC test simulates there is a server between the source and agent, the GRPC source client
// sends/receives events to/from server, then server forward the events to agent via GRPC broker.
func GetGRPCSourceOptions(ctx context.Context, sourceID string) (*options.CloudEventsSourceOptions, string) {
	return grpcoptions.NewSourceOptions(util.NewGRPCSourceOptions(grpcServerHost), sourceID), constants.ConfigTypeGRPC
}
