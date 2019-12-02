//Package clusterimport ...
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
	"fmt"

	"github.com/ghodss/yaml"
	multicloudv1beta1 "github.ibm.com/IBMPrivateCloud/ibm-klusterlet-operator/pkg/apis/multicloud/v1beta1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// DefaultImageRegistry ...
const DefaultImageRegistry = "ibmcom"

// DefaultOperatorImage ...
const DefaultOperatorImage = "icp-multicluster-endpoint-operator"

// DefaultOperatorImageTag ...
const DefaultOperatorImageTag = "3.2.1"

// DefaultImagePullSecretName ...
const DefaultImagePullSecretName = "multicluster-endpoint-operator-pull-secret"

// Config contain the endpoint config and the registry information
type Config struct {
	multicloudv1beta1.EndpointSpec `json:",inline"`
	RegistryEnabled                bool   `json:"private_registry_enabled"`
	Username                       string `json:"docker_username"`
	Password                       string `json:"docker_password"`
	OperatorImage                  string `json:"opeator_image"`
}

var log = logf.Log.WithName("clusterimport_config")

// NewConfig create Config from the configBytes in the cluster secret
func NewConfig(configBytes []byte) *Config {
	importConfig := &Config{}

	if err := yaml.Unmarshal(configBytes, importConfig); err != nil {
		log.Error(err, "error unmarshalling import config yaml")
	}

	if importConfig.ImageRegistry == "" {
		importConfig.ImageRegistry = DefaultImageRegistry
	}

	if importConfig.Version == "" {
		importConfig.Version = DefaultOperatorImageTag
	}

	if importConfig.OperatorImage == "" {
		importConfig.OperatorImage = fmt.Sprintf("%s/%s%s:%s", importConfig.ImageRegistry, DefaultOperatorImage, importConfig.ImageNamePostfix, importConfig.Version)
	}

	if importConfig.RegistryEnabled {
		importConfig.ImagePullSecret = DefaultImagePullSecretName
	}

	return importConfig
}
