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

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getBootstrapSecret looks for the bootstrap secret from bootstrap sa
func getBootstrapSecret(ctx context.Context, kubeClient kubernetes.Interface, managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error) {
	saName := getBootstrapSAName(managedCluster.Name)
	sa, err := kubeClient.CoreV1().ServiceAccounts(managedCluster.Name).Get(ctx, saName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	for _, secretRef := range sa.Secrets {
		if secretRef.Namespace != "" && secretRef.Namespace != managedCluster.Name {
			continue
		}

		// refer definitions:
		// https://github.com/kubernetes/kubernetes/blob/65178fec72df6275ed0aa3ede12c785ac79ab97a/pkg/controller/serviceaccount/tokens_controller.go#L392
		// https://github.com/kubernetes/kubernetes/blob/65178fec72df6275ed0aa3ede12c785ac79ab97a/staging/src/k8s.io/apiserver/pkg/storage/names/generate.go#L49
		prefix := saName + "-token-"
		if len(prefix) > names.MaxGeneratedNameLength {
			prefix = prefix[:names.MaxGeneratedNameLength]
		}

		if strings.HasPrefix(secretRef.Name, prefix) {
			secret, err := kubeClient.CoreV1().Secrets(managedCluster.Name).Get(ctx, secretRef.Name, metav1.GetOptions{})
			if err != nil {
				continue
			}

			if secret.Type != corev1.SecretTypeServiceAccountToken {
				continue
			}

			token, ok := secret.Data["token"]
			if !ok {
				continue
			}
			if len(token) == 0 {
				continue
			}

			return secret, nil
		}
	}

	return nil, fmt.Errorf("secret with prefix %s and type %s not found in service account %s/%s",
		saName,
		corev1.SecretTypeServiceAccountToken,
		managedCluster.Name,
		saName,
	)
}

// getKubeAPIServerAddress get the kube-apiserver URL from ocp infrastructure
func getKubeAPIServerAddress(ctx context.Context, client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// getKubeAPIServerSecretName iterate through all named certificates from apiserver
// returns the first one which has a name matches the given dnsName
func getKubeAPIServerSecretName(ctx context.Context, client client.Client, dnsName string) (string, error) {
	apiserver := &ocinfrav1.APIServer{}
	if err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, apiserver); err != nil {
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
func checkIsIBMCloud(ctx context.Context, client client.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	if err := client.List(ctx, nodes); err != nil {
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
func getKubeAPIServerCertificate(ctx context.Context, kubeClient kubernetes.Interface, secretName string) ([]byte, error) {
	secret, err := kubeClient.CoreV1().Secrets("openshift-config").Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Failed to get secret", fmt.Sprintf("openshift-config/%s", secretName))
			return nil, nil
		}

		return nil, err
	}

	res, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf("failed to find data[tls.crt] in secret openshift-config/%s", secretName)
	}

	return res, nil
}

// getImagePullSecret get image pull secret from env
func getImagePullSecret(ctx context.Context, clientHolder *helpers.ClientHolder, managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error) {
	secret, err := clientHolder.ImageRegistryClient.Cluster(managedCluster).PullSecret()
	if err != nil {
		return nil, err
	}
	if secret != nil {
		return secret, nil
	}

	defaultSecretName := os.Getenv(defaultImagePullSecretEnvVarName)
	if defaultSecretName == "" {
		log.Info(fmt.Sprintf("Cannot find the default image pull secret of the managed cluster %s from %s",
			managedCluster.Name, defaultImagePullSecretEnvVarName))
		return nil, nil
	}

	ns := os.Getenv(constants.PodNamespaceEnvVarName)
	secret, err = clientHolder.KubeClient.CoreV1().Secrets(ns).Get(ctx, defaultSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func getImage(managedCluster *clusterv1.ManagedCluster, envName string) (string, error) {
	defaultImage := os.Getenv(envName)
	if defaultImage == "" {
		return "", fmt.Errorf("environment variable %s not defined", envName)
	}

	return imageregistry.OverrideImageByAnnotation(managedCluster.GetAnnotations(), defaultImage)
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
func createKubeconfigData(ctx context.Context, clientHolder *helpers.ClientHolder, bootStrapSecret *corev1.Secret) ([]byte, error) {
	saToken := bootStrapSecret.Data["token"]

	kubeAPIServer, err := getKubeAPIServerAddress(ctx, clientHolder.RuntimeClient)
	if err != nil {
		return nil, err
	}

	var certData []byte
	if u, err := url.Parse(kubeAPIServer); err == nil {
		// get the ca cert from ocp apiserver firstly
		apiServerCertSecretName, err := getKubeAPIServerSecretName(ctx, clientHolder.RuntimeClient, u.Hostname())
		if err != nil {
			return nil, err
		}

		if len(apiServerCertSecretName) > 0 {
			apiServerCert, err := getKubeAPIServerCertificate(ctx, clientHolder.KubeClient, apiServerCertSecretName)
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
		} else {
			log.Info(fmt.Sprintf("No ca.crt in the bootstrap secret %s/%s", bootStrapSecret.Namespace, bootStrapSecret.Name))
		}

		// if it's ocp && it's on ibm cloud, we treat it as roks
		isROKS, err := checkIsIBMCloud(ctx, clientHolder.RuntimeClient)
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
				log.Info("Using certs signed by known CAs cas on the ROKS.")
				certData = nil
			} else {
				log.Info("No additional valid certificate found for APIserver on the ROKS. Skipping.")
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

func getBootstrapSAName(clusterName string) string {
	bootstrapSAName := fmt.Sprintf("%s-%s", clusterName, bootstrapSASuffix)
	if len(bootstrapSAName) > 63 {
		return fmt.Sprintf("%s-%s", clusterName[:63-len("-"+bootstrapSASuffix)], bootstrapSASuffix)
	}
	return bootstrapSAName
}
