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
)

// getBootstrapKubeConfigDataFromImportSecret aims to reuse the bootstrap kubeconfig data if possible.
// The return values are: 1. kubeconfig data, 2. token expiration, 3. error
// Note that the kubeconfig data could be `nil` if the import secret is not found or the kubeconfig data is invalid.
func getBootstrapKubeConfigDataFromImportSecret(ctx context.Context, clientHolder *helpers.ClientHolder, clusterName string,
	klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) ([]byte, []byte, []byte, error) {
	importSecret, err := getImportSecret(ctx, clientHolder, clusterName)
	if apierrors.IsNotFound(err) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}

	kubeConfigData := extractBootstrapKubeConfigDataFromImportSecret(importSecret)
	if len(kubeConfigData) == 0 {
		return nil, nil, nil, nil
	}

	kubeAPIServer, proxyURL, caData, token, ctxClusterName, err := parseKubeConfigData(kubeConfigData)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse kubeconfig data: %v", err)
	}

	valid, err := bootstrap.ValidateBootstrapKubeconfig(ctx, clientHolder, klusterletConfig, clusterName,
		kubeAPIServer, caData, proxyURL, ctxClusterName)
	if err != nil {
		return nil, nil, nil, err
	}
	if !valid {
		return nil, nil, nil, nil
	}

	creation := importSecret.Data[constants.ImportSecretTokenCreation]
	expiration := importSecret.Data[constants.ImportSecretTokenExpiration]
	if !validateToken(token, creation, expiration) {
		klog.Infof("token is invalid for the managed cluster %s, creation: %v, expiration: %v",
			clusterName, string(creation), string(expiration))
		return nil, nil, nil, nil
	}

	return kubeConfigData, creation, expiration, nil
}

func getImportSecret(ctx context.Context, clientHolder *helpers.ClientHolder, clusterName string) (*corev1.Secret, error) {
	importSecretName := fmt.Sprintf("%s-%s", clusterName, constants.ImportSecretNameSuffix)
	return clientHolder.KubeClient.CoreV1().Secrets(clusterName).Get(ctx, importSecretName, metav1.GetOptions{})
}

func extractBootstrapKubeConfigDataFromImportSecret(importSecret *corev1.Secret) []byte {
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
