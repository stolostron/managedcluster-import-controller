// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"fmt"
	"time"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		switch secret := obj.(type) {
		case *corev1.Secret:
			if secret.Name == constants.DefaultBootstrapHubKubeConfigSecretName {
				return secret.Data["kubeconfig"]
			}
		}
	}

	return nil
}

func parseKubeConfigData(kubeConfigData []byte) (
	kubeAPIServer, proxyURL string, caData []byte, token string, ctxClusterName string, err error) {

	config, err := clientcmd.Load(kubeConfigData)
	if err != nil {
		// kubeconfig data is invalid
		return "", "", nil, "", "", err
	}

	context := config.Contexts[config.CurrentContext]
	if context == nil {
		return "", "", nil, "", "", fmt.Errorf("failed to get current context")
	}

	if cluster, ok := config.Clusters[context.Cluster]; ok {
		ctxClusterName = context.Cluster
		kubeAPIServer = cluster.Server
		caData = cluster.CertificateAuthorityData
		proxyURL = cluster.ProxyURL
	}

	if authInfo, ok := config.AuthInfos["default-auth"]; ok {
		token = authInfo.Token
	}

	return
}

func validateToken(token string, creation, expiration []byte) bool {
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
	requiredKubeAPIServer, requiredProxyURL, requiredCAData, err := bootstrap.GetKubeAPIServerConfig(
		ctx, clientHolder, managedCluster.Name, klusterletConfig)
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
		kubeAPIServer, proxyURL, caData, tokenString, ctxClusterName, err := parseKubeConfigData(kubeconfigData)
		if err != nil {
			klog.Infof("failed to parse the bootstrap hub kubeconfig in the import.yaml. Recreation is required: %v", err)
		} else {
			// use the existing token if it is still valid
			creation := importSecret.Data[constants.ImportSecretTokenCreation]
			expiration := importSecret.Data[constants.ImportSecretTokenExpiration]
			if valid := validateToken(tokenString, creation, expiration); valid {
				tokenData = []byte(tokenString)
				tokenCreation = creation
				tokenExpiration = expiration
			} else {
				klog.Infof("token should be refreshed for the managed cluster %s, creation: %v, expiration: %v",
					managedCluster.Name, string(creation), string(expiration))
			}

			// use the kubeconfig if it is still valid
			if valid := bootstrap.ValidateBootstrapKubeconfig(managedCluster.Name,
				kubeAPIServer, proxyURL, caData, ctxClusterName,
				requiredKubeAPIServer, requiredProxyURL, requiredCAData, requiredCtxClusterName); valid {
				bootstrapKubeconfigData = kubeconfigData
			}
		}
	}

	// retrieve the non-expiring token if available or generate a new one.
	if len(tokenData) == 0 {
		klog.Infof("create a new token for the managed cluster %s", managedCluster.Name)
		tokenData, tokenCreation, tokenExpiration, err = bootstrap.GetBootstrapToken(ctx, clientHolder.KubeClient,
			bootstrap.GetBootstrapSAName(managedCluster.Name),
			managedCluster.Name, constants.DefaultSecretTokenExpirationSecond)
		if err != nil {
			return nil, nil, nil, err
		}

		// reset the bootstrap kubeconfig to trigger the re since the token is updated
		bootstrapKubeconfigData = nil
	}

	// create a new bootstrap kubeconfig if it is invalid or missing
	if len(bootstrapKubeconfigData) == 0 {
		klog.Infof("create a new bootstrap kubeconfig for the managed cluster %s", managedCluster.Name)
		bootstrapKubeconfigData, err = bootstrap.CreateBootstrapKubeConfig(requiredCtxClusterName,
			requiredKubeAPIServer, requiredProxyURL, requiredCAData, tokenData)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return bootstrapKubeconfigData, tokenCreation, tokenExpiration, nil
}

func buildImportSecret(ctx context.Context, clientHolder *helpers.ClientHolder, managedCluster *clusterv1.ManagedCluster,
	mode operatorv1.InstallMode, klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig,
	bootstrapKubeconfigData, tokenCreation, tokenExpiration []byte) (*corev1.Secret, error) {
	var yamlcontent, crdsV1YAML, crdsV1beta1YAML []byte
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
		yamlcontent, err = config.Generate(ctx, clientHolder)
		if err != nil {
			return nil, err
		}

		crdsV1beta1YAML, err = config.GenerateKlusterletCRDsV1Beta1()
		if err != nil {
			return nil, err
		}

		crdsV1YAML, err = config.GenerateKlusterletCRDsV1()
		if err != nil {
			return nil, err
		}
	case operatorv1.InstallModeHosted, operatorv1.InstallModeSingletonHosted:
		yamlcontent, err = bootstrap.NewKlusterletManifestsConfig(
			mode,
			managedCluster.Name,
			bootstrapKubeconfigData).
			WithManagedCluster(managedCluster).
			WithImagePullSecretGenerate(false).
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
			constants.ImportSecretImportYamlKey:      yamlcontent,
			constants.ImportSecretCRDSYamlKey:        crdsV1YAML,
			constants.ImportSecretCRDSV1YamlKey:      crdsV1YAML,
			constants.ImportSecretCRDSV1beta1YamlKey: crdsV1beta1YAML,
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
