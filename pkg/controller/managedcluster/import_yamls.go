// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// Package managedcluster ...
package managedcluster

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"k8s.io/klog"

	"net/url"

	corev1 "k8s.io/api/core/v1"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/applier/pkg/templateprocessor"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/bindata"
	"github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/imageregistry/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	envVarNotDefined                    = "environment variable %s not defined"
	managedClusterImagePullSecretName   = "open-cluster-management-image-pull-credentials"

	clusterImageRegistryLabel = "open-cluster-management.io/image-registry"
)

func generateImportYAMLs(
	client client.Client,
	managedCluster *clusterv1.ManagedCluster,
	excluded []string,
) (crds map[string][]*unstructured.Unstructured, yamls []*unstructured.Unstructured, err error) {

	klog.V(4).Info("Create templateProcessor")
	tp, err := templateprocessor.NewTemplateProcessor(bindata.NewBindataReader(), &templateprocessor.Options{})
	if err != nil {
		return nil, nil, err
	}

	crds = make(map[string][]*unstructured.Unstructured)
	klog.V(4).Info("TemplateResources klusterlet/crds/v1beta1/")
	crds["v1beta1"], err = tp.TemplateResourcesInPathUnstructured("klusterlet/crds/v1beta1/", nil, true, nil)
	if err != nil {
		return nil, nil, err
	}

	klog.V(4).Info("TemplateResources klusterlet/crds/v1/")
	crds["v1"], err = tp.TemplateResourcesInPathUnstructured("klusterlet/crds/v1/", nil, true, nil)
	if err != nil {
		return nil, nil, err
	}

	bootStrapSecret, err := getBootstrapSecret(client, managedCluster)
	if err != nil {
		return nil, nil, err
	}

	klog.V(4).Infof("createKubeconfigData for bootsrapSecret %s", bootStrapSecret.Name)
	bootstrapKubeconfigData, err := createKubeconfigData(client, bootStrapSecret)
	if err != nil {
		return nil, nil, err
	}

	pullSecretNamespace := ""
	pullSecret := ""
	registry := ""
	imageRegistry, err := getImageRegistry(client,
		managedCluster.Labels[clusterImageRegistryLabel])
	if err != nil {
		klog.Errorf("failed to get custom registry and pull secret %v", err)
	}

	if imageRegistry != nil {
		pullSecretNamespace = imageRegistry.Namespace
		pullSecret = imageRegistry.Spec.PullSecret.Name
		registry = imageRegistry.Spec.Registry
	}

	useImagePullSecret := false
	imagePullSecretDataBase64 := ""
	imagePullSecret, err := getImagePullSecret(pullSecretNamespace, pullSecret, client)
	if err != nil {
		return nil, nil, err
	}
	if imagePullSecret != nil && len(imagePullSecret.Data[".dockerconfigjson"]) != 0 {
		imagePullSecretDataBase64 = base64.StdEncoding.EncodeToString(imagePullSecret.Data[".dockerconfigjson"])
		useImagePullSecret = true
	}

	registrationOperatorImageName, err := getImage(registry, registrationOperatorImageEnvVarName)
	if err != nil {
		return nil, nil, err
	}

	registrationImageName, err := getImage(registry, registrationImageEnvVarName)
	if err != nil {
		return nil, nil, err
	}

	workImageName, err := getImage(registry, workImageEnvVarName)
	if err != nil {
		return nil, nil, err
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
	klusterletYAMLs, err := tp.TemplateResourcesInPathUnstructured(
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

func getImagePullSecret(pullSecretNamespace, pullSecret string, client client.Client) (*corev1.Secret, error) {
	var secretNamespace, secretName string
	var err error

	if pullSecretNamespace != "" && pullSecret != "" {
		secretNamespace = pullSecretNamespace
		secretName = pullSecret
	} else {
		secretName = os.Getenv("DEFAULT_IMAGE_PULL_SECRET")
		secretNamespace = os.Getenv("POD_NAMESPACE")
		if secretName == "" || secretNamespace == "" {
			klog.Errorf("the default image pull secret %v/%v in invalid", secretNamespace, secretName)
			return nil, fmt.Errorf("the default image pull secret %v/%v in invalid", secretNamespace, secretName)
		}
	}

	secret := &corev1.Secret{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		klog.Errorf("failed to get image pull secret %v/%v", secretNamespace, secretName)
		return nil, err
	}
	return secret, nil
}

// getImage returns then image of components
// if customRegistry is empty, return the default image
// if customRegistry is not empty, replace the registry address of default image.
func getImage(customRegistry, envName string) (string, error) {
	defaultImage := os.Getenv(envName)
	if defaultImage == "" {
		klog.Errorf("the default image %v is invalid", envName)
		return "", fmt.Errorf(envVarNotDefined, registrationImageEnvVarName)
	}
	if customRegistry == "" {
		return defaultImage, nil
	}

	// image format: registryAddress/imageName@SHA256 or registryAddress/imageName:tag
	// replace the registryAddress of image with customRegistry
	customRegistry = strings.TrimSuffix(customRegistry, "/")
	imageSegments := strings.Split(defaultImage, "/")
	customImage := customRegistry + "/" + imageSegments[len(imageSegments)-1]

	return customImage, nil
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

// getImageRegistry gets imageRegistry.
// imageRegistryLabelValue format is namespace.imageRegistry
func getImageRegistry(client client.Client,
	imageRegistryLabelValue string) (*v1alpha1.ManagedClusterImageRegistry, error) {
	if imageRegistryLabelValue == "" {
		return nil, nil
	}

	segments := strings.Split(imageRegistryLabelValue, ".")
	if len(segments) != 2 {
		klog.Errorf("invalid format of image registry label value %v", imageRegistryLabelValue)
		return nil, fmt.Errorf("invalid format of image registry label value %v", imageRegistryLabelValue)
	}
	namespace := segments[0]
	imageRegistryName := segments[1]
	imageRegistry := &v1alpha1.ManagedClusterImageRegistry{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: imageRegistryName, Namespace: namespace}, imageRegistry)
	if err != nil {
		klog.Errorf("failed to get imageregistry %v/%v", namespace, imageRegistryName)
		return nil, err
	}
	return imageRegistry, nil
}
