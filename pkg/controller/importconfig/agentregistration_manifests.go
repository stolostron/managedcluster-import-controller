package importconfig

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/utils/pointer"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

const (
	AgentRegistrationDefaultBootstrapName = "agent-registration-bootstrap"
)

func getKlusterletManifests() (files []string) {
	// klusterlet Operator Files(namespace should be deploy first)
	files = append(files, klusterletOperatorFiles...)

	// klusterlet
	files = append(files, klusterletFiles...)
	return
}

func getKlusterletCRDs() (files []string) {
	return append(files, klusterletCrdsV1File, klusterletCrdsV1beta1File)
}

func bootstrapToken(ctx context.Context, kubeClient kubernetes.Interface, saName string, secretNamespace string) ([]byte, []byte, error) {
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
				ExpirationSeconds: pointer.Int64Ptr(7 * 24 * 3600), // 7 days
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, nil, err
	}

	expiration, err := tokenRequest.Status.ExpirationTimestamp.MarshalText()
	if err != nil {
		return nil, nil, err
	}

	return []byte(tokenRequest.Status.Token), expiration, nil
}

func boostrapCAData(ctx context.Context, clientHolder *helpers.ClientHolder, kubeAPIServer string, caNamespace string) ([]byte, error) {
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

func agentRegistrationDefaultBootstrapKubeconfig(ctx context.Context, clientHolder *helpers.ClientHolder) ([]byte, []byte, error) {
	podNS := os.Getenv(constants.PodNamespaceEnvVarName)

	token, expiration, err := bootstrapToken(ctx, clientHolder.KubeClient, AgentRegistrationDefaultBootstrapName, podNS)
	if err != nil {
		return nil, nil, err
	}

	kubeAPIServer, err := getKubeAPIServerAddress(ctx, clientHolder.RuntimeClient)
	if err != nil {
		return nil, nil, err
	}

	certData, err := boostrapCAData(ctx, clientHolder, kubeAPIServer, podNS)
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

func getImageFromEvnName(envName string) (string, error) {
	defaultImage := os.Getenv(envName)
	if defaultImage == "" {
		return "", fmt.Errorf("environment variable %s not defined", envName)
	}
	return defaultImage, nil
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

func GenerateKlusterletCRDs() ([]byte, error) {
	// Get klusterlet crds
	crdFiles := getKlusterletCRDs()
	crds := new(bytes.Buffer)

	for _, file := range crdFiles {
		crdByte, err := manifestFiles.ReadFile(file)
		if err != nil {
			return nil, err
		}

		crds.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(crdByte)))
	}

	return crds.Bytes(), nil
}

func GenerateAgentRegistrationManifests(ctx context.Context, clientHolder *helpers.ClientHolder, clusterID string) ([]byte, error) {
	// Bootstrap kubeconfig
	bootstrapKubeconfigData, _, err := agentRegistrationDefaultBootstrapKubeconfig(ctx, clientHolder)
	if err != nil {
		return nil, err
	}

	// Image Pull
	imagePullSecret, err := getDefaultImagePullSecret(ctx, clientHolder)
	if err != nil {
		return nil, err
	}
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
			return nil, fmt.Errorf("there is invalid type of the data of pull secret %v/%v",
				imagePullSecret.GetNamespace(), imagePullSecret.GetName())
		}
	}

	// Images
	registrationOperatorImageName, err := getImageFromEvnName(registrationOperatorImageEnvVarName)
	if err != nil {
		return nil, err
	}
	registrationImageName, err := getImageFromEvnName(registrationImageEnvVarName)
	if err != nil {
		return nil, err
	}
	workImageName, err := getImageFromEvnName(workImageEnvVarName)
	if err != nil {
		return nil, err
	}

	config := DefaultRenderConfig{
		KlusterletRenderConfig: KlusterletRenderConfig{
			ManagedClusterNamespace: clusterID,
			KlusterletNamespace:     defaultKlusterletNamespace,            // TODO: get via `klusterletNamespace`
			InstallMode:             string(operatorv1.InstallModeDefault), // TODO: only support default mode by now
			BootstrapKubeconfig:     base64.StdEncoding.EncodeToString(bootstrapKubeconfigData),
			RegistrationImageName:   registrationImageName,
			WorkImageName:           workImageName,
			NodeSelector:            make(map[string]string),
			Tolerations: []corev1.Toleration{
				{
					Effect:   corev1.TaintEffectNoSchedule,
					Key:      "node-role.kubernetes.io/infra",
					Operator: corev1.TolerationOpExists,
				},
			},
		},
		RegistrationOperatorImage: registrationOperatorImageName,
		// Image Pull
		UseImagePullSecret:       useImagePullSecret,
		ImagePullSecretName:      managedClusterImagePullSecretName,
		ImagePullSecretData:      imagePullSecretDataBase64,
		ImagePullSecretType:      imagePullSecretType,
		ImagePullSecretConfigKey: dockerConfigKey,
		ClusterAnnotations:       map[string]string{"agent.open-cluster-management.io/create-with-default-klusterletaddonconfig": "true"},
	}

	agentRegistrationManifests := new(bytes.Buffer)
	files := getKlusterletManifests()
	for _, file := range files {
		template, err := manifestFiles.ReadFile(file)
		if err != nil {
			panic(err)
		}

		raw := helpers.MustCreateAssetFromTemplate(file, template, config)
		agentRegistrationManifests.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(raw)))
	}

	return agentRegistrationManifests.Bytes(), nil
}
