// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"

	"net/url"

	corev1 "k8s.io/api/core/v1"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/library-go/pkg/templateprocessor"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/bindata"
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

// getValidCertificatesFromURL dial to serverURL and get certificates
// only will return certificates signed by trusted ca and verified (with verifyOptions)
// if certificates are all signed by unauthorized party, will return nil
// rootCAs is for tls handshake verification
func getValidCertificatesFromURL(serverURL string, rootCAs *x509.CertPool) ([]*x509.Certificate, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		log.Error(err, "failed to parse url: "+serverURL)
		return nil, err
	}
	log.Info("getting certificate of " + u.Hostname() + ":" + u.Port())
	conf := &tls.Config{
		// server should support tls1.2
		MinVersion: tls.VersionTLS12,
		ServerName: u.Hostname(),
	}
	if rootCAs != nil {
		conf.RootCAs = rootCAs
	}

	conn, err := tls.Dial("tcp", u.Hostname()+":"+u.Port(), conf)

	if err != nil {
		log.Error(err, "failed to dial "+serverURL)
		// ignore certificate signed by unknown authority error
		if _, ok := err.(x509.UnknownAuthorityError); ok {
			return nil, nil
		}
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	retCerts := []*x509.Certificate{}
	opt := x509.VerifyOptions{Roots: rootCAs}
	// check certificates
	for _, cert := range certs {
		if _, err := cert.Verify(opt); err == nil {
			log.V(2).Info("Adding a valid certificate")
			retCerts = append(retCerts, cert)
		} else {
			log.V(2).Info("Skipping an invalid certificate")
		}
	}
	return retCerts, nil
}

func createKubeconfigData(client client.Client, bootStrapSecret *corev1.Secret) ([]byte, error) {
	saToken := bootStrapSecret.Data["token"]

	kubeAPIServer, err := getKubeAPIServerAddress(client)
	if err != nil {
		return nil, err
	}

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
	if len(certData) == 0 {
		// fallback to service account token ca.crt
		if _, ok := bootStrapSecret.Data["ca.crt"]; ok {
			certData = bootStrapSecret.Data["ca.crt"]
		}
		// check if it's roks
		// if it's ocp && it's on ibm cloud, we treat it as roks
		isROKS, err := checkIsIBMCloud(client)
		if err != nil {
			return nil, err
		}
		if isROKS {
			// ROKS should have a certificate that is signed by trusted CA
			if certs, err := getValidCertificatesFromURL(kubeAPIServer, nil); err != nil {
				// should retry if failed to connect to apiserver
				log.Error(err, fmt.Sprintf("failed to connect to %s", kubeAPIServer))
				return nil, err
			} else if len(certs) > 0 {
				// simply don't give any certs as the apiserver is using certs signed by known CAs
				certData = nil
			} else {
				log.Info("No additional valid certificate found for APIserver. Skipping.")
			}
		}
	}

	bootstrapConfig := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   kubeAPIServer,
			InsecureSkipTLSVerify:    false,
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
