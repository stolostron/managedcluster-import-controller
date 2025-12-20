package grpc

import (
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	clienttesting "open-cluster-management.io/sdk-go/pkg/testing"
)

func TestBuildGRPCOptionsFromFlags(t *testing.T) {
	cases := []struct {
		name             string
		config           string
		expectedOptions  *GRPCOptions
		expectedErrorMsg string
	}{
		{
			name:             "empty config",
			config:           "",
			expectedErrorMsg: "url is required",
		},
		{
			name:             "tls config without clientCertFile",
			config:           "{\"url\":\"test\",\"clientCertData\":\"dGVzdAo=\"}",
			expectedErrorMsg: "either both or none of clientCertFile and clientKeyFile must be set",
		},
		{
			name:             "token config without caFile",
			config:           "{\"url\":\"test\",\"token\":\"test\"}",
			expectedErrorMsg: "setting token requires authority certificates",
		},
		{
			name:   "customized options",
			config: "{\"url\":\"test\"}",
			expectedOptions: &GRPCOptions{
				&GRPCDialer{
					URL: "test",
					KeepAliveOptions: KeepAliveOptions{
						Enable:              false,
						Time:                30 * time.Second,
						Timeout:             10 * time.Second,
						PermitWithoutStream: false,
					},
				},
			},
		},
		{
			name:   "customized options with yaml format",
			config: "url: test",
			expectedOptions: &GRPCOptions{
				&GRPCDialer{
					URL: "test",
					KeepAliveOptions: KeepAliveOptions{
						Enable:              false,
						Time:                30 * time.Second,
						Timeout:             10 * time.Second,
						PermitWithoutStream: false,
					},
				},
			},
		},
		{
			name:   "customized options with keepalive",
			config: "{\"url\":\"test\",\"keepAliveConfig\":{\"enable\":true,\"time\":10s,\"timeout\":5s,\"permitWithoutStream\":true}}",
			expectedOptions: &GRPCOptions{
				&GRPCDialer{
					URL: "test",
					KeepAliveOptions: KeepAliveOptions{
						Enable:              true,
						Time:                10 * time.Second,
						Timeout:             5 * time.Second,
						PermitWithoutStream: true,
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file, err := clienttesting.WriteToTempFile("grpc-config-test-", []byte(c.config))
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file.Name())

			options, err := BuildGRPCOptionsFromFlags(file.Name())
			if err != nil {
				if err.Error() != c.expectedErrorMsg {
					t.Errorf("unexpected err %v", err)
				}
			}

			if !cmp.Equal(options, c.expectedOptions, cmpopts.IgnoreUnexported(GRPCDialer{})) {
				t.Errorf("unexpected options %+v", options)
			}
		})
	}
}
