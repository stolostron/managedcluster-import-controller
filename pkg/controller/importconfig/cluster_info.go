// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	imgregistryv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/imageregistry/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getBootstrapSecret looks for the bootstrap secret from bootstrap sa
func getBootstrapSecret(client client.Client, managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error) {
	saName := fmt.Sprintf("%s-%s", managedCluster.Name, bootstrapSASuffix)

	sa := &corev1.ServiceAccount{}
	if err := client.Get(context.TODO(), types.NamespacedName{Namespace: managedCluster.Name, Name: saName}, sa); err != nil {
		return nil, err
	}

	var secret *corev1.Secret
	for _, objectRef := range sa.Secrets {
		if objectRef.Namespace != "" && objectRef.Namespace != managedCluster.Name {
			continue
		}
		prefix := saName
		if len(prefix) > 63 {
			prefix = prefix[:37]
		}
		if strings.HasPrefix(objectRef.Name, prefix) {
			secret = &corev1.Secret{}
			err := client.Get(context.TODO(), types.NamespacedName{Name: objectRef.Name, Namespace: managedCluster.Name}, secret)
			if err != nil {
				continue
			}
			if secret.Type == corev1.SecretTypeServiceAccountToken {
				break
			}
		}
	}

	if secret == nil {
		return nil, fmt.Errorf("secret with prefix %s and type %s not found in service account %s/%s",
			saName,
			corev1.SecretTypeServiceAccountToken,
			managedCluster.Name,
			saName,
		)
	}

	return secret, nil
}

// getKubeAPIServerAddress get the kube-apiserver URL from ocp infrastructure
func getKubeAPIServerAddress(client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// getKubeAPIServerSecretName iterate through all named certificates from apiserver
// returns the first one which has a name matches the given dnsName
func getKubeAPIServerSecretName(client client.Client, dnsName string) (string, error) {
	apiserver := &ocinfrav1.APIServer{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, apiserver); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Failed to get ocp apiserver cluster")
			return "", nil
		}
		return "", err
	}

	// iterate through all namedcertificates
	for _, namedCert := range apiserver.Spec.ServingCerts.NamedCertificates {
		for _, name := range namedCert.Names {
			if strings.EqualFold(name, dnsName) {
				return namedCert.ServingCertificate.Name, nil
			}
		}
	}
	return "", nil
}

// checkIsIBMCloud detects if the current cloud vendor is ibm or not
// we know we are on OCP already, so if it's also ibm cloud, it's roks
func checkIsIBMCloud(client client.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	if err := client.List(context.TODO(), nodes); err != nil {
		return false, err
	}

	if len(nodes.Items) == 0 {
		log.Info("Failed to get nodes")
		return false, nil
	}

	providerID := nodes.Items[0].Spec.ProviderID
	if strings.Contains(providerID, "ibm") {
		return true, nil
	}

	return false, nil
}

// getKubeAPIServerCertificate looks for secret in openshift-config namespace, and returns tls.crt
func getKubeAPIServerCertificate(client client.Client, secretName string) ([]byte, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: "openshift-config"}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Failed to get secret", fmt.Sprintf("openshift-config/%s", secretName))
			return nil, nil
		}

		return nil, err
	}

	if secret.Type != corev1.SecretTypeTLS {
		return nil, fmt.Errorf("secret openshift-config/%s should have type=kubernetes.io/tls", secretName)
	}

	res, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf("failed to find data[tls.crt] in secret openshift-config/%s", secretName)
	}

	return res, nil
}

// getImagePullSecret get image pull secret from env
func getImagePullSecret(client client.Client, managedCluster *clusterv1.ManagedCluster) (string, *corev1.Secret, error) {
	imageRegistry, err := getImageRegistry(client, managedCluster)
	if err != nil {
		return "", nil, err
	}

	secret := &corev1.Secret{}
	if imageRegistry != nil {
		secretKey := types.NamespacedName{Namespace: imageRegistry.Namespace, Name: imageRegistry.Spec.PullSecret.Name}
		if err := client.Get(context.TODO(), secretKey, secret); err != nil {
			return "", nil, err
		}

		return imageRegistry.Spec.Registry, secret, nil
	}

	defaultSecretName := os.Getenv(defaultImagePullSecretEnvVarName)
	if defaultSecretName == "" {
		log.Info(fmt.Sprintf("Cannot find the default image pull secret of the managed cluster %s from %s",
			managedCluster.Name, defaultImagePullSecretEnvVarName))
		return "", nil, nil
	}

	secretKey := types.NamespacedName{Namespace: os.Getenv(constants.PodNamespaceEnvVarName), Name: defaultSecretName}
	if err := client.Get(context.TODO(), secretKey, secret); err != nil {
		return "", nil, err
	}
	return "", secret, nil
}

// getValidCertificatesFromURL dial to serverURL and get certificates
// only will return certificates signed by trusted ca and verified (with verifyOptions)
// if certificates are all signed by unauthorized party, will return nil
// rootCAs is for tls handshake verification
func getValidCertificatesFromURL(serverURL string, rootCAs *x509.CertPool) ([]*x509.Certificate, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}

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
			retCerts = append(retCerts, cert)
		}
	}
	return retCerts, nil
}

// create kubeconfig from bootstrap secret
func createKubeconfigData(client client.Client, bootStrapSecret *corev1.Secret) ([]byte, error) {
	saToken := bootStrapSecret.Data["token"]
	if len(saToken) == 0 {
		return nil, fmt.Errorf("no token value found in the boot strap secret")
	}

	kubeAPIServer, err := getKubeAPIServerAddress(client)
	if err != nil {
		return nil, err
	}

	var certData []byte
	if u, err := url.Parse(kubeAPIServer); err == nil {
		// get the ca cert from ocp apiserver firstly
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
		if v := bootStrapSecret.Data["ca.crt"]; len(v) > 0 {
			certData = v
		} else {
			log.Info("No ca.crt found in the boot strap secret", "secret name", bootStrapSecret.Name)
		}

		// if it's ocp && it's on ibm cloud, we treat it as roks
		isROKS, err := checkIsIBMCloud(client)
		if err != nil {
			return nil, err
		}

		if isROKS {
			// ROKS should have a certificate that is signed by trusted CA
			if certs, err := getValidCertificatesFromURL(kubeAPIServer, nil); err != nil {
				// should retry if failed to connect to apiserver
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
func getImageRegistry(
	client client.Client, managedCluster *clusterv1.ManagedCluster) (*imgregistryv1alpha1.ManagedClusterImageRegistry, error) {
	imageRegistryLabelValue, ok := managedCluster.Labels[clusterImageRegistryLabel]
	if !ok || imageRegistryLabelValue == "" {
		return nil, nil
	}

	segments := strings.Split(imageRegistryLabelValue, ".")
	if len(segments) != 2 {
		return nil, fmt.Errorf("invalid format of image registry label value %s", imageRegistryLabelValue)
	}

	namespace := segments[0]
	imageRegistryName := segments[1]
	imageRegistry := &imgregistryv1alpha1.ManagedClusterImageRegistry{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: imageRegistryName}, imageRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to get imageregistry %s/%s: %v", namespace, imageRegistryName, err)
	}
	return imageRegistry, nil
}

// getImage returns then image of components
// if customRegistry is empty, return the default image
// if customRegistry is not empty, replace the registry address of default image.
func getImage(customRegistry, envName string) (string, error) {
	defaultImage := os.Getenv(envName)
	if defaultImage == "" {
		return "", fmt.Errorf("environment variable %s not defined", envName)
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
