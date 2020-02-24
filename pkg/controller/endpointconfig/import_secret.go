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

package endpointconfig

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
	"github.com/open-cluster-management/rcm-controller/pkg/clusterimport"
)

const importSecretNamePostfix = "-import"

func importSecretNsN(endpointConfig *multicloudv1alpha1.EndpointConfig) (types.NamespacedName, error) {
	if endpointConfig == nil {
		return types.NamespacedName{}, fmt.Errorf("nil EndpointConfig")
	}

	if endpointConfig.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("empty EndpointConfig.Spec.ClusterName")
	}

	if endpointConfig.Spec.ClusterNamespace == "" {
		return types.NamespacedName{}, fmt.Errorf("empty EndpointConfig.Spec.ClusterNamespace")
	}

	return types.NamespacedName{
		Name:      endpointConfig.Spec.ClusterName + importSecretNamePostfix,
		Namespace: endpointConfig.Spec.ClusterNamespace,
	}, nil
}

func getImportSecret(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	secretNsN, err := importSecretNsN(endpointConfig)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{}

	if err := client.Get(context.TODO(), secretNsN, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

func newImportSecret(client client.Client, scheme *runtime.Scheme, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	runtimeObjects, err := clusterimport.GenerateImportObjects(client, endpointConfig)
	if err != nil {
		return nil, err
	}

	importYAML, err := toYAML(runtimeObjects)
	if err != nil {
		return nil, err
	}

	sNsN, err := importSecretNsN(endpointConfig)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sNsN.Name,
			Namespace: sNsN.Namespace,
		},
		Data: map[string][]byte{
			"import.yaml": importYAML,
		},
	}

	if err := controllerutil.SetControllerReference(endpointConfig, secret, scheme); err != nil {
		return nil, err
	}

	return secret, nil
}

func createImportSecret(client client.Client, scheme *runtime.Scheme, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	secret, err := newImportSecret(client, scheme, endpointConfig)
	if err != nil {
		return nil, err
	}

	if err := client.Create(context.TODO(), secret); err != nil {
		return nil, err
	}

	return secret, nil
}

func toYAML(runtimeObjects []runtime.Object) ([]byte, error) {
	buf := new(bytes.Buffer)

	for _, item := range runtimeObjects {
		if _, err := buf.WriteString("\n---\n"); err != nil {
			return nil, err
		}

		s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
		if err := s.Encode(item, buf); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
