// Copyright (c) 2020 Red Hat, Inc.

//Package managedcluster ...
package managedcluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/library-go/pkg/applier"
	"github.com/open-cluster-management/rcm-controller/pkg/bindata"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	registrationOperatorImageEnvVarName = "REGISTRATION_OPERATOR_IMAGE"
	registrationImageEnvVarName         = "REGISTRATION_IMAGE"
	workImageEnvVarName                 = "WORK_IMAGE"
	klusterletNamespace                 = "open-cluster-management-agent"
	envVarNotDefined                    = "Environment variable %s not defined"
	managedClusterImagePullSecretName   = "open-cluster-management-image-pull-credentials"
)

func generateImportYAMLs(
	client client.Client,
	managedCluster *clusterv1.ManagedCluster,
) (yamls [][]byte, crds [][]byte, err error) {

	tp, err := applier.NewTemplateProcessor(bindata.NewBindataReader(), &applier.Options{})
	if err != nil {
		return nil, nil, err
	}
	crds, err = tp.Assets("klusterlet/crds", nil, true)
	if err != nil {
		return nil, nil, err
	}

	bootStrapSecret, err := getBootstrapSecret(client, managedCluster)
	if err != nil {
		return nil, nil, err
	}

	bootstrapKubeconfigData, err := createKubeconfigData(client, bootStrapSecret)
	if err != nil {
		return nil, nil, err
	}

	imagePullSecret, err := getImagePullSecret(client)
	if err != nil {
		return nil, nil, err
	}

	registrationOperatorImageName := os.Getenv(registrationOperatorImageEnvVarName)
	if registrationOperatorImageName == "" {
		return nil, nil, fmt.Errorf(envVarNotDefined, registrationOperatorImageEnvVarName)
	}

	registrationImageName := os.Getenv(registrationImageEnvVarName)
	if registrationImageName == "" {
		return nil, nil, fmt.Errorf(envVarNotDefined, registrationImageEnvVarName)
	}

	workImageName := os.Getenv(workImageEnvVarName)
	if workImageName == "" {
		return nil, nil, fmt.Errorf(envVarNotDefined, workImageEnvVarName)
	}

	config := struct {
		KlusterletNamespace       string
		ManagedClusterNamespace   string
		BootstrapKubeconfig       string
		ImagePullSecretName       string
		ImagePullSecretData       string
		ImagePullSecretType       corev1.SecretType
		RegistrationOperatorImage string
		RegistrationImageName     string
		WorkImageName             string
	}{
		ManagedClusterNamespace:   managedCluster.Name,
		KlusterletNamespace:       klusterletNamespace,
		BootstrapKubeconfig:       base64.StdEncoding.EncodeToString(bootstrapKubeconfigData),
		ImagePullSecretName:       managedClusterImagePullSecretName,
		ImagePullSecretData:       base64.StdEncoding.EncodeToString(imagePullSecret.Data[".dockerconfigjson"]),
		ImagePullSecretType:       imagePullSecret.Type,
		RegistrationOperatorImage: registrationOperatorImageName,
		RegistrationImageName:     registrationImageName,
		WorkImageName:             workImageName,
	}

	tp, err = applier.NewTemplateProcessor(bindata.NewBindataReader(), &applier.Options{})
	if err != nil {
		return nil, nil, err
	}
	klusterletYAMLs, err := tp.TemplateAssetsInPathYaml(
		"klusterlet",
		nil,
		false,
		config,
	)

	if err != nil {
		return nil, nil, err
	}

	yamls = append(yamls, klusterletYAMLs...)

	return crds, yamls, nil
}

func getImagePullSecret(client client.Client) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
		Namespace: os.Getenv("POD_NAMESPACE"),
	}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func createKubeconfigData(client client.Client, bootStrapSecret *corev1.Secret) ([]byte, error) {
	saToken := bootStrapSecret.Data["token"]

	kubeAPIServer, err := getKubeAPIServerAddress(client)
	if err != nil {
		return nil, err
	}

	insecureSkipTLSVerify := false
	if _, ok := bootStrapSecret.Data["ca.crt"]; !ok {
		insecureSkipTLSVerify = true
	}

	bootstrapConfig := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   kubeAPIServer,
			InsecureSkipTLSVerify:    insecureSkipTLSVerify,
			CertificateAuthorityData: bootStrapSecret.Data["ca.crt"],
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

	return runtime.Encode(clientcmdlatest.Codec, &bootstrapConfig)

}
