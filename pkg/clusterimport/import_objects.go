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

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

// EndpointNamespace is the namespace that multicluster-endpoint operator and its components will be deployed in
const EndpointNamespace = "multicluster-endpoint"

// EndpointName is the name of the endpoint.multicloud.ibm.com resource
const EndpointName = "endpoint"

// EndpointOperatorName is the name of the multicluster endpoint operator
const EndpointOperatorName = "endpoint-operator"

// BootstrapSecretName is the name of the bootstrap secret
const BootstrapSecretName = "klusterlet-bootstrap"

// EndpointOperatorImageName is the name of the Endpoinmulticluster-endpoint operator image
const EndpointOperatorImageName = "endpoint-operator"

// ImageTagPostfixKey is the name of the environment variable of endpoint operator image tag's postfix
const ImageTagPostfixKey = "IMAGE_TAG_POSTFIX"

// EndpointOperatorImageKey is the path of the endpoint operator image
const EndpointOperatorImageKey = "ENDPOINT_OPERATOR_IMAGE"

var log = logf.Log.WithName("clusterimport")

// GenerateEndpointCRD returns an array of runtime.Object, which contains only the endpoint crd
func GenerateEndpointCRD() ([]runtime.Object, error) {
	crd, err := newEndpointCRD()
	if err != nil {
		return nil, err
	}
	return []runtime.Object{
		crd,
	}, nil
}

// GenerateImportObjects generate all the object in the manifest use for installing multicluster-endpoint on managed cluster
func GenerateImportObjects(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) ([]runtime.Object, error) {
	importObjects, err := generateCommonImportObjects()
	if err != nil {
		return nil, err
	}

	bootstrapSecret, err := newBootstrapSecret(client, endpointConfig)
	if err != nil {
		return nil, err
	}

	if bootstrapSecret == nil {
		return nil, fmt.Errorf("bootstrapSecret is nil")
	}

	imagePullSecret, err := newEndpointImagePullSecret(client, endpointConfig)
	if err != nil {
		return nil, err
	}

	if imagePullSecret != nil {
		importObjects = append(importObjects, imagePullSecret)
	}

	return append(
		importObjects,
		bootstrapSecret,
		newOperatorDeployment(endpointConfig),
		newEndpointResource(endpointConfig),
	), nil
}

// generateCommonObjects generates the objects in the manifest stays constant used in the SelectorSyncSet
func generateCommonImportObjects() ([]runtime.Object, error) {
	crd, err := newEndpointCRD()
	if err != nil {
		return nil, err
	}

	return []runtime.Object{
		crd,
		newEndpointNamespace(),
		newOperatorServiceAccount(),
		newOperatorClusterRoleBinding(),
	}, nil
}

func newEndpointCRD() (*apiextensionv1beta1.CustomResourceDefinition, error) {
	fileName := os.Getenv("ENDPOINT_CRD_FILE")
	if fileName == "" {
		return nil, fmt.Errorf("ENV ENDPOINT_CRD_FILE undefine")
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

func newEndpointNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: EndpointNamespace,
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
			Name:      EndpointOperatorName,
			Namespace: EndpointNamespace,
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
			Name: EndpointOperatorName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      EndpointOperatorName,
				Namespace: EndpointNamespace,
			},
		},
	}
}

func newBootstrapSecret(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	saToken, err := getBootstrapToken(client, endpointConfig)
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
			Namespace: EndpointNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": bootstrapConfigData,
		},
	}, nil
}

func newEndpointImagePullSecret(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	secret, err := getImagePullSecret(client, endpointConfig)
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
			Namespace: EndpointNamespace,
		},
		Data: secret.Data,
		Type: secret.Type,
	}, nil
}

// GetEndpointOperatorImage returns endpoint-operator image, imageTagPostfix, and a boolean indicates use SHA or not.
// If `IMAGE_TAG_POSTFIX` env var is set, will return false for the boolean of useSHA.
func GetEndpointOperatorImage(endpointConfig *multicloudv1alpha1.EndpointConfig) (imageName string, imageTagPostfix string, useSHA bool) {
	imageTagPostfix = os.Getenv(ImageTagPostfixKey)
	endpointOperatorImage := os.Getenv(EndpointOperatorImageKey)
	useSHA = imageTagPostfix == ""
	if endpointConfig.Spec.ImageRegistry == "" {
		return endpointOperatorImage, imageTagPostfix, useSHA
	}

	imageName = endpointConfig.Spec.ImageRegistry +
		"/" + EndpointOperatorImageName +
		endpointConfig.Spec.ImageNamePostfix +
		":" + endpointConfig.Spec.Version

	if imageTagPostfix != "" {
		imageName += imageTagPostfix
		return imageName, imageTagPostfix, useSHA
	}

	if endpointOperatorImage != "" {
		imageName = endpointOperatorImage
	}

	return imageName, "", useSHA
}

func newOperatorDeployment(endpointConfig *multicloudv1alpha1.EndpointConfig) *appsv1.Deployment {
	imageName, imageTagPostfix, useSHA := GetEndpointOperatorImage(endpointConfig)
	imagePullSecrets := []corev1.LocalObjectReference{}
	if len(endpointConfig.Spec.ImagePullSecret) > 0 {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: endpointConfig.Spec.ImagePullSecret})
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
			Name:      EndpointOperatorName,
			Namespace: EndpointNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": EndpointOperatorName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": EndpointOperatorName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: EndpointOperatorName,
					Containers: []corev1.Container{
						{
							Name:            EndpointOperatorName,
							Image:           imageName,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "WATCH_NAMESPACE",
									Value: "",
								},
								{
									Name:  "OPERATOR_NAME",
									Value: EndpointOperatorName,
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

func newEndpointResource(endpointConfig *multicloudv1alpha1.EndpointConfig) *multicloudv1beta1.Endpoint {
	return &multicloudv1beta1.Endpoint{
		TypeMeta: metav1.TypeMeta{
			APIVersion: multicloudv1beta1.SchemeGroupVersion.String(),
			Kind:       "Endpoint",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      EndpointName,
			Namespace: EndpointNamespace,
		},
		Spec: endpointConfig.Spec,
	}
}
