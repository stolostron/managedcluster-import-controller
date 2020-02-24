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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

// BootstrapServiceAccountNamePostfix is the postfix for bootstrap service account
const BootstrapServiceAccountNamePostfix = "-bootstrap-sa"

func bootstrapServiceAccountNsN(endpointConfig *multicloudv1alpha1.EndpointConfig) (types.NamespacedName, error) {
	if endpointConfig == nil {
		return types.NamespacedName{}, fmt.Errorf("endpontConfig can not be nil")
	}

	if endpointConfig.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("endpontConfig can not have empty ClusterName")
	}

	return types.NamespacedName{
		Name:      endpointConfig.Spec.ClusterName + BootstrapServiceAccountNamePostfix,
		Namespace: endpointConfig.Namespace,
	}, nil
}

func newBootstrapServiceAccount(endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.ServiceAccount, error) {
	saNsN, err := bootstrapServiceAccountNsN(endpointConfig)
	if err != nil {
		return nil, err
	}

	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saNsN.Name,
			Namespace: saNsN.Namespace,
		},
	}, nil
}

// GetBootstrapServiceAccount get the service account use for multicluster-endpoint bootstrap
func GetBootstrapServiceAccount(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.ServiceAccount, error) {
	saNsN, err := bootstrapServiceAccountNsN(endpointConfig)
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{}

	if err := client.Get(context.TODO(), saNsN, sa); err != nil {
		return nil, err
	}

	return sa, nil
}

// CreateBootstrapServiceAccount create the service account use for multicluster-endpoint bootstrap
func CreateBootstrapServiceAccount(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.ServiceAccount, error) {
	sa, err := newBootstrapServiceAccount(endpointConfig)
	if err != nil {
		return nil, err
	}

	if err := client.Create(context.TODO(), sa); err != nil {
		return nil, err
	}

	return sa, nil
}

func getBootstrapTokenSecret(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	sa, err := GetBootstrapServiceAccount(client, endpointConfig)
	if err != nil {
		return nil, err
	}

	for _, secret := range sa.Secrets {
		secretNsN := types.NamespacedName{
			Name:      secret.Name,
			Namespace: sa.Namespace,
		}

		saSecret := &corev1.Secret{}
		if err := client.Get(context.TODO(), secretNsN, saSecret); err != nil {
			continue
		}

		if saSecret.Type == corev1.SecretTypeServiceAccountToken {
			return saSecret, nil
		}
	}

	return nil, fmt.Errorf("fail to find service account token secret")
}

func getBootstrapToken(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) ([]byte, error) {
	secret, err := getBootstrapTokenSecret(client, endpointConfig)
	if err != nil {
		return nil, err
	}

	token, ok := secret.Data["token"]
	if !ok {
		return nil, fmt.Errorf("data of bootstrap serviceaccount token secret does not contain token")
	}

	return token, nil
}
