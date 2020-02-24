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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func imagePullSecretNsN(endpointConfig *multicloudv1alpha1.EndpointConfig) types.NamespacedName {
	return types.NamespacedName{
		Name:      endpointConfig.Spec.ImagePullSecret,
		Namespace: endpointConfig.Namespace,
	}
}

func getImagePullSecret(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	if endpointConfig.Spec.ImagePullSecret == "" {
		return nil, nil
	}

	secretNsN := imagePullSecretNsN(endpointConfig)

	secret := &corev1.Secret{}

	if err := client.Get(context.TODO(), secretNsN, secret); err != nil {
		return nil, err
	}

	if secret.Type != corev1.SecretTypeDockerConfigJson {
		return nil, fmt.Errorf("secret is not of type corev1.SecretTypeDockerConfigJson")
	}

	return secret, nil
}
