package autoimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func updateClusterURL(ctx context.Context, client client.Client,
	managedCluster *clusterv1.ManagedCluster, autoImportSecret *corev1.Secret) error {
	apiServerURL, err := getAPIServerURL(autoImportSecret)
	if err != nil {
		return err
	}

	if apiServerURL == "" {
		return nil
	}

	apiServerURL = strings.TrimSuffix(apiServerURL, "/")

	clusterCopy := managedCluster.DeepCopy()
	for _, config := range clusterCopy.Spec.ManagedClusterClientConfigs {
		configURL := strings.TrimSuffix(config.URL, "/")
		if configURL == apiServerURL {
			return nil
		}
	}

	clusterCopy.Spec.ManagedClusterClientConfigs = append(clusterCopy.Spec.ManagedClusterClientConfigs,
		clusterv1.ClientConfig{
			URL: apiServerURL,
		})

	klog.Infof("update the apiServerURL %v to the spec of managedCluster %v.", apiServerURL, clusterCopy.Name)
	return client.Update(ctx, clusterCopy)
}

func getAPIServerURL(secret *corev1.Secret) (string, error) {
	switch secret.Type {
	case corev1.SecretTypeOpaque:
		// for compatibility, we parse the secret fields to determine which generator should be used
		if kubeConfig, ok := secret.Data[constants.AutoImportSecretKubeConfigKey]; ok {
			return getAPISeverURLFromKubeConfig(kubeConfig)
		}

		// check token and server
		if kubeServer, ok := secret.Data[constants.AutoImportSecretKubeServerKey]; ok {
			return string(kubeServer), nil
		}
		return "", fmt.Errorf("secret %s/%s does not have an APIServer URL", secret.Namespace, secret.Name)

	case constants.AutoImportSecretKubeConfig:
		if kubeConfig, ok := secret.Data[constants.AutoImportSecretKubeConfigKey]; ok {
			return getAPISeverURLFromKubeConfig(kubeConfig)
		}
		return "", fmt.Errorf("secret %s/%s does not have an kubeConfig URL", secret.Namespace, secret.Name)

	case constants.AutoImportSecretKubeToken:
		if kubeServer, ok := secret.Data[constants.AutoImportSecretKubeServerKey]; ok {
			return string(kubeServer), nil
		}
		return "", fmt.Errorf("cannot get APIServer URL from secret %s/%s", secret.Namespace, secret.Name)

	case constants.AutoImportSecretRosaConfig:
		// TODO： need to call ocm api to get managed cluster api server URL.
		return "", nil

	default:
		return "", fmt.Errorf("unsupported secret type %s", secret.Type)
	}
}

func getAPISeverURLFromKubeConfig(kubeConfig []byte) (string, error) {
	config, err := clientcmd.Load(kubeConfig)
	if err != nil {
		return "", fmt.Errorf("failed to load kubeConfig: %v", err)
	}

	if config.CurrentContext == "" {
		return "", fmt.Errorf("currentContext in the kubeConfig is empty")
	}
	if config.Contexts[config.CurrentContext] == nil {
		return "", fmt.Errorf("currentContext not found in the kubeConfig")
	}

	currentClusterName := config.Contexts[config.CurrentContext].Cluster
	if currentClusterName == "" {
		return "", fmt.Errorf("the cluster in the currentContext not found in the kubeConfig")
	}

	cluster := config.Clusters[currentClusterName]
	if cluster == nil {
		return "", fmt.Errorf("the cluster in the currentContext not found in the kubeConfig")
	}
	return cluster.Server, nil
}
