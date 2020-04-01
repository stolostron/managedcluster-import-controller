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

	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	corev1 "k8s.io/api/core/v1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

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

func newImportSecret(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	// get endpoint crd yaml
	endpointCRD, err := clusterimport.GenerateEndpointCRD()
	if err != nil {
		return nil, err
	}
	endpointCRDYAML, err := toYAML(endpointCRD)
	if err != nil {
		return nil, err
	}
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
			"import.yaml":       importYAML,
			"endpoint-crd.yaml": endpointCRDYAML,
		},
	}

	return secret, nil
}

func createImportSecret(
	client client.Client,
	scheme *runtime.Scheme,
	cluster *clusterregistryv1alpha1.Cluster,
	endpointConfig *multicloudv1alpha1.EndpointConfig,
) (*corev1.Secret, error) {
	secret, err := newImportSecret(client, endpointConfig)
	if err != nil {
		return nil, err
	}
	if err := controllerutil.SetControllerReference(cluster, secret, scheme); err != nil {
		return nil, err
	}

	if err := client.Create(context.TODO(), secret); err != nil {
		return nil, err
	}

	return secret, nil
}

func updateImportSecret(
	client client.Client,
	endpointConfig *multicloudv1alpha1.EndpointConfig,
	oldImportSecret *corev1.Secret,
) (*corev1.Secret, error) {
	secret, err := newImportSecret(client, endpointConfig)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(oldImportSecret.Data["import.yaml"], secret.Data["import.yaml"]) {
		oldImportSecret.Data = secret.Data
		if err := client.Update(context.TODO(), oldImportSecret); err != nil {
			return nil, err
		}
	}

	return oldImportSecret, nil
}

// getPrreProcessedJSON will return json of the given object. If the given item is a CRD, we will remove status fields.
func getPreProcessedJSON(item runtime.Object) ([]byte, error) {
	oldJSONObj, err := json.Marshal(item)
	newJSONObj := oldJSONObj
	if err != nil {
		return nil, fmt.Errorf("error marshaling into JSON: %v", err)
	}

	//remove status of crd
	if _, ok := item.(*apiextensionv1beta1.CustomResourceDefinition); ok {
		patchJSON := []byte(`[{"op": "remove", "path": "/status"}]`)
		patch, err := jsonpatch.DecodePatch(patchJSON)
		if err != nil {
			return nil, err
		}
		newJSONObj, err = patch.Apply(oldJSONObj)
		if err != nil {
			return nil, err
		}
	}

	return newJSONObj, nil
}

func toYAML(runtimeObjects []runtime.Object) ([]byte, error) {
	buf := new(bytes.Buffer)

	for _, item := range runtimeObjects {
		if _, err := buf.WriteString("\n---\n"); err != nil {
			return nil, err
		}
		j, err := getPreProcessedJSON(item)
		if err != nil {
			return nil, err
		}
		y, err := yaml.JSONToYAML(j)
		if err != nil {
			return nil, fmt.Errorf("error converting JSON to YAML: %v", err)
		}

		if _, err := buf.Write(y); err != nil {
			return nil, err
		}

	}

	return buf.Bytes(), nil
}
