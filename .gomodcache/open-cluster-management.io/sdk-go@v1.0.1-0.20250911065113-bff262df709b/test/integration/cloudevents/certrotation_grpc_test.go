package cloudevents

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/onsi/ginkgo"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/cert"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("CloudEvents Certificate Rotation Test - GRPC", runCloudeventsCertRotationTest(GetGRPCAgentOptions))

func GetGRPCAgentOptions(_ context.Context, agentID, clusterName, clientCertFile, clientKeyFile string) *options.CloudEventsAgentOptions {
	grpcOptions := newTLSGRPCOptions(certPool, grpcBrokerHost, clientCertFile, clientKeyFile)
	return grpc.NewAgentOptions(grpcOptions, clusterName, agentID)
}

func newTLSGRPCOptions(certPool *x509.CertPool, brokerHost, clientCertFile, clientKeyFile string) *grpc.GRPCOptions {
	o := &grpc.GRPCOptions{
		Dialer: &grpc.GRPCDialer{
			URL: brokerHost,
			TLSConfig: &tls.Config{
				RootCAs: certPool,
				GetClientCertificate: func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
					return cert.CachingCertificateLoader(util.ReloadCerts(clientCertFile, clientKeyFile))()
				},
			},
			KeepAliveOptions: grpc.KeepAliveOptions{
				Enable:              true,
				Time:                10 * time.Second,
				Timeout:             5 * time.Second,
				PermitWithoutStream: true,
			},
		},
	}

	cert.StartClientCertRotating(o.Dialer.TLSConfig.GetClientCertificate, o.Dialer)
	return o
}
