//go:build kafka

package kafka

import (
	"os"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/require"
	clienttesting "open-cluster-management.io/sdk-go/pkg/testing"
)

func TestBuildKafkaOptionsFromFlags(t *testing.T) {
	cases := []struct {
		name             string
		config           string
		expectedOptions  *KafkaOptions
		expectedErrorMsg string
	}{
		{
			name:             "empty configs",
			config:           "",
			expectedErrorMsg: "bootstrapServer is required",
		},
		{
			name:             "tls config without clientCertFile",
			config:           `{"bootstrapServer":"test","groupID":"test","clientCertFile":"test"}`,
			expectedErrorMsg: "either both or none of clientCertFile and clientKeyFile must be set",
		},
		{
			name:             "tls config without caFile",
			config:           `{"bootstrapServer":"test","groupID":"test","clientCertFile":"test","clientKeyFile":"test"}`,
			expectedErrorMsg: "setting clientCertFile and clientKeyFile requires caFile",
		},
		{
			name:   "options without ssl",
			config: `{"bootstrapServer":"testBroker","groupID":"testGroupID"}`,
			expectedOptions: &KafkaOptions{
				ConfigMap: kafka.ConfigMap{
					"acks":                                  1,
					"auto.commit.interval.ms":               5000,
					"auto.offset.reset":                     "earliest",
					"bootstrap.servers":                     "testBroker",
					"enable.auto.commit":                    true,
					"enable.auto.offset.store":              true,
					"go.events.channel.size":                1000,
					"group.id":                              "testGroupID",
					"log.connection.close":                  false,
					"queued.max.messages.kbytes":            32768,
					"retries":                               0,
					"socket.keepalive.enable":               true,
					"ssl.endpoint.identification.algorithm": "none",
				},
			},
		},

		{
			name:   "options with ssl",
			config: `{"bootstrapServer":"broker1","groupID":"id","clientCertFile":"cert","clientKeyFile":"key","caFile":"ca"}`,
			expectedOptions: &KafkaOptions{
				ConfigMap: kafka.ConfigMap{
					"acks":                                  1,
					"auto.commit.interval.ms":               5000,
					"auto.offset.reset":                     "earliest",
					"bootstrap.servers":                     "broker1",
					"enable.auto.commit":                    true,
					"enable.auto.offset.store":              true,
					"go.events.channel.size":                1000,
					"group.id":                              "id",
					"log.connection.close":                  false,
					"queued.max.messages.kbytes":            32768,
					"retries":                               0,
					"security.protocol":                     "ssl",
					"socket.keepalive.enable":               true,
					"ssl.ca.location":                       "ca",
					"ssl.certificate.location":              "cert",
					"ssl.endpoint.identification.algorithm": "none",
					"ssl.key.location":                      "key",
				},
			},
		},
		{
			name:   "options with advanced config to disable ssl",
			config: `{"bootstrapServer":"broker1","groupID":"id","clientCertFile":"cert","clientKeyFile":"key","caFile":"ca","security.protocol":"plaintext"}`,
			expectedOptions: &KafkaOptions{
				ConfigMap: kafka.ConfigMap{
					"acks":                                  1,
					"auto.commit.interval.ms":               5000,
					"auto.offset.reset":                     "earliest",
					"bootstrap.servers":                     "broker1",
					"enable.auto.commit":                    true,
					"enable.auto.offset.store":              true,
					"go.events.channel.size":                1000,
					"group.id":                              "id",
					"log.connection.close":                  false,
					"queued.max.messages.kbytes":            32768,
					"retries":                               0,
					"security.protocol":                     "plaintext",
					"socket.keepalive.enable":               true,
					"ssl.ca.location":                       "ca",
					"ssl.certificate.location":              "cert",
					"ssl.endpoint.identification.algorithm": "none",
					"ssl.key.location":                      "key",
				},
			},
		},
		{
			name:   "options with advanced config to override default",
			config: `{"bootstrapServer":"broker1","groupID":"id","clientCertFile":"cert","clientKeyFile":"key","caFile":"ca","enable.auto.commit":false,"auto.offset.reset":"smallest","queued.max.messages.kbytes":10000}`,
			expectedOptions: &KafkaOptions{
				ConfigMap: kafka.ConfigMap{
					"acks":                                  1,
					"auto.commit.interval.ms":               5000,
					"auto.offset.reset":                     "smallest",
					"bootstrap.servers":                     "broker1",
					"enable.auto.commit":                    false,
					"enable.auto.offset.store":              true,
					"go.events.channel.size":                1000,
					"group.id":                              "id",
					"log.connection.close":                  false,
					"queued.max.messages.kbytes":            10000,
					"retries":                               0,
					"security.protocol":                     "ssl",
					"socket.keepalive.enable":               true,
					"ssl.ca.location":                       "ca",
					"ssl.certificate.location":              "cert",
					"ssl.endpoint.identification.algorithm": "none",
					"ssl.key.location":                      "key",
				},
			},
		},
		{
			name:   "options with advanced config to add more config",
			config: `{"bootstrapServer":"broker1","groupID":"id","clientCertFile":"cert","clientKeyFile":"key","caFile":"ca","debug":"generic","message.max.bytes":20000,"enable.random.seed":false}`,
			expectedOptions: &KafkaOptions{
				ConfigMap: kafka.ConfigMap{
					"acks":                                  1,
					"auto.commit.interval.ms":               5000,
					"auto.offset.reset":                     "earliest",
					"bootstrap.servers":                     "broker1",
					"enable.auto.commit":                    true,
					"enable.auto.offset.store":              true,
					"go.events.channel.size":                1000,
					"group.id":                              "id",
					"log.connection.close":                  false,
					"queued.max.messages.kbytes":            32768,
					"retries":                               0,
					"security.protocol":                     "ssl",
					"socket.keepalive.enable":               true,
					"ssl.ca.location":                       "ca",
					"ssl.certificate.location":              "cert",
					"ssl.endpoint.identification.algorithm": "none",
					"ssl.key.location":                      "key",
					"debug":                                 "generic",
					"message.max.bytes":                     20000,
					"enable.random.seed":                    false,
				},
			},
		},
		{
			name:   "options with advanced config to add complicated config",
			config: `{"bootstrapServer":"broker1","groupID":"id","clientCertFile":"cert","clientKeyFile":"key","caFile":"ca","sasl.kerberos.kinit.cmd":"kinit -R -t \"%{sasl.kerberos.keytab}\" -k %{sasl.kerberos.principal} || kinit -t \"%{sasl.kerberos.keytab}\" -k %{sasl.kerberos.principal}"}`,
			expectedOptions: &KafkaOptions{
				ConfigMap: kafka.ConfigMap{
					"acks":                                  1,
					"auto.commit.interval.ms":               5000,
					"auto.offset.reset":                     "earliest",
					"bootstrap.servers":                     "broker1",
					"enable.auto.commit":                    true,
					"enable.auto.offset.store":              true,
					"go.events.channel.size":                1000,
					"group.id":                              "id",
					"log.connection.close":                  false,
					"queued.max.messages.kbytes":            32768,
					"retries":                               0,
					"security.protocol":                     "ssl",
					"socket.keepalive.enable":               true,
					"ssl.ca.location":                       "ca",
					"ssl.certificate.location":              "cert",
					"ssl.endpoint.identification.algorithm": "none",
					"ssl.key.location":                      "key",
					"sasl.kerberos.kinit.cmd":               "kinit -R -t \"%{sasl.kerberos.keytab}\" -k %{sasl.kerberos.principal} || kinit -t \"%{sasl.kerberos.keytab}\" -k %{sasl.kerberos.principal}",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file, err := clienttesting.WriteToTempFile("kafka-config-test-", []byte(c.config))
			require.Nil(t, err)
			defer os.Remove(file.Name())

			options, err := BuildKafkaOptionsFromFlags(file.Name())
			if c.expectedErrorMsg != "" {
				require.Equal(t, c.expectedErrorMsg, err.Error())
			} else {
				require.Nil(t, err, "failed to get kafkaOptions")
			}
			if c.expectedOptions != nil {
				require.EqualValues(t, c.expectedOptions, options)
			}
		})
	}
}
