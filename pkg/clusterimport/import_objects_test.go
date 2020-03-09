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
	"os"
	"testing"

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestNewOperatorDeployment(t *testing.T) {
	type args struct {
		endpointConfig  *multicloudv1alpha1.EndpointConfig
		imageTagPostfix string
	}
	type expectValues struct {
		imageName          string
		imageTagPostfixEnv string
	}

	tests := []struct {
		name string
		args args
		want expectValues
	}{
		{
			name: "Empty Postfix",
			args: args{
				endpointConfig: &multicloudv1alpha1.EndpointConfig{
					Spec: multicloudv1beta1.EndpointSpec{
						ImageRegistry: "sample-registry/uniquePath",
						Version:       "2.3.0",
					},
				},
				imageTagPostfix: "",
			},
			want: expectValues{"sample-registry/uniquePath/endpoint-operator:2.3.0", ""},
		},
		{
			name: "With Postfix Set",
			args: args{
				endpointConfig: &multicloudv1alpha1.EndpointConfig{
					Spec: multicloudv1beta1.EndpointSpec{
						ImageRegistry: "sample-registry-2/uniquePath-2",
						Version:       "1.2.0",
					},
				},
				imageTagPostfix: "-Unique-Postfix",
			},
			want: expectValues{"sample-registry-2/uniquePath-2/endpoint-operator:1.2.0-Unique-Postfix", "-Unique-Postfix"},
		},
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			err := os.Setenv(ImageTagPostfixKey, tt.args.imageTagPostfix)
			if err != nil {
				t.Errorf("Cannot set env %s", ImageTagPostfixKey)
			}
			deployment := newOperatorDeployment(tt.args.endpointConfig)
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Image, tt.want.imageName, "image name should match")
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Env[3].Name, ImageTagPostfixKey)
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Env[3].Value, tt.want.imageTagPostfixEnv, "tag postfix should be passed to env")
		})
	}
}
