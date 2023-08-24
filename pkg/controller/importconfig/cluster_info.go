// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const defaultKlusterletNamespace = "open-cluster-management-agent"

// getBootstrapKubeConfigDataFromImportSecret aims to reuse the bootstrap kubeconfig data if possible.
// The return values are: 1. kubeconfig data, 2. token expiration, 3. error
// Note that the kubeconfig data could be `nil` if the import secret is not found or the kubeconfig data is invalid.
func getBootstrapKubeConfigDataFromImportSecret(ctx context.Context, clientHolder *helpers.ClientHolder, clusterName string) ([]byte, []byte, error) {
	importSecret, err := getImportSecret(ctx, clientHolder, clusterName)
	if apierrors.IsNotFound(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	kubeConfigData := extractBootstrapKubeConfigDataFromImportSecret(importSecret)
	if len(kubeConfigData) == 0 {
		return nil, nil, nil
	}

	kubeAPIServer, caData, token, err := parseKubeConfigData(kubeConfigData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse kubeconfig data: %v", err)
	}

	// check if the kube apiserver address is changed
	validKubeAPIServer, err := validateKubeAPIServerAddress(ctx, kubeAPIServer, clientHolder)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate kube apiserver address: %v", err)
	}
	if !validKubeAPIServer {
		klog.Infof("KubeAPIServer invalid for the managed cluster %s, kubeAPIServer: %v", clusterName, kubeAPIServer)
		return nil, nil, nil
	}

	// check if the CA data is changed
	validCAData, err := validateCAData(ctx, caData, kubeAPIServer, clientHolder, clusterName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate CA data: %v", err)
	}
	if !validCAData {
		klog.Infof("CAdata is invalid for the managed cluster %s", clusterName)
		return nil, nil, nil
	}

	expiration := importSecret.Data[constants.ImportSecretTokenExpiration]
	if !validateToken(token, expiration) {
		klog.Infof("token is invalid for the managed cluster %s, expiration: %v", clusterName, string(expiration))
		return nil, nil, nil
	}

	return kubeConfigData, expiration, nil
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

	currentKubeAPIServer, err := bootstrap.GetKubeAPIServerAddress(ctx, clientHolder.RuntimeClient)
	if err != nil {
		return false, err
	}

	return kubeAPIServer == currentKubeAPIServer, nil
}

func validateCAData(ctx context.Context, caData []byte, kubeAPIServer string, clientHolder *helpers.ClientHolder, clusterName string) (bool, error) {
	if len(caData) == 0 {
		// CA data is empty
		return false, nil
	}

	currentCAData, err := bootstrap.GetBootstrapCAData(ctx, clientHolder, kubeAPIServer, clusterName)
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

func klusterletNamespace(managedClusterAnnotations map[string]string) string {
	if klusterletNamespace, ok := managedClusterAnnotations[constants.KlusterletNamespaceAnnotation]; ok {
		return klusterletNamespace
	}

	return defaultKlusterletNamespace
}
