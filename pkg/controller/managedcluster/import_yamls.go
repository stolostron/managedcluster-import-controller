// Copyright (c) 2020 Red Hat, Inc.

//Package managedcluster ...
package managedcluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"net/url"

	corev1 "k8s.io/api/core/v1"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/library-go/pkg/templateprocessor"
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
	excluded []string,
) (yamls [][]byte, crds [][]byte, err error) {

	tp, err := templateprocessor.NewTemplateProcessor(bindata.NewBindataReader(), &templateprocessor.Options{})
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

	useImagePullSecret := false
	imagePullSecretDataBase64 := ""
	imagePullSecret, err := getImagePullSecret(client)
	if err != nil {
		return nil, nil, err
	}
	if imagePullSecret != nil && len(imagePullSecret.Data[".dockerconfigjson"]) != 0 {
		imagePullSecretDataBase64 = base64.StdEncoding.EncodeToString(imagePullSecret.Data[".dockerconfigjson"])
		useImagePullSecret = true
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
		UseImagePullSecret        bool
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
		UseImagePullSecret:        useImagePullSecret,
		ImagePullSecretName:       managedClusterImagePullSecretName,
		ImagePullSecretData:       imagePullSecretDataBase64,
		ImagePullSecretType:       corev1.SecretTypeDockerConfigJson,
		RegistrationOperatorImage: registrationOperatorImageName,
		RegistrationImageName:     registrationImageName,
		WorkImageName:             workImageName,
	}

	tp, err = templateprocessor.NewTemplateProcessor(bindata.NewBindataReader(), &templateprocessor.Options{})
	if err != nil {
		return nil, nil, err
	}
	if !useImagePullSecret {
		excluded = append(excluded, "klusterlet/image_pull_secret.yaml")
	}
	klusterletYAMLs, err := tp.TemplateAssetsInPathYaml(
		"klusterlet",
		excluded,
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
	if os.Getenv("DEFAULT_IMAGE_PULL_SECRET") == "" {
		return nil, nil
	}
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
	var certData []byte
	if u, err := url.Parse(kubeAPIServer); err == nil {
		apiServerCertSecretName, err := getKubeAPIServerSecretName(client, u.Hostname())
		if err != nil {
			return nil, err
		}
		if len(apiServerCertSecretName) > 0 {
			apiServerCert, err := getKubeAPIServerCertificate(client, apiServerCertSecretName)
			if err != nil {
				return nil, err
			}
			certData = apiServerCert
		}
	}
	if _, ok := bootStrapSecret.Data["ca.crt"]; ok && certData == nil {
		certData = bootStrapSecret.Data["ca.crt"]
	}
	if len(certData) == 0 {
		insecureSkipTLSVerify = true
	}

	bootstrapConfig := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   kubeAPIServer,
			InsecureSkipTLSVerify:    insecureSkipTLSVerify,
			CertificateAuthorityData: certData,
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
