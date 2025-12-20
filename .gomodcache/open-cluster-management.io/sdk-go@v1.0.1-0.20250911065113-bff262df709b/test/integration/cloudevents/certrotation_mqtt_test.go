package cloudevents

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/onsi/ginkgo"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/cert"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/mqtt"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("CloudEvents Certificate Rotation Test - MQTT", runCloudeventsCertRotationTest(GetMQTTAgentOptions))

func GetMQTTAgentOptions(_ context.Context, agentID, clusterName, clientCertFile, clientKeyFile string) *options.CloudEventsAgentOptions {
	mqttOptions := newTLSMQTTOptions(certPool, mqttTLSBrokerHost, clientCertFile, clientKeyFile)
	return mqtt.NewAgentOptions(mqttOptions, clusterName, agentID)
}

func newTLSMQTTOptions(certPool *x509.CertPool, brokerHost, clientCertFile, clientKeyFile string) *mqtt.MQTTOptions {
	o := &mqtt.MQTTOptions{
		KeepAlive: 60,
		PubQoS:    1,
		SubQoS:    1,
		Topics: types.Topics{
			SourceEvents: "sources/certrotationtest/clusters/+/sourceevents",
			AgentEvents:  "sources/certrotationtest/clusters/+/agentevents",
		},
		Dialer: &mqtt.MQTTDialer{
			BrokerHost: brokerHost,
			Timeout:    5 * time.Second,
			TLSConfig: &tls.Config{
				RootCAs: certPool,
				GetClientCertificate: func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
					return cert.CachingCertificateLoader(util.ReloadCerts(clientCertFile, clientKeyFile))()
				},
			},
		},
	}

	cert.StartClientCertRotating(o.Dialer.TLSConfig.GetClientCertificate, o.Dialer)

	return o
}
