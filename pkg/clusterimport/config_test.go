//Package clusterimport contains common utility functions that gets call by many differerent packages
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package clusterimport

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	multicloudv1beta1 "github.ibm.com/IBMPrivateCloud/ibm-klusterlet-operator/pkg/apis/multicloud/v1beta1"
)

func TestNewConfig(t *testing.T) {
	input1 := `
clusterLabels:
  cloud: auto-detect
  vendor: auto-detect
version: latest
applicationManager:
  enabled: true
tillerIntegration:
  enabled: true
prometheusIntegration:
  enabled: true
topologyCollector:
  enabled: true
  updateInterval: 15
searchCollector:
  enabled: true
policyController:
  enabled: true
serviceRegistry:
  enabled: true
  dnsSuffix: mcm.svc
  plugins: kube-service
metering:
  enabled: false
clusterName: test-cluster
clusterNamespace: test-cluster
`

	output1 := &Config{
		EndpointSpec: multicloudv1beta1.EndpointSpec{
			ClusterName:      "test-cluster",
			ClusterNamespace: "test-cluster",
			ClusterLabels: map[string]string{
				"cloud":  "auto-detect",
				"vendor": "auto-detect",
			},
			Version: "latest",
			ApplicationManagerConfig: multicloudv1beta1.EndpointApplicationManagerSpec{
				Enabled: true,
			},
			TillerIntegration: multicloudv1beta1.EndpointTillerIntegrationSpec{
				Enabled: true,
			},
			PrometheusIntegration: multicloudv1beta1.EndpointPrometheusIntegrationSpec{
				Enabled: true,
			},
			TopologyCollectorConfig: multicloudv1beta1.EndpointTopologyCollectorSpec{
				Enabled:                 true,
				CollectorUpdateInterval: 15,
			},
			SearchCollectorConfig: multicloudv1beta1.EndpointSearchCollectorSpec{
				Enabled: true,
			},
			PolicyController: multicloudv1beta1.EndpointPolicyControllerSpec{
				Enabled: true,
			},
			ServiceRegistryConfig: multicloudv1beta1.EndpointServiceRegistrySpec{
				Enabled:   true,
				DNSSuffix: "mcm.svc",
				Plugins:   "kube-service",
			},
			EndpointMeteringConfig: multicloudv1beta1.EndpointMeteringSpec{
				Enabled: false,
			},
			ImageRegistry:    "ibmcom",
			ImageNamePostfix: "",
		},
		RegistryEnabled: false,
		Username:        "",
		Password:        "",
		OperatorImage:   "ibmcom/icp-multicluster-endpoint-operator:latest",
	}

	input2 := `
clusterLabels:
  cloud: auto-detect
  vendor: auto-detect
version: latest
applicationManager:
  enabled: true
tillerIntegration:
  enabled: true
prometheusIntegration:
  enabled: true
topologyCollector:
  enabled: true
  updateInterval: 15
searchCollector:
  enabled: true
policyController:
  enabled: true
serviceRegistry:
  enabled: true
  dnsSuffix: mcm.svc
  plugins: kube-service
metering:
  enabled: false
private_registry_enabled: true
docker_username: user@company.com
docker_password: user_password
imageRegistry: registry.com/project
imageNamePostfix: -amd64
clusterName: test-cluster
clusterNamespace: test-cluster
`

	output2 := &Config{
		EndpointSpec: multicloudv1beta1.EndpointSpec{
			ClusterName:      "test-cluster",
			ClusterNamespace: "test-cluster",
			ClusterLabels: map[string]string{
				"cloud":  "auto-detect",
				"vendor": "auto-detect",
			},
			Version: "latest",
			ApplicationManagerConfig: multicloudv1beta1.EndpointApplicationManagerSpec{
				Enabled: true,
			},
			TillerIntegration: multicloudv1beta1.EndpointTillerIntegrationSpec{
				Enabled: true,
			},
			PrometheusIntegration: multicloudv1beta1.EndpointPrometheusIntegrationSpec{
				Enabled: true,
			},
			TopologyCollectorConfig: multicloudv1beta1.EndpointTopologyCollectorSpec{
				Enabled:                 true,
				CollectorUpdateInterval: 15,
			},
			SearchCollectorConfig: multicloudv1beta1.EndpointSearchCollectorSpec{
				Enabled: true,
			},
			PolicyController: multicloudv1beta1.EndpointPolicyControllerSpec{
				Enabled: true,
			},
			ServiceRegistryConfig: multicloudv1beta1.EndpointServiceRegistrySpec{
				Enabled:   true,
				DNSSuffix: "mcm.svc",
				Plugins:   "kube-service",
			},
			EndpointMeteringConfig: multicloudv1beta1.EndpointMeteringSpec{
				Enabled: false,
			},
			ImageRegistry:    "registry.com/project",
			ImageNamePostfix: "-amd64",
			ImagePullSecret:  "multicluster-endpoint-operator-pull-secret",
		},
		RegistryEnabled: true,
		Username:        "user@company.com",
		Password:        "user_password",
		OperatorImage:   "registry.com/project/icp-multicluster-endpoint-operator-amd64:latest",
	}

	//if no information is given returns the default
	input3 := []byte{}
	output3 := &Config{
		EndpointSpec: multicloudv1beta1.EndpointSpec{
			Version:       "3.2.1",
			ImageRegistry: "ibmcom",
		},
		OperatorImage: "ibmcom/icp-multicluster-endpoint-operator:3.2.1",
	}

	testCases := []struct {
		Input          []byte
		ExpectedOutput *Config
	}{
		{[]byte(input1), output1},
		{[]byte(input2), output2},
		{input3, output3},
	}

	for _, testCase := range testCases {
		output := NewConfig(testCase.Input)
		deepEqual := reflect.DeepEqual(output, testCase.ExpectedOutput)
		assert.True(t, deepEqual, "NewConfig is equal to ExpectedOutput")
	}
}
