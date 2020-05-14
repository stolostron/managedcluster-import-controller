// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package klusterletconfig

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

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
	"github.com/open-cluster-management/rcm-controller/pkg/clusterimport"
)

const importSecretNamePostfix = "-import"

func importSecretNsN(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (types.NamespacedName, error) {
	if klusterletConfig == nil {
		return types.NamespacedName{}, fmt.Errorf("nil KlusterletConfig")
	}

	if klusterletConfig.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("empty KlusterletConfig.Spec.ClusterName")
	}

	if klusterletConfig.Spec.ClusterNamespace == "" {
		return types.NamespacedName{}, fmt.Errorf("empty KlusterletConfig.Spec.ClusterNamespace")
	}

	return types.NamespacedName{
		Name:      klusterletConfig.Spec.ClusterName + importSecretNamePostfix,
		Namespace: klusterletConfig.Spec.ClusterNamespace,
	}, nil
}

func getImportSecret(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.Secret, error) {
	secretNsN, err := importSecretNsN(klusterletConfig)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{}

	if err := client.Get(context.TODO(), secretNsN, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

func newImportSecret(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.Secret, error) {
	// get klusterlet crd yaml
	klusterletCRD, err := clusterimport.GenerateKlusterletCRD()
	if err != nil {
		return nil, err
	}
	klusterletCRDYAML, err := toYAML(klusterletCRD)
	if err != nil {
		return nil, err
	}
	runtimeObjects, err := clusterimport.GenerateImportObjects(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	importYAML, err := toYAML(runtimeObjects)
	if err != nil {
		return nil, err
	}

	sNsN, err := importSecretNsN(klusterletConfig)
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
			"import.yaml":         importYAML,
			"klusterlet-crd.yaml": klusterletCRDYAML,
		},
	}

	return secret, nil
}

func createImportSecret(
	client client.Client,
	scheme *runtime.Scheme,
	cluster *clusterregistryv1alpha1.Cluster,
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
) (*corev1.Secret, error) {
	secret, err := newImportSecret(client, klusterletConfig)
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
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
	oldImportSecret *corev1.Secret,
) (*corev1.Secret, error) {
	secret, err := newImportSecret(client, klusterletConfig)
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
