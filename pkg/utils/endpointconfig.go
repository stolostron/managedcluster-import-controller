//Package utils contains common utility functions that gets call by many differerent packages
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
package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func endpointConfigNsN(name string, namespace string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
}

// GetEndpointConfig - Get the endpoint config
func GetEndpointConfig(client client.Client, name string, namespace string) (*multicloudv1alpha1.EndpointConfig, error) {
	ncNsN := endpointConfigNsN(name, namespace)
	nc := &multicloudv1alpha1.EndpointConfig{}

	if err := client.Get(context.TODO(), ncNsN, nc); err != nil {
		return nil, err
	}

	return nc, nil
}

// DeleteEndpointConfig - Delete the endpoint config
func DeleteEndpointConfig(client client.Client, name string, namespace string) error {
	endpointConfig, err := GetEndpointConfig(client, name, namespace)
	if err != nil {
		return err
	}

	if err := client.Delete(context.TODO(), endpointConfig); err != nil {
		return err
	}

	return nil
}
