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

// create kubeconfig for bootstrap
func CreateBootstrapKubeConfig(ctx context.Context, clientHolder *helpers.ClientHolder, saName string, ns string,
	tokenExpirationSeconds int64, klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) ([]byte, []byte, error) {
	token, expiration, err := getBootstrapToken(ctx, clientHolder.KubeClient, saName, ns, tokenExpirationSeconds)
	if err != nil {
		return nil, nil, err
	}

	kubeAPIServer, err := GetKubeAPIServerAddress(ctx, clientHolder.RuntimeClient, klusterletConfig)
	if err != nil {
		return nil, nil, err
	}

	certData, err := GetBootstrapCAData(ctx, clientHolder, kubeAPIServer, ns, klusterletConfig)
	if err != nil {
		return nil, nil, err
	}

	proxyURL, _ := GetProxySettings(klusterletConfig)
	bootstrapConfig := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   kubeAPIServer,
			InsecureSkipTLSVerify:    false,
			CertificateAuthorityData: certData,
			ProxyURL:                 proxyURL,
		}},
		// Define auth based on the obtained client cert.
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			Token: string(token),
		}},
		// Define a context that connects the auth info and cluster, and set it as the default
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	boostrapConfigData, err := runtime.Encode(clientcmdlatest.Codec, &bootstrapConfig)
	if err != nil {
		return nil, nil, err
	}

	return boostrapConfigData, expiration, err
}

const bootstrapSASuffix = "bootstrap-sa"

func GetBootstrapSAName(clusterName string) string {
	bootstrapSAName := fmt.Sprintf("%s-%s", clusterName, bootstrapSASuffix)
	if len(bootstrapSAName) > 63 {
		return fmt.Sprintf("%s-%s", clusterName[:63-len("-"+bootstrapSASuffix)], bootstrapSASuffix)
	}
	return bootstrapSAName
}

// getBootstrapToken lists the secrets from the managed cluster namespace to look for the managed cluster
// bootstrap token firstly (compatibility with the ocp that version is less than 4.11), if there is no
// token found, uses tokenrequest to request token.
func getBootstrapToken(ctx context.Context, kubeClient kubernetes.Interface,
	saName, secretNamespace string, tokenExpirationSeconds int64) ([]byte, []byte, error) {
	secrets, err := kubeClient.CoreV1().Secrets(secretNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
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

		return token, nil, nil
	}

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
		return nil, nil, fmt.Errorf("create token request failed: %v", err)
	}

	expiration, err := tokenRequest.Status.ExpirationTimestamp.MarshalText()
	if err != nil {
		return nil, nil, err
	}

	return []byte(tokenRequest.Status.Token), expiration, nil
}

func GetKubeAPIServerAddress(ctx context.Context, client client.Client,
	klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) (string, error) {
	// use the custom hub Kube APIServer URL if specified
	if klusterletConfig != nil && len(klusterletConfig.Spec.HubKubeAPIServerURL) > 0 {
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
	// use the custom hub Kube APIServer CA bundle if specified
	if klusterletConfig != nil && len(klusterletConfig.Spec.HubKubeAPIServerCABundle) > 0 {
		return klusterletConfig.Spec.HubKubeAPIServerCABundle, nil
	}

	// get caBundle from the kube-root-ca.crt configmap in the pod namespace for non-ocp case.
	if !helpers.DeployOnOCP {
		return getCABundleFromConfigmap(ctx, clientHolder, caNamespace)
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
	klog.Info(fmt.Sprintf("No ca.crt was found, fallback to the %s/kube-root-ca.crt", caNamespace))
	return getCABundleFromConfigmap(ctx, clientHolder, caNamespace)
}

func getCABundleFromConfigmap(ctx context.Context, clientHolder *helpers.ClientHolder, caNamespace string) ([]byte, error) {
	rootCA, err := clientHolder.KubeClient.CoreV1().ConfigMaps(caNamespace).Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return []byte(rootCA.Data["ca.crt"]), nil
}

// getKubeAPIServerSecretName iterate through all named certificates from apiserver
// returns the first one which has a name matches the given dnsName
func getKubeAPIServerSecretName(ctx context.Context, client client.Client, dnsName string) (string, error) {
	apiserver := &ocinfrav1.APIServer{}
	if err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, apiserver); err != nil {
		if apierrors.IsNotFound(err) || helpers.ResourceIsNotFound(err) {
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
	proxyConfig := klusterletConfig.Spec.HubKubeAPIServerProxyConfig

	// use https proxy if both http and https proxy are specified
	if len(proxyConfig.HTTPSProxy) > 0 {
		return proxyConfig.HTTPSProxy, proxyConfig.CABundle
	}

	return proxyConfig.HTTPProxy, nil
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
