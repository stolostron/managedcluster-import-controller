// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"fmt"
	"strings"
	"time"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

func getImportSecret(ctx context.Context, clientHolder *helpers.ClientHolder, clusterName string) (*corev1.Secret, error) {
	importSecretName := fmt.Sprintf("%s-%s", clusterName, constants.ImportSecretNameSuffix)
	return clientHolder.KubeClient.CoreV1().Secrets(clusterName).Get(ctx, importSecretName, metav1.GetOptions{})
}

func extractBootstrapKubeConfigDataFromImportSecret(importSecret *corev1.Secret) []byte {
	if importSecret == nil {
		return nil
	}

	importYaml, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
	if !ok {
		return nil
	}

	for _, yaml := range helpers.SplitYamls(importYaml) {
		obj := helpers.MustCreateObject(yaml)
		if secret, ok := obj.(*corev1.Secret); ok {
			if secret.Name == constants.DefaultBootstrapHubKubeConfigSecretName {
				return secret.Data["kubeconfig"]
			}
		}
	}

	return nil
}

func parseKubeConfigData(kubeConfigData []byte) (
	kubeAPIServer, proxyURL, ca string, caData []byte, token string, ctxClusterName string, err error) {

	config, err := clientcmd.Load(kubeConfigData)
	if err != nil {
		// kubeconfig data is invalid
		return "", "", "", nil, "", "", err
	}

	context := config.Contexts[config.CurrentContext]
	if context == nil {
		return "", "", "", nil, "", "", fmt.Errorf("failed to get current context")
	}

	if cluster, ok := config.Clusters[context.Cluster]; ok {
		ctxClusterName = context.Cluster
		kubeAPIServer = cluster.Server
		ca = cluster.CertificateAuthority
		caData = cluster.CertificateAuthorityData
		proxyURL = cluster.ProxyURL
	}

	if authInfo, ok := config.AuthInfos["default-auth"]; ok {
		token = authInfo.Token
	}

	return
}

// validateLegacyServiceAccountToken validates that a legacy serviceaccount token secret exists
// and contains the expected token value
func validateLegacyServiceAccountToken(ctx context.Context, kubeClient kubernetes.Interface,
	saName, secretNamespace, expectedToken string) bool {
	if len(expectedToken) == 0 {
		return false
	}

	secrets, err := kubeClient.CoreV1().Secrets(secretNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("failed to list secrets for serviceaccount token validation: %v", err)
		return false
	}

	// Find the serviceaccount token secret with the expected naming pattern
	prefix := saName + "-token-"
	if len(prefix) > names.MaxGeneratedNameLength {
		prefix = prefix[:names.MaxGeneratedNameLength]
	}

	for _, secret := range secrets.Items {
		if secret.Type != corev1.SecretTypeServiceAccountToken {
			continue
		}

		if !strings.HasPrefix(secret.Name, prefix) {
			continue
		}

		token, ok := secret.Data["token"]
		if !ok || len(token) == 0 {
			continue
		}

		// Check if the token in the secret matches the expected token
		if string(token) == expectedToken {
			return true
		}
	}

	klog.Infof("serviceaccount token validation failed: no matching secret found for %s/%s", secretNamespace, saName)
	return false
}

func validateTokenExpiration(token string, creation, expiration []byte) bool {
	if len(token) == 0 {
		// no token in the kubeconfig
		return false
	}

	if len(expiration) == 0 {
		// token is from the service account token secret - no expiration to validate
		// Additional secret existence validation will be done by the caller with proper context
		return true
	}
	expirationTime, err := time.Parse(time.RFC3339, string(expiration))
	if err != nil {
		klog.Errorf("failed to parse expiration time: %v", err)
		return false
	}

	refreshThreshold := constants.DefaultSecretTokenRefreshThreshold
	if len(creation) != 0 {
		creationTime, err := time.Parse(time.RFC3339, string(creation))
		if err != nil {
			klog.Errorf("failed to parse creation time: %v", err)
			return false
		}

		refreshThreshold = expirationTime.Sub(creationTime) / 5
	}

	lifetime := time.Until(expirationTime)
	return lifetime > refreshThreshold
}

func buildBootstrapKubeconfigData(ctx context.Context, clientHolder *helpers.ClientHolder,
	managedCluster *clusterv1.ManagedCluster,
	klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) ([]byte, []byte, []byte, error) {
	var bootstrapKubeconfigData, tokenData, tokenCreation, tokenExpiration []byte

	// get the import secret
	importSecret, err := getImportSecret(ctx, clientHolder, managedCluster.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, nil, err
	}

	// get the latest kube apiserver configuration
	requiredKubeAPIServer, requiredProxyURL, requiredCA, requiredCAData, err := bootstrap.GetKubeAPIServerConfig(
		ctx, clientHolder, managedCluster.Name, klusterletConfig, isSelfManaged(managedCluster))
	if err != nil {
		return nil, nil, nil, err
	}

	// get the cluster name in the kubeconfig
	requiredCtxClusterName, err := bootstrap.GetKubeconfigClusterName(ctx, clientHolder.RuntimeClient)
	if err != nil {
		return nil, nil, nil, err
	}

	if importSecret == nil {
		klog.Infof("the import secret is missing for the managed cluster %s", managedCluster.Name)
	}

	// check if the bootstrap kubeconfig and token in the import secret are still valid
	if kubeconfigData := extractBootstrapKubeConfigDataFromImportSecret(importSecret); len(kubeconfigData) > 0 {
		kubeAPIServer, proxyURL, ca, caData, tokenString, ctxClusterName, err := parseKubeConfigData(kubeconfigData)
		if err != nil {
			klog.Infof("failed to parse the bootstrap hub kubeconfig in the import.yaml. Recreation is required: %v", err)
		} else {
			// use the existing token if it is still valid
			creation := importSecret.Data[constants.ImportSecretTokenCreation]
			expiration := importSecret.Data[constants.ImportSecretTokenExpiration]
			valid := validateTokenExpiration(tokenString, creation, expiration)

			// For legacy tokens (no expiration), additionally validate the serviceaccount secret exists
			if valid && len(expiration) == 0 {
				saName := helpers.GetBootstrapSAName(managedCluster.Name)
				valid = validateLegacyServiceAccountToken(ctx, clientHolder.KubeClient, saName, managedCluster.Name, tokenString)
				if !valid {
					klog.Infof("legacy serviceaccount token validation failed for managed cluster %s", managedCluster.Name)
				}
			}

			if valid {
				tokenData = []byte(tokenString)
				tokenCreation = creation
				tokenExpiration = expiration
			} else {
				klog.Infof("token should be refreshed for the managed cluster %s, creation: %v, expiration: %v",
					managedCluster.Name, string(creation), string(expiration))
			}

			// use the kubeconfig if it is still valid
			if valid := bootstrap.ValidateBootstrapKubeconfig(managedCluster.Name,
				kubeAPIServer, proxyURL, ca, caData, ctxClusterName,
				requiredKubeAPIServer, requiredProxyURL, requiredCA, requiredCAData, requiredCtxClusterName); valid {
				bootstrapKubeconfigData = kubeconfigData
			}
		}
	}

	// retrieve the non-expiring token if available or generate a new one.
	if len(tokenData) == 0 {
		klog.Infof("create a new token for the managed cluster %s", managedCluster.Name)
		tokenData, tokenCreation, tokenExpiration, err = bootstrap.GetBootstrapToken(ctx, clientHolder.KubeClient,
			helpers.GetBootstrapSAName(managedCluster.Name),
			managedCluster.Name, constants.DefaultSecretTokenExpirationSecond)
		if err != nil {
			return nil, nil, nil, err
		}

		// reset the bootstrap kubeconfig to trigger the regeneration since the token is updated
		bootstrapKubeconfigData = nil
	}

	// create a new bootstrap kubeconfig if it is invalid or missing
	if len(bootstrapKubeconfigData) == 0 {
		klog.Infof("create a new bootstrap kubeconfig for the managed cluster %s", managedCluster.Name)
		bootstrapKubeconfigData, err = bootstrap.CreateBootstrapKubeConfig(requiredCtxClusterName,
			requiredKubeAPIServer, requiredProxyURL, requiredCA, requiredCAData, tokenData)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return bootstrapKubeconfigData, tokenCreation, tokenExpiration, nil
}

func isSelfManaged(managedCluster *clusterv1.ManagedCluster) bool {
	if managedCluster == nil {
		return false
	}
	if value := managedCluster.Labels[constants.SelfManagedLabel]; strings.EqualFold(value, "true") {
		return true
	}
	return false
}

func buildImportSecret(ctx context.Context, clientHolder *helpers.ClientHolder, managedCluster *clusterv1.ManagedCluster,
	mode operatorv1.InstallMode, klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig,
	bootstrapKubeconfigData, tokenCreation, tokenExpiration []byte) (*corev1.Secret, error) {
	var yamlcontent, crdsYAML []byte
	var secretAnnotations map[string]string
	var err error
	switch mode {
	case operatorv1.InstallModeDefault, operatorv1.InstallModeSingleton:
		supportPriorityClass, err := helpers.SupportPriorityClass(managedCluster)
		if err != nil {
			return nil, err
		}
		var priorityClassName string
		if supportPriorityClass {
			priorityClassName = constants.DefaultKlusterletPriorityClassName
		}
		config := bootstrap.NewKlusterletManifestsConfig(
			mode,
			managedCluster.Name,
			bootstrapKubeconfigData).
			WithManagedCluster(managedCluster).
			WithKlusterletConfig(klusterletConfig).
			WithPriorityClassName(priorityClassName)
		yamlcontent, crdsYAML, err = config.Generate(ctx, clientHolder)
		if err != nil {
			return nil, err
		}

	case operatorv1.InstallModeHosted, operatorv1.InstallModeSingletonHosted:
		yamlcontent, _, err = bootstrap.NewKlusterletManifestsConfig(
			mode,
			managedCluster.Name,
			bootstrapKubeconfigData).
			WithManagedCluster(managedCluster).
			WithoutImagePullSecretGenerate().
			// the hosting cluster should support PriorityClass API and have
			// already had the default PriorityClass
			WithPriorityClassName(constants.DefaultKlusterletPriorityClassName).
			WithKlusterletConfig(klusterletConfig).
			Generate(ctx, clientHolder)
		if err != nil {
			return nil, err
		}

		secretAnnotations = map[string]string{
			constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
		}
	default:
		return nil, fmt.Errorf("klusterlet deploy mode %s not supported", mode)
	}

	// generate import secret
	importSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.ImportSecretNameSuffix),
			Namespace: managedCluster.Name,
			Labels: map[string]string{
				constants.ClusterImportSecretLabel: "",
			},
			Annotations: secretAnnotations,
		},
		Data: map[string][]byte{
			constants.ImportSecretImportYamlKey: yamlcontent,
			constants.ImportSecretCRDSYamlKey:   crdsYAML,
		},
	}

	if len(tokenCreation) != 0 {
		importSecret.Data[constants.ImportSecretTokenCreation] = tokenCreation
	}
	if len(tokenExpiration) != 0 {
		importSecret.Data[constants.ImportSecretTokenExpiration] = tokenExpiration
	}
	return importSecret, nil
}
