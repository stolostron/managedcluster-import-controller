// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterimport ...
package clusterimport

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

// KlusterletNamespace is the namespace that klusterlet operator and its components will be deployed in
const KlusterletNamespace = "klusterlet"

// KlusterletName is the name of the klusterlet.agent.open-cluster-management.io resource
const KlusterletName = "klusterlet"

// KlusterletOperatorName is the name of the klusterlet operator
const KlusterletOperatorName = "klusterlet-operator"

// BootstrapSecretName is the name of the bootstrap secret
const BootstrapSecretName = "klusterlet-bootstrap"

// KlusterletOperatorImageName is the name of the klusterlet operator image
const KlusterletOperatorImageName = "klusterlet-operator"

// ImageTagPostfixKey is the name of the environment variable of klusterlet operator image tag's postfix
const ImageTagPostfixKey = "IMAGE_TAG_POSTFIX"

// KlusterletOperatorImageKey is the path of the klusterlet operator image
const KlusterletOperatorImageKey = "KLUSTERLET_OPERATOR_IMAGE"

var log = logf.Log.WithName("clusterimport")

// GenerateKlusterletCRD returns an array of runtime.Object, which contains only the klusterlet crd
func GenerateKlusterletCRD() ([]runtime.Object, error) {
	crd, err := newKlusterletCRD()
	if err != nil {
		return nil, err
	}
	return []runtime.Object{
		crd,
	}, nil
}

// GenerateImportObjects generate all the object in the manifest use for installing klusterlet on managed cluster
func GenerateImportObjects(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) ([]runtime.Object, error) {
	importObjects, err := generateCommonImportObjects()
	if err != nil {
		return nil, err
	}

	bootstrapSecret, err := newBootstrapSecret(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	if bootstrapSecret == nil {
		return nil, fmt.Errorf("bootstrapSecret is nil")
	}

	imagePullSecret, err := newKlusterletImagePullSecret(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	if imagePullSecret != nil {
		importObjects = append(importObjects, imagePullSecret)
	}

	return append(
		importObjects,
		bootstrapSecret,
		newOperatorDeployment(klusterletConfig),
		newKlusterletResource(klusterletConfig),
	), nil
}

// generateCommonObjects generates the objects in the manifest stays constant used in the SelectorSyncSet
func generateCommonImportObjects() ([]runtime.Object, error) {
	crd, err := newKlusterletCRD()
	if err != nil {
		return nil, err
	}

	return []runtime.Object{
		crd,
		newKlusterletNamespace(),
		newOperatorServiceAccount(),
		newOperatorClusterRoleBinding(),
	}, nil
}

func newKlusterletCRD() (*apiextensionv1beta1.CustomResourceDefinition, error) {
	fileName := os.Getenv("KLUSTERLET_CRD_FILE")
	if fileName == "" {
		return nil, fmt.Errorf("ENV KLUSTERLET_CRD_FILE undefined")
	}

	data, err := ioutil.ReadFile(fileName) // #nosec G304
	if err != nil {
		log.Error(err, "fail to CRD ReadFile", "filename", fileName)
		return nil, err
	}

	crd := &apiextensionv1beta1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(data, crd); err != nil {
		log.Error(err, "fail to Unmarshal CRD", "content", data)
		return nil, err
	}
	// make sure it won't generate nil in the final yaml
	if crd.Status.Conditions == nil {
		crd.Status.Conditions = []apiextensionv1beta1.CustomResourceDefinitionCondition{}
	}
	if crd.Status.StoredVersions == nil {
		crd.Status.StoredVersions = []string{}
	}

	return crd, nil
}

func newKlusterletNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: KlusterletNamespace,
		},
	}
}

func newOperatorServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      KlusterletOperatorName,
			Namespace: KlusterletNamespace,
		},
	}
}

func newOperatorClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: KlusterletOperatorName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      KlusterletOperatorName,
				Namespace: KlusterletNamespace,
			},
		},
	}
}

func newBootstrapSecret(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.Secret, error) {
	saToken, err := getBootstrapToken(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	kubeAPIServer, err := getKubeAPIServerAddress(client)
	if err != nil {
		return nil, err
	}

	bootstrapConfig := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                kubeAPIServer,
			InsecureSkipTLSVerify: true,
		}},
		// Define auth based on the obtained client cert.
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			Token: string(saToken),
		}},
		// Define a context that connects the auth info and cluster, and set it as the default
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	bootstrapConfigData, err := runtime.Encode(clientcmdlatest.Codec, &bootstrapConfig)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      BootstrapSecretName,
			Namespace: KlusterletNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": bootstrapConfigData,
		},
	}, nil
}

func newKlusterletImagePullSecret(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.Secret, error) {
	secret, err := getImagePullSecret(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	if secret == nil {
		return nil, nil
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: KlusterletNamespace,
		},
		Data: secret.Data,
		Type: secret.Type,
	}, nil
}

// GetKlusterletOperatorImage returns klusterlet-operator image, imageTagPostfix, and a boolean indicates use SHA or not.
// If `IMAGE_TAG_POSTFIX` env var is set, will return false for the boolean of useSHA.
func GetKlusterletOperatorImage(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (imageName string, imageTagPostfix string, useSHA bool) {
	imageTagPostfix = os.Getenv(ImageTagPostfixKey)
	klusterletOperatorImage := os.Getenv(KlusterletOperatorImageKey)
	useSHA = imageTagPostfix == ""
	if klusterletConfig.Spec.ImageRegistry == "" {
		return klusterletOperatorImage, imageTagPostfix, useSHA
	}

	imageName = klusterletConfig.Spec.ImageRegistry +
		"/" + KlusterletOperatorImageName +
		klusterletConfig.Spec.ImageNamePostfix +
		":" + klusterletConfig.Spec.Version

	if imageTagPostfix != "" {
		imageName += imageTagPostfix
		return imageName, imageTagPostfix, useSHA
	}

	if klusterletOperatorImage != "" {
		imageName = klusterletOperatorImage
	}

	return imageName, "", useSHA
}

func newOperatorDeployment(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) *appsv1.Deployment {
	imageName, imageTagPostfix, useSHA := GetKlusterletOperatorImage(klusterletConfig)
	imagePullSecrets := []corev1.LocalObjectReference{}
	if len(klusterletConfig.Spec.ImagePullSecret) > 0 {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: klusterletConfig.Spec.ImagePullSecret})
	}

	useSHAManifestEnv := "false"
	if useSHA {
		useSHAManifestEnv = "true"
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      KlusterletOperatorName,
			Namespace: KlusterletNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": KlusterletOperatorName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": KlusterletOperatorName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: KlusterletOperatorName,
					Containers: []corev1.Container{
						{
							Name:            KlusterletOperatorName,
							Image:           imageName,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "WATCH_NAMESPACE",
									Value: "",
								},
								{
									Name:  "OPERATOR_NAME",
									Value: KlusterletOperatorName,
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name:  ImageTagPostfixKey,
									Value: imageTagPostfix,
								},
								{
									Name:  "USE_SHA_MANIFEST",
									Value: useSHAManifestEnv,
								},
							},
						},
					},
					ImagePullSecrets: imagePullSecrets,
				},
			},
		},
	}
}

func newKlusterletResource(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) *klusterletv1beta1.Klusterlet {
	return &klusterletv1beta1.Klusterlet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: klusterletv1beta1.SchemeGroupVersion.String(),
			Kind:       "Klusterlet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      KlusterletName,
			Namespace: KlusterletNamespace,
		},
		Spec: klusterletConfig.Spec,
	}
}
