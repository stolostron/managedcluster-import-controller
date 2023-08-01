// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

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
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/utils/pointer"

	authv1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
			log.Info("Ignore ocp apiserver, it is not found")
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
			log.Info(fmt.Sprintf("Failed to get secret openshift-config/%s, skipping", secretName))
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

	return getDefaultImagePullSecret(ctx, clientHolder)
}

func getDefaultImagePullSecret(ctx context.Context, clientHolder *helpers.ClientHolder) (*corev1.Secret, error) {
	var err error

	defaultSecretName := os.Getenv(defaultImagePullSecretEnvVarName)
	if defaultSecretName == "" {
		log.Info(fmt.Sprintf("Ignore the image pull secret, it can't be found from from env %s", defaultImagePullSecretEnvVarName))
		return nil, nil
	}

	ns := os.Getenv(constants.PodNamespaceEnvVarName)
	secret, err := clientHolder.KubeClient.CoreV1().Secrets(ns).Get(ctx, defaultSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func getImage(envName string, clusterAnnotations map[string]string) (string, error) {
	defaultImage := os.Getenv(envName)
	if defaultImage == "" {
		return "", fmt.Errorf("environment variable %s not defined", envName)
	}

	return imageregistry.OverrideImageByAnnotation(clusterAnnotations, defaultImage)
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

func getBootstrapKubeConfigData(ctx context.Context, clientHolder *helpers.ClientHolder, cluster *clusterv1.ManagedCluster) ([]byte, []byte, error) {
	importSecretName := fmt.Sprintf("%s-%s", cluster.Name, constants.ImportSecretNameSuffix)
	importSecret, err := clientHolder.KubeClient.CoreV1().Secrets(cluster.Name).Get(ctx, importSecretName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// create bootstrap kubeconfig
		return createKubeConfig(ctx, clientHolder, getBootstrapSAName(cluster.Name), cluster.Name, 8640*3600)
	}
	if err != nil {
		return nil, nil, err
	}

	kubeConifgData := getBootstrapKubeConfigDataFromImportSecret(importSecret)
	kubeAPIServer, caData, token, err := parseKubeConfigData(kubeConifgData)
	if err != nil {
		return nil, nil, err
	}

	// check if the kube apiserver address is changed
	validKubeAPIServer, err := validateKubeAPIServerAddress(ctx, kubeAPIServer, clientHolder)
	if err != nil {
		return nil, nil, err
	}

	// check if the CA data is changed
	validCAData, err := validateCAData(ctx, caData, kubeAPIServer, clientHolder, cluster)
	if err != nil {
		return nil, nil, err
	}

	expiration := importSecret.Data[constants.ImportSecretTokenExpiration]
	if validKubeAPIServer && validCAData && validateToken(token, expiration) {
		// both the kube apiserver address, CA data and token is valid, return the current kubeconfig
		return kubeConifgData, expiration, nil
	}

	// recreate bootstrap kubeconfig
	return createKubeConfig(ctx, clientHolder, getBootstrapSAName(cluster.Name), cluster.Name, 8640*3600)
}

func getBootstrapKubeConfigDataFromImportSecret(importSecret *corev1.Secret) []byte {
	importYaml, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
	if !ok {
		return nil
	}

	for _, yaml := range helpers.SplitYamls(importYaml) {
		obj := helpers.MustCreateObject(yaml)
		switch secret := obj.(type) {
		case *corev1.Secret:
			if secret.Name == "bootstrap-hub-kubeconfig" {
				return secret.Data["kubeconfig"]
			}
		}
	}

	return nil
}
func parseKubeConfigData(kubeConfigData []byte) (kubeAPIServer string, caData []byte, token string, err error) {
	if len(kubeConfigData) == 0 {
		// kubeconfig data is empty
		return "", nil, "", nil
	}

	config, err := clientcmd.Load(kubeConfigData)
	if err != nil {
		// kubeconfig data is invalid
		return "", nil, "", err
	}

	if cluster, ok := config.Clusters["default-cluster"]; ok {
		kubeAPIServer = cluster.Server
		caData = cluster.CertificateAuthorityData
	}

	if authInfo, ok := config.AuthInfos["default-auth"]; ok {
		token = authInfo.Token
	}

	return
}

func validateKubeAPIServerAddress(ctx context.Context, kubeAPIServer string, clientHolder *helpers.ClientHolder) (bool, error) {
	if len(kubeAPIServer) == 0 {
		return false, nil
	}

	currentKubeAPIServer, err := getKubeAPIServerAddress(ctx, clientHolder.RuntimeClient)
	if err != nil {
		return false, err
	}

	return kubeAPIServer == currentKubeAPIServer, nil
}

func validateCAData(ctx context.Context, caData []byte, kubeAPIServer string, clientHolder *helpers.ClientHolder,
	cluster *clusterv1.ManagedCluster) (bool, error) {
	if len(caData) == 0 {
		// CA data is empty
		return false, nil
	}

	currentCAData, err := getBootstrapCAData(ctx, clientHolder, kubeAPIServer, cluster.Name)
	if err != nil {
		return false, err
	}

	return reflect.DeepEqual(caData, currentCAData), nil
}

func validateToken(token string, expiration []byte) bool {
	if len(token) == 0 {
		// no token in the kubeconfig
		return false
	}

	if len(expiration) == 0 {
		// token is from the service account token secret
		return true
	}

	expirationTime, err := time.Parse(time.RFC3339, string(expiration))
	if err != nil {
		return false
	}

	now := metav1.Now()
	refreshThreshold := 8640 * time.Hour / 5
	lifetime := expirationTime.Sub(now.Time)
	return lifetime > refreshThreshold
}

// create kubeconfig from bootstrap secret
func createKubeConfig(ctx context.Context, clientHolder *helpers.ClientHolder, saName string, ns string, tokenExpirationSeconds int64) ([]byte, []byte, error) {
	token, expiration, err := getBootstrapToken(ctx, clientHolder.KubeClient, saName, ns, tokenExpirationSeconds)
	if err != nil {
		return nil, nil, err
	}

	kubeAPIServer, err := getKubeAPIServerAddress(ctx, clientHolder.RuntimeClient)
	if err != nil {
		return nil, nil, err
	}

	certData, err := getBootstrapCAData(ctx, clientHolder, kubeAPIServer, ns)
	if err != nil {
		return nil, nil, err
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

func getBootstrapCAData(ctx context.Context, clientHolder *helpers.ClientHolder, kubeAPIServer string, caNamespace string) ([]byte, error) {
	// get the ca cert from ocp apiserver firstly
	if u, err := url.Parse(kubeAPIServer); err == nil {
		apiServerCertSecretName, err := getKubeAPIServerSecretName(ctx, clientHolder.RuntimeClient, u.Hostname())
		if err != nil {
			return nil, err
		}

		if len(apiServerCertSecretName) > 0 {
			certData, err := getKubeAPIServerCertificate(ctx, clientHolder.KubeClient, apiServerCertSecretName)
			if err != nil {
				return nil, err
			}

			if len(certData) != 0 {
				log.Info(fmt.Sprintf("Using openshift-config/%s as the bootstrap ca", apiServerCertSecretName))
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
			log.Info("Using certs signed by known CAs cas on the ROKS.")
			return nil, nil
		} else {
			log.Info("No additional valid certificate found for APIserver on the ROKS, skipping.")
		}
	}

	// failed to get the ca from ocp, fallback to the kube-root-ca.crt configmap from the pod namespace.
	log.Info(fmt.Sprintf("No ca.crt was found, fallback to the %s/kube-root-ca.crt", caNamespace))
	rootCA, err := clientHolder.KubeClient.CoreV1().ConfigMaps(caNamespace).Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return []byte(rootCA.Data["ca.crt"]), nil
}

func getBootstrapSAName(clusterName string) string {
	bootstrapSAName := fmt.Sprintf("%s-%s", clusterName, bootstrapSASuffix)
	if len(bootstrapSAName) > 63 {
		return fmt.Sprintf("%s-%s", clusterName[:63-len("-"+bootstrapSASuffix)], bootstrapSASuffix)
	}
	return bootstrapSAName
}

func filesToTemplateBytes(files []string, config interface{}) ([]byte, error) {
	manifests := new(bytes.Buffer)
	for _, file := range files {
		b, err := manifestFiles.ReadFile(file)
		if err != nil {
			return nil, err
		}

		if config != nil {
			b = helpers.MustCreateAssetFromTemplate(file, b, config)
		}
		manifests.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(b)))
	}
	return manifests.Bytes(), nil
}

func getImagePullSecretConfig(imagePullSecret *corev1.Secret) (ImagePullSecretConfig, error) {
	useImagePullSecret := false
	var imagePullSecretType corev1.SecretType
	var dockerConfigKey string
	imagePullSecretDataBase64 := ""
	if imagePullSecret != nil {
		switch {
		case len(imagePullSecret.Data[corev1.DockerConfigJsonKey]) != 0:
			dockerConfigKey = corev1.DockerConfigJsonKey
			imagePullSecretType = corev1.SecretTypeDockerConfigJson
			imagePullSecretDataBase64 = base64.StdEncoding.EncodeToString(imagePullSecret.Data[corev1.DockerConfigJsonKey])
			useImagePullSecret = true
		case len(imagePullSecret.Data[corev1.DockerConfigKey]) != 0:
			dockerConfigKey = corev1.DockerConfigKey
			imagePullSecretType = corev1.SecretTypeDockercfg
			imagePullSecretDataBase64 = base64.StdEncoding.EncodeToString(imagePullSecret.Data[corev1.DockerConfigKey])
			useImagePullSecret = true
		default:
			return ImagePullSecretConfig{}, fmt.Errorf("there is invalid type of the data of pull secret %v/%v",
				imagePullSecret.GetNamespace(), imagePullSecret.GetName())
		}
	}

	return ImagePullSecretConfig{
		UseImagePullSecret:       useImagePullSecret,
		ImagePullSecretName:      managedClusterImagePullSecretName,
		ImagePullSecretType:      imagePullSecretType,
		ImagePullSecretData:      imagePullSecretDataBase64,
		ImagePullSecretConfigKey: dockerConfigKey,
	}, nil
}
