// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"bytes"
	"context"
	"fmt"

	"github.com/open-cluster-management/applier/pkg/templateprocessor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	importSecretNamePostfix = "-import"
	importYAMLKey           = "import.yaml"
	crdsYAMLKey             = "crds.yaml"
	crdsV1YAMLKey           = "crdsv1.yaml"
	crdsV1beta1YAMLKey      = "crdsv1beta1.yaml"
)

func importSecretNsN(managedCluster *clusterv1.ManagedCluster) (types.NamespacedName, error) {
	if managedCluster == nil {
		return types.NamespacedName{}, fmt.Errorf("managedCluster is nil")
	} else if managedCluster.Name == "" {
		return types.NamespacedName{}, fmt.Errorf("managedCluster.Name is blank")
	}
	return types.NamespacedName{
		Name:      managedCluster.Name + importSecretNamePostfix,
		Namespace: managedCluster.Name,
	}, nil
}

func newImportSecret(
	managedCluster *clusterv1.ManagedCluster,
	crds map[string][]*unstructured.Unstructured,
	yamls []*unstructured.Unstructured,
) (*corev1.Secret, error) {
	importYAML := new(bytes.Buffer)
	crdsV1YAML := new(bytes.Buffer)
	crdsV1beta1YAML := new(bytes.Buffer)

	secretNsN, err := importSecretNsN(managedCluster)
	if err != nil {
		return nil, err
	}

	for _, crd := range crds["v1"] {
		b, err := templateprocessor.ToYAMLUnstructured(crd)
		if err != nil {
			return nil, err
		}
		crdsV1YAML.WriteString(fmt.Sprintf("\n---\n%s", string(b)))
	}

	for _, crd := range crds["v1beta1"] {
		b, err := templateprocessor.ToYAMLUnstructured(crd)
		if err != nil {
			return nil, err
		}
		crdsV1beta1YAML.WriteString(fmt.Sprintf("\n---\n%s", string(b)))
	}

	for _, y := range yamls {
		b, err := templateprocessor.ToYAMLUnstructured(y)
		if err != nil {
			return nil, err
		}
		importYAML.WriteString(fmt.Sprintf("\n---\n%s", string(b)))
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretNsN.Name,
			Namespace: secretNsN.Namespace,
		},
		Data: map[string][]byte{
			importYAMLKey:      importYAML.Bytes(),
			crdsYAMLKey:        crdsV1YAML.Bytes(),
			crdsV1YAMLKey:      crdsV1YAML.Bytes(),
			crdsV1beta1YAMLKey: crdsV1beta1YAML.Bytes(),
		},
	}

	return secret, nil
}

func createOrUpdateImportSecret(
	client client.Client,
	scheme *runtime.Scheme,
	managedCluster *clusterv1.ManagedCluster,
	crds map[string][]*unstructured.Unstructured,
	yamls []*unstructured.Unstructured,
) (*corev1.Secret, error) {
	secret, err := newImportSecret(managedCluster, crds, yamls)
	if err != nil {
		return nil, err
	}
	if err := controllerutil.SetControllerReference(managedCluster, secret, scheme); err != nil {
		return nil, err
	}

	log.Info("Create/update of Import secret", "name", secret.Name, "namespace", secret.Namespace)
	oldImportSecret := &corev1.Secret{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, oldImportSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			err := client.Create(context.TODO(), secret)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		if !bytes.Equal(oldImportSecret.Data[importYAMLKey], secret.Data[importYAMLKey]) ||
			!bytes.Equal(oldImportSecret.Data[crdsYAMLKey], secret.Data[crdsYAMLKey]) ||
			!bytes.Equal(oldImportSecret.Data[crdsV1beta1YAMLKey], secret.Data[crdsV1beta1YAMLKey]) ||
			!bytes.Equal(oldImportSecret.Data[crdsV1YAMLKey], secret.Data[crdsV1YAMLKey]) {
			oldImportSecret.Data = secret.Data
			if err := client.Update(context.TODO(), oldImportSecret); err != nil {
				return nil, err
			}
		}
	}

	return secret, nil
}
