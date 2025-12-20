package mqtt

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	clienttesting "open-cluster-management.io/sdk-go/pkg/testing"
)

const (
	testYamlConfig = `
brokerHost: test
topics:
  sourceEvents: sources/hub1/clusters/+/sourceevents
  agentEvents: sources/hub1/clusters/+/agentevents
`
	testCustomizedConfig = `
brokerHost: test
keepAlive: 30
dialTimeout: 10m
pubQoS: 0
subQoS: 2
topics:
  sourceEvents: sources/hub1/clusters/+/sourceevents
  agentEvents: sources/hub1/clusters/+/agentevents
`
	testConfig = `
{
	"brokerHost": "test",
	"topics": {
		"sourceEvents": "sources/hub1/clusters/+/sourceevents",
		"agentEvents": "sources/hub1/clusters/+/agentevents"
	}
}
`
)

func TestBuildMQTTOptionsFromFlags(t *testing.T) {
	cases := []struct {
		name             string
		config           string
		expectedOptions  *MQTTOptions
		expectedErrorMsg string
	}{
		{
			name:             "empty config",
			config:           "",
			expectedErrorMsg: "brokerHost is required",
		},
		{
			name:             "tls config without clientCertFile",
			config:           "{\"brokerHost\":\"test\",\"clientCertData\":\"dGVzdAo=\"}",
			expectedErrorMsg: "either both or none of clientCertFile and clientKeyFile must be set",
		},
		{
			name:             "without topics",
			config:           "{\"brokerHost\":\"test\"}",
			expectedErrorMsg: "the topics must be set",
		},
		{
			name:   "default options",
			config: testConfig,
			expectedOptions: &MQTTOptions{
				KeepAlive: 60,
				PubQoS:    1,
				SubQoS:    1,
				Topics: types.Topics{
					SourceEvents: "sources/hub1/clusters/+/sourceevents",
					AgentEvents:  "sources/hub1/clusters/+/agentevents",
				},
				Dialer: &MQTTDialer{
					BrokerHost: "test",
					Timeout:    60 * time.Second,
				},
			},
		},
		{
			name:   "default options with yaml format",
			config: testYamlConfig,
			expectedOptions: &MQTTOptions{
				KeepAlive: 60,
				PubQoS:    1,
				SubQoS:    1,
				Topics: types.Topics{
					SourceEvents: "sources/hub1/clusters/+/sourceevents",
					AgentEvents:  "sources/hub1/clusters/+/agentevents",
				},
				Dialer: &MQTTDialer{
					BrokerHost: "test",
					Timeout:    60 * time.Second,
				},
			},
		},
		{
			name:   "customized options",
			config: testCustomizedConfig,
			expectedOptions: &MQTTOptions{
				KeepAlive: 30,
				PubQoS:    0,
				SubQoS:    2,
				Topics: types.Topics{
					SourceEvents: "sources/hub1/clusters/+/sourceevents",
					AgentEvents:  "sources/hub1/clusters/+/agentevents",
				},
				Dialer: &MQTTDialer{
					BrokerHost: "test",
					Timeout:    10 * time.Minute,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file, err := clienttesting.WriteToTempFile("mqtt-config-test-", []byte(c.config))
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file.Name())

			options, err := BuildMQTTOptionsFromFlags(file.Name())
			if err != nil {
				if err.Error() != c.expectedErrorMsg {
					t.Errorf("unexpected err %v", err)
				}
			}

			if !equality.Semantic.DeepEqual(options, c.expectedOptions) {
				t.Errorf("unexpected options %v", options)
			}
		})
	}
}

func TestValidateTopics(t *testing.T) {
	cases := []struct {
		name        string
		topics      *types.Topics
		expectedErr bool
	}{
		{
			name: "events topics config (clusters)",
			topics: &types.Topics{
				SourceEvents: "sources/maestro/clusters/+/sourceevents",
				AgentEvents:  "sources/maestro/clusters/+/agentevents",
			},
			expectedErr: false,
		},
		{
			name: "events topics config (consumers)",
			topics: &types.Topics{
				SourceEvents: "sources/maestro/consumers/+/sourceevents",
				AgentEvents:  "sources/maestro/consumers/+/agentevents",
			},
			expectedErr: false,
		},
		{
			name: "events topics config (hubs)",
			topics: &types.Topics{
				SourceEvents: "hubs/maestro/consumers/+/sourceevents",
				AgentEvents:  "hubs/maestro/consumers/+/agentevents",
			},
			expectedErr: false,
		},
		{
			name: "shared topics",
			topics: &types.Topics{
				SourceEvents:    "$share/group1/sources/maestro/consumers/+/sourceevents",
				AgentEvents:     "$share/group/sources/maestro/consumers/+/agentevents",
				SourceBroadcast: "$share/source-group/sources/maestro/sourcebroadcast",
				AgentBroadcast:  "$share/agent-group1/clusters/+/agentbroadcast",
			},
			expectedErr: false,
		},
		{
			name: "events topics config (wildcard)",
			topics: &types.Topics{
				SourceEvents:    "sources/+/clusters/+/sourceevents",
				AgentEvents:     "sources/+/clusters/+/agentevents",
				SourceBroadcast: "sources/+/sourcebroadcast",
				AgentBroadcast:  "clusters/+/agentbroadcast",
			},
			expectedErr: false,
		},
		{
			name: "events topics config (no wildcard)",
			topics: &types.Topics{
				SourceEvents:    "sources/maestro/clusters/cluster-1/sourceevents",
				AgentEvents:     "sources/maestro/clusters/cluster-1/agentevents",
				SourceBroadcast: "sources/maestro/sourcebroadcast",
				AgentBroadcast:  "clusters/cluster1/agentbroadcast",
			},
			expectedErr: false,
		},
		{
			name: "source topics config",
			topics: &types.Topics{
				SourceEvents:    "sources/maestro/clusters/+/sourceevents",
				AgentEvents:     "sources/maestro/clusters/+/agentevents",
				SourceBroadcast: "sources/maestro/sourcebroadcast",
				AgentBroadcast:  "clusters/+/agentbroadcast",
			},
			expectedErr: false,
		},
		{
			name: "source topics config (uuid)",
			topics: &types.Topics{
				SourceEvents:    "sources/5328eff5-b0c7-48f3-b82e-10052abbf51d/clusters/+/sourceevents",
				AgentEvents:     "sources/5328eff5-b0c7-48f3-b82e-10052abbf51d/clusters/+/agentevents",
				SourceBroadcast: "sources/5328eff5-b0c7-48f3-b82e-10052abbf51d/sourcebroadcast",
				AgentBroadcast:  "clusters/+/agentbroadcast",
			},
			expectedErr: false,
		},
		{
			name: "agent topics config (multiple sources)",
			topics: &types.Topics{
				SourceEvents:    "sources/+/clusters/+/sourceevents",
				AgentEvents:     "sources/+/clusters/+/agentevents",
				SourceBroadcast: "sources/+/sourcebroadcast",
				AgentBroadcast:  "clusters/+/agentbroadcast",
			},
			expectedErr: false,
		},
		{
			name: "agent topics config (multiple sources)",
			topics: &types.Topics{
				SourceEvents:    "sources/+/clusters/+/sourceevents",
				AgentEvents:     "sources/+/clusters/+/agentevents",
				SourceBroadcast: "sources/+/sourcebroadcast",
				AgentBroadcast:  "clusters/+/agentbroadcast",
			},
			expectedErr: false,
		},
		{
			name:        "no topics",
			topics:      nil,
			expectedErr: true,
		},
		{
			name:        "empty topics",
			topics:      &types.Topics{},
			expectedErr: true,
		},
		{
			name: "bad topics",
			topics: &types.Topics{
				SourceEvents:    "sources/+/clusters/+/agentevents",
				AgentEvents:     "sources/+/clusters/+/sourceevents",
				SourceBroadcast: "sources/+/specresync",
				AgentBroadcast:  "clusters/+/statusresync",
			},
			expectedErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateTopics(c.topics)
			if c.expectedErr {
				if err == nil {
					t.Errorf("expected error, but failed")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetSourceFromEventsTopic(t *testing.T) {
	cases := []struct {
		name           string
		topic          string
		expectedSource string
	}{
		{
			name:           "get source from agent events share topic",
			topic:          "$share/group/sources/source1/consumers/+/agentevents",
			expectedSource: "source1",
		},
		{
			name:           "get source from agent events topic",
			topic:          "sources/source2/consumers/+/agentevents",
			expectedSource: "source2",
		},
		{
			name:           "get source from source events share topic",
			topic:          "$share/group/sources/source3/consumers/+/sourceevents",
			expectedSource: "source3",
		},
		{
			name:           "get source from source events topic",
			topic:          "sources/source4/consumers/+/sourceevents",
			expectedSource: "source4",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source, err := getSourceFromEventsTopic(c.topic)
			if err != nil {
				t.Errorf("unexpected error %v", err)
			}

			if source != c.expectedSource {
				t.Errorf("expected source %q, but %q", c.expectedSource, source)
			}
		})
	}
}

func TestConnectionTimeout(t *testing.T) {
	ln := newLocalListener(t)
	defer ln.Close()

	config := strings.Replace(testYamlConfig, "test", ln.Addr().String(), 1)
	file, err := clienttesting.WriteToTempFile("mqtt-config-test-", []byte(config))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	options, err := BuildMQTTOptionsFromFlags(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	options.Dialer.Timeout = 10 * time.Millisecond

	agentOptions := &mqttAgentOptions{
		MQTTOptions: *options,
		clusterName: "cluster1",
	}
	_, err = agentOptions.Protocol(context.TODO(), types.CloudEventsDataType{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("%T, %v", err, err)
	}
}

func newLocalListener(t *testing.T) net.Listener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		ln, err = net.Listen("tcp6", "[::1]:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	return ln
}
