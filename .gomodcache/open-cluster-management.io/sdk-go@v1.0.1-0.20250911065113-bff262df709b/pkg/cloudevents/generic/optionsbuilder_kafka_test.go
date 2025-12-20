//go:build kafka

package generic

import (
	"testing"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/kafka"
)

const kafkaConfig = `
bootstrapServer: broker1
groupID: source
clientCertFile: cert
clientKeyFile: key
caFile: ca
auto.commit.interval.ms: 8000
enable.random.seed: false
`

func TestBuildCloudEventsSourceOptionsWithKafka(t *testing.T) {
	cases := []buildingCloudEventsOptionTestCase{
		{
			name:       "kafka config",
			configType: "kafka",
			configFile: configFile(t, "kafka-config-test-", []byte(kafkaConfig)),
			expectedOptions: &kafka.KafkaOptions{
				ConfigMap: confluentkafka.ConfigMap{
					"acks":                                  1,
					"auto.commit.interval.ms":               8000,
					"auto.offset.reset":                     "earliest",
					"bootstrap.servers":                     "broker1",
					"enable.auto.commit":                    true,
					"enable.auto.offset.store":              true,
					"go.events.channel.size":                1000,
					"group.id":                              sourceId,
					"log.connection.close":                  false,
					"queued.max.messages.kbytes":            32768,
					"retries":                               0,
					"security.protocol":                     "ssl",
					"socket.keepalive.enable":               true,
					"ssl.ca.location":                       "ca",
					"ssl.certificate.location":              "cert",
					"ssl.endpoint.identification.algorithm": "none",
					"ssl.key.location":                      "key",
					"enable.random.seed":                    false,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertOptions(t, c)
		})
	}
}
