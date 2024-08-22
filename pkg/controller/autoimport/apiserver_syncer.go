package autoimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	kevents "k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type apiServerSyncer struct {
	client       client.Client
	recorder     events.Recorder
	mcRecorder   kevents.EventRecorder
	importHelper *helpers.ImportHelper
}

func (s *apiServerSyncer) sync(ctx context.Context,
	managedCluster *clusterv1.ManagedCluster, autoImportSecret *corev1.Secret) (reconcile.Result, error) {

	apiServerURL, err := s.getAPIServerURL(autoImportSecret)
	if err != nil {
		return reconcile.Result{}, err
	}

	if apiServerURL == "" {
		return reconcile.Result{}, nil
	}

	apiServerURL = strings.TrimSuffix(apiServerURL, "/")

	clusterCopy := managedCluster.DeepCopy()
	for _, config := range clusterCopy.Spec.ManagedClusterClientConfigs {
		if config.URL == apiServerURL {
			return reconcile.Result{}, nil
		}
	}

	clusterCopy.Spec.ManagedClusterClientConfigs = append(clusterCopy.Spec.ManagedClusterClientConfigs,
		clusterv1.ClientConfig{
			URL: apiServerURL,
		})

	klog.Infof("update the apiServerURL %v to the spec of managedCluster %v.", apiServerURL, clusterCopy.Name)
	return reconcile.Result{}, s.client.Update(ctx, clusterCopy)
}

func (s *apiServerSyncer) getAPIServerURL(secret *corev1.Secret) (string, error) {
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
