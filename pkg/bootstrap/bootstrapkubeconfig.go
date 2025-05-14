// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	authv1 "k8s.io/api/authentication/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	ocinfrav1 "github.com/openshift/api/config/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	apiServerInternalEndpoint   = "https://kubernetes.default.svc:443"
	apiServerInternalEndpointCA = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// create kubeconfig for bootstrap
func CreateBootstrapKubeConfig(ctxClusterName string,
	kubeAPIServer, proxyURL, ca string, caData, token []byte) ([]byte, error) {

	// CA file and CA data cannot be set simultaneously
	if len(caData) > 0 {
		ca = ""
	}

	bootstrapConfig := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{
			ctxClusterName: {
				Server:                   kubeAPIServer,
				InsecureSkipTLSVerify:    false,
				CertificateAuthority:     ca,
				CertificateAuthorityData: caData,
				ProxyURL:                 proxyURL,
			}},
		// Define auth based on the obtained client cert.
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			Token: string(token),
		}},
		// Define a context that connects the auth info and cluster, and set it as the default
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   ctxClusterName,
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	boostrapConfigData, err := runtime.Encode(clientcmdlatest.Codec, &bootstrapConfig)
	if err != nil {
		return nil, err
	}

	return boostrapConfigData, err
}

// GetKubeAPIServerConfig returns the expected apiserver url, proxy url, ca file and ca data
// for cluster registration.
func GetKubeAPIServerConfig(ctx context.Context, clientHolder *helpers.ClientHolder, ns string,
	klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig, selfManaged bool) (string, string,
	string, []byte, error) {
	// the proxy settings in the klusterletConfig will be ignored when the internal endpoint
	// is used for the self managed cluster
	if selfManaged && !hasCustomServerURLOrStrategy(klusterletConfig) {
		return apiServerInternalEndpoint, "", apiServerInternalEndpointCA, nil, nil
	}

	// get the proxy settings
	proxy, _ := GetProxySettings(klusterletConfig)

	// get the apiserver address
	url, err := GetKubeAPIServerAddress(ctx, clientHolder.RuntimeClient, klusterletConfig)
	if err != nil {
		return "", "", "", nil, err
	}

	// get the ca data
	caData, err := GetBootstrapCAData(ctx, clientHolder, url, ns, klusterletConfig)
	if err != nil {
		return "", "", "", nil, err
	}

	return url, proxy, "", caData, err
}

// Return true if the managed cluster has a custom URL or its server verification strategy
// is not `UseAutoDetectedCABundle`.
func hasCustomServerURLOrStrategy(klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) bool {
	if klusterletConfig == nil {
		return false
	}

	if klusterletConfig.Spec.HubKubeAPIServerConfig != nil {
		if len(klusterletConfig.Spec.HubKubeAPIServerConfig.URL) > 0 {
			return true
		}
		if len(klusterletConfig.Spec.HubKubeAPIServerConfig.ServerVerificationStrategy) > 0 &&
			klusterletConfig.Spec.HubKubeAPIServerConfig.ServerVerificationStrategy !=
				klusterletconfigv1alpha1.ServerVerificationStrategyUseAutoDetectedCABundle {
			return true
		}
	} else {
		if len(klusterletConfig.Spec.HubKubeAPIServerURL) > 0 {
			return true
		}
		if len(klusterletConfig.Spec.HubKubeAPIServerCABundle) > 0 {
			return true
		}
	}

	return false
}

func RequestSAToken(ctx context.Context, kubeClient kubernetes.Interface, saName, secretNamespace string,
	tokenExpirationSeconds int64) ([]byte, []byte, []byte, error) {
	klog.Infof("request a new token for serviceaccount %s/%s", secretNamespace, saName)
	tokenRequest, err := kubeClient.CoreV1().ServiceAccounts(secretNamespace).CreateToken(
		ctx,
		saName,
		&authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{
				ExpirationSeconds: pointer.Int64(tokenExpirationSeconds),
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create token request failed: %v", err)
	}

	tokenCreation, err := tokenRequest.CreationTimestamp.MarshalText()
	if err != nil {
		return nil, nil, nil, err
	}
	expiration, err := tokenRequest.Status.ExpirationTimestamp.MarshalText()
	if err != nil {
		return nil, nil, nil, err
	}

	return []byte(tokenRequest.Status.Token), tokenCreation, expiration, nil
}

// GetBootstrapToken lists the secrets from the managed cluster namespace to look for the managed cluster
// bootstrap token firstly (compatibility with the ocp that version is less than 4.11), if there is no
// token found, uses tokenrequest to request token.
func GetBootstrapToken(ctx context.Context, kubeClient kubernetes.Interface,
	saName, secretNamespace string, tokenExpirationSeconds int64) ([]byte, []byte, []byte, error) {
	secrets, err := kubeClient.CoreV1().Secrets(secretNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, nil, err
	}

	for _, secret := range secrets.Items {
		if secret.Type != corev1.SecretTypeServiceAccountToken {
			continue
		}

		// refer definitions:
		// https://github.com/kubernetes/kubernetes/blob/v1.11.0/pkg/controller/serviceaccount/tokens_controller.go#L386
		// https://github.com/kubernetes/kubernetes/blob/v1.11.0/staging/src/k8s.io/apiserver/pkg/storage/names/generate.go#L49
		prefix := saName + "-token-"
		if len(prefix) > names.MaxGeneratedNameLength {
			prefix = prefix[:names.MaxGeneratedNameLength]
		}

		if !strings.HasPrefix(secret.Name, prefix) {
			continue
		}

		token, ok := secret.Data["token"]
		if !ok {
			continue
		}

		if len(token) == 0 {
			continue
		}

		return token, nil, nil, nil
	}

	return RequestSAToken(ctx, kubeClient, saName, secretNamespace, tokenExpirationSeconds)
}

func GetKubeAPIServerAddress(ctx context.Context, client client.Client,
	klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) (string, error) {

	if klusterletConfig != nil && klusterletConfig.Spec.HubKubeAPIServerConfig != nil &&
		len(klusterletConfig.Spec.HubKubeAPIServerConfig.URL) > 0 {
		return klusterletConfig.Spec.HubKubeAPIServerConfig.URL, nil
	}

	// TODO: DEPRECATE the following code and only use the HubKubeAPIServerConfig in the future
	// use the custom hub Kube APIServer URL if specified
	if klusterletConfig != nil && klusterletConfig.Spec.HubKubeAPIServerConfig == nil &&
		len(klusterletConfig.Spec.HubKubeAPIServerURL) > 0 {
		return klusterletConfig.Spec.HubKubeAPIServerURL, nil
	}

	if !helpers.DeployOnOCP {
		return "", fmt.Errorf("failed get Hub kube apiserver on non-OCP cluster")
	}

	infraConfig := &ocinfrav1.Infrastructure{}
	err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, infraConfig)
	if err == nil {
		return infraConfig.Status.APIServerURL, nil
	}
	if helpers.ResourceIsNotFound(err) {
		return "", fmt.Errorf("cannot get kubeAPIServer URL since the Infrastructure is not found, please use" +
			"klusterletConfig to set the hub kubeAPIServer URL")
	}

	return "", err
}

// GetKubeconfigClusterName returns the cluster name used in the bootstrap kubeconfig current context.
// This is to fix the issue that when the hub cluster is rebuilt, and we backup restore the resources
// on the same hub cluster, even the bootstrp kubeconfig is generated by the new hub cluster, it will not
// trigger the agent to rebootstrap. Using a different cluster name will make the agent to rebootstrap.
// Currently, the above issue is only fixed on OCP as we get the cluster name from the infrastructure.
// TODO: On non-OCP, we use "default-cluster" as the cluster name now and need to fix it in the future.
func GetKubeconfigClusterName(ctx context.Context, client client.Client) (string, error) {
	defaultCluster := "default-cluster"
	if !helpers.DeployOnOCP {
		klog.Infof("Deploying on non-OCP cluster, using %s as the cluster name", defaultCluster)
		return defaultCluster, nil
	}

	infraConfig := &ocinfrav1.Infrastructure{}
	err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, infraConfig)
	if err == nil {
		return string(infraConfig.UID), nil
	}
	if helpers.ResourceIsNotFound(err) {
		klog.Infof("Infrastructure is not found, using %s as the cluster name", defaultCluster)
		return defaultCluster, nil
	}

	return "", err
}

func GetBootstrapCAData(ctx context.Context, clientHolder *helpers.ClientHolder, kubeAPIServer string,
	caNamespace string, klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) ([]byte, error) {
	apiServerCAData, err := getKubeAPIServerCAData(ctx, clientHolder, kubeAPIServer, caNamespace, klusterletConfig)
	if err != nil {
		return nil, err
	}

	_, proxyCAData := GetProxySettings(klusterletConfig)
	return mergeCertificateData(apiServerCAData, proxyCAData)
}

func getKubeAPIServerCAData(ctx context.Context, clientHolder *helpers.ClientHolder, kubeAPIServer string,
	caNamespace string, klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) ([]byte, error) {

	if klusterletConfig != nil && klusterletConfig.Spec.HubKubeAPIServerConfig != nil {
		return getKubeAPIServerCADataFromConfig(ctx, clientHolder, kubeAPIServer, caNamespace,
			klusterletConfig.Spec.HubKubeAPIServerConfig)
	}

	// TODO: DEPRECATE the following code and only use the HubKubeAPIServerConfig in the future
	// use the custom hub Kube APIServer CA bundle if specified
	if klusterletConfig != nil && klusterletConfig.Spec.HubKubeAPIServerConfig == nil &&
		len(klusterletConfig.Spec.HubKubeAPIServerCABundle) > 0 {
		return klusterletConfig.Spec.HubKubeAPIServerCABundle, nil
	}

	return autoDetectCAData(ctx, clientHolder, kubeAPIServer, caNamespace)
}

func getKubeAPIServerCADataFromConfig(ctx context.Context, clientHolder *helpers.ClientHolder, kubeAPIServer string,
	caNamespace string, config *klusterletconfigv1alpha1.KubeAPIServerConfig) ([]byte, error) {
	if config == nil {
		return nil, fmt.Errorf("failed to get ca data from the custom kubeAPIServerConfig, config is nil")
	}

	switch config.ServerVerificationStrategy {
	case klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore:
		return nil, nil
	case klusterletconfigv1alpha1.ServerVerificationStrategyUseAutoDetectedCABundle, "":
		detectedCA, err := autoDetectCAData(ctx, clientHolder, kubeAPIServer, caNamespace)
		if err != nil {
			return nil, err
		}
		customCustomCA, err := getCustomCAData(ctx, clientHolder, config.TrustedCABundles)
		if err != nil {
			return nil, err
		}
		return mergeCertificateData(detectedCA, customCustomCA)
	case klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles:
		return getCustomCAData(ctx, clientHolder, config.TrustedCABundles)
	default:
		return nil, fmt.Errorf("unknown server verification strategy: %s", config.ServerVerificationStrategy)
	}

}

func getCustomCAData(ctx context.Context, clientHolder *helpers.ClientHolder,
	caBundles []klusterletconfigv1alpha1.CABundle) ([]byte, error) {
	if len(caBundles) == 0 {
		return nil, nil
	}

	var all []byte
	for _, caBundle := range caBundles {
		data, err := getCABundleFromConfigmap(ctx, clientHolder,
			caBundle.CABundle.Namespace, caBundle.CABundle.Name)
		if err != nil {
			return nil, err
		}
		all, err = mergeCertificateData(all, data)
		if err != nil {
			return nil, err
		}
	}

	return all, nil
}

func autoDetectCAData(ctx context.Context, clientHolder *helpers.ClientHolder, kubeAPIServer string,
	caNamespace string) ([]byte, error) {
	// get caBundle from the kube-root-ca.crt configmap in the pod namespace for non-ocp case.
	if !helpers.DeployOnOCP {
		return getKubeRootCABundle(ctx, clientHolder, caNamespace)
	}

	// and then get the ca cert from ocp apiserver firstly
	if u, err := url.Parse(kubeAPIServer); err == nil {
		apiServerCertSecretName, err := getKubeAPIServerSecretName(ctx, clientHolder.RuntimeClient, u.Hostname())
		if err != nil {
			return nil, err
		}

		if len(apiServerCertSecretName) > 0 {
			certData, err := getCustomKubeAPIServerCertificate(ctx, clientHolder.KubeClient, apiServerCertSecretName)
			if err != nil {
				return nil, err
			}

			if len(certData) != 0 {
				klog.Info(fmt.Sprintf("Using openshift-config/%s as the bootstrap ca", apiServerCertSecretName))
				return certData, nil
			}
		}
	}

	// failed to get the ca from ocp apiserver, if the cluster is on ibm cloud, we treat it as roks
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
			klog.Info("Using certs signed by known CAs cas on the ROKS.")
			return nil, nil
		} else {
			klog.Info("No additional valid certificate found for APIserver on the ROKS, skipping.")
		}
	}

	// failed to get the ca from ocp, fallback to the kube-root-ca.crt configmap from the pod namespace.
	klog.V(5).Info(fmt.Sprintf("No ca.crt was found, fallback to the %s/kube-root-ca.crt", caNamespace))
	return getKubeRootCABundle(ctx, clientHolder, caNamespace)
}

func getKubeRootCABundle(ctx context.Context, clientHolder *helpers.ClientHolder,
	caNamespace string) ([]byte, error) {
	return getCABundleFromConfigmap(ctx, clientHolder, caNamespace, "kube-root-ca.crt", "ca.crt")
}

func getCABundleFromConfigmap(ctx context.Context, clientHolder *helpers.ClientHolder,
	caNamespace, caName string, caKeys ...string) ([]byte, error) {
	rootCA, err := clientHolder.KubeClient.CoreV1().ConfigMaps(caNamespace).Get(ctx, caName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if len(caKeys) == 0 {
		caKeys = []string{"ca-bundle.crt", "ca.crt", "tls.crt"}
	}

	for _, key := range caKeys {
		if data, ok := rootCA.Data[key]; ok {
			return []byte(data), nil
		}
	}

	return nil, fmt.Errorf("failed to find ca data in configmap %s/%s", caNamespace, caName)
}

// getKubeAPIServerSecretName iterate through all named certificates from apiserver
// returns the first one which has a name matches the given dnsName
func getKubeAPIServerSecretName(ctx context.Context, client client.Client, dnsName string) (string, error) {
	apiserver := &ocinfrav1.APIServer{}
	if err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, apiserver); err != nil {
		if helpers.ResourceIsNotFound(err) {
			klog.Info("Ignore ocp apiserver, it is not found")
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
		klog.V(4).Info("Failed to get nodes")
		return false, nil
	}

	providerID := nodes.Items[0].Spec.ProviderID
	if strings.HasPrefix(providerID, "ibm") {
		return true, nil
	}

	return false, nil
}

// getCustomKubeAPIServerCertificate looks for secret in openshift-config namespace, and returns tls.crt
func getCustomKubeAPIServerCertificate(ctx context.Context, kubeClient kubernetes.Interface,
	secretName string) ([]byte, error) {
	secret, err := kubeClient.CoreV1().Secrets("openshift-config").Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info(fmt.Sprintf("Failed to get secret openshift-config/%s, skipping", secretName))
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
		if errors.As(err, &x509.UnknownAuthorityError{}) {
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

func GetProxySettings(klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) (string, []byte) {
	if klusterletConfig == nil {
		return "", nil
	}

	if klusterletConfig.Spec.HubKubeAPIServerConfig != nil &&
		len(klusterletConfig.Spec.HubKubeAPIServerConfig.ProxyURL) > 0 {
		// the TrustedCABundles configured in the HubKubeAPIServerConfig is already added by the getKubeAPIServerCAData
		return klusterletConfig.Spec.HubKubeAPIServerConfig.ProxyURL, nil
	}

	// TODO: DEPRECATE the following code and only return the proxyURL
	if klusterletConfig.Spec.HubKubeAPIServerConfig == nil {
		proxyConfig := klusterletConfig.Spec.HubKubeAPIServerProxyConfig

		// use https proxy if both http and https proxy are specified
		if len(proxyConfig.HTTPSProxy) > 0 {
			return proxyConfig.HTTPSProxy, proxyConfig.CABundle
		}

		return proxyConfig.HTTPProxy, nil
	}

	return "", nil
}

func mergeCertificateData(caBundles ...[]byte) ([]byte, error) {
	var all []*x509.Certificate
	for _, caBundle := range caBundles {
		if len(caBundle) == 0 {
			continue
		}
		certs, err := certutil.ParseCertsPEM(caBundle)
		if err != nil {
			return []byte{}, err
		}
		all = append(all, certs...)
	}

	// remove duplicated cert
	var merged []*x509.Certificate
	for i := range all {
		found := false
		for j := range merged {
			if reflect.DeepEqual(all[i].Raw, merged[j].Raw) {
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, all[i])
		}
	}

	// encode the merged certificates
	b := bytes.Buffer{}
	for _, cert := range merged {
		if err := pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
			return []byte{}, err
		}
	}
	return b.Bytes(), nil
}

// ValidateBootstrapKubeconfig validates the bootstrap kubeconfig data by checking for changes in:
//   - the kube apiserver address
//   - the CA file path
//   - the CA data
//   - the proxy url
//   - the context cluster name
func ValidateBootstrapKubeconfig(clusterName string,
	kubeAPIServer, proxyURL, ca string, caData []byte, ctxClusterName string,
	requiredKubeAPIServer, requiredProxyURL, requiredCA string, requiredCAData []byte,
	requiredCtxClusterName string) bool {
	// validate kube api server endpoint
	if kubeAPIServer != requiredKubeAPIServer {
		klog.Infof("KubeAPIServer invalid for the managed cluster %s: %s", clusterName, kubeAPIServer)
		return false
	}

	// validate kube api server CA file path
	if ca != requiredCA {
		klog.Infof("CA is invalid for the managed cluster %s: %s", clusterName, ca)
		return false
	}

	// validate kube api server CA data
	if !bytes.Equal(caData, requiredCAData) {
		klog.Infof("CAdata is invalid for the managed cluster %s", clusterName)
		return false
	}

	// validate proxy server url
	if proxyURL != requiredProxyURL {
		klog.Infof("Proxy config is invalid for the managed cluster %s: %s", clusterName, proxyURL)
		return false
	}

	// validate context cluster name
	if ctxClusterName != requiredCtxClusterName {
		klog.Infof("Context cluster name is invalid for the managed cluster %s: %s", clusterName, ctxClusterName)
		return false
	}

	return true
}
