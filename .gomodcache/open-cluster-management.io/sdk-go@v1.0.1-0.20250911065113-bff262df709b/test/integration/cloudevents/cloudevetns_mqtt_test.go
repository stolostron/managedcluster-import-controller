package cloudevents

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/constants"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/mqtt"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("CloudEvents Clients Test - MQTT", runCloudeventsClientPubSubTest(GetMQTTSourceOptions))

func GetMQTTSourceOptions(_ context.Context, sourceID string) (*options.CloudEventsSourceOptions, string) {
	return mqtt.NewSourceOptions(
		util.NewMQTTSourceOptions(mqttBrokerHost, sourceID),
		fmt.Sprintf("%s-client", sourceID),
		sourceID,
	), constants.ConfigTypeMQTT
}
