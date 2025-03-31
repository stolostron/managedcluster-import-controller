// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"context"
	"os"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const EmptyImagePullSecret = "empty-image-pull-secret"

// getImagePullSecret get image pull secret from env
func getImagePullSecret(ctx context.Context, clientHolder *helpers.ClientHolder,
	kcImagePullSecret corev1.ObjectReference, clusterAnnotations map[string]string) (*corev1.Secret, error) {
	if kcImagePullSecret.Name != "" {
		secret, err := clientHolder.KubeClient.CoreV1().Secrets(kcImagePullSecret.Namespace).Get(ctx, kcImagePullSecret.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return secret, nil
	}

	secret, err := clientHolder.ImageRegistryClient.Cluster(clusterAnnotations).PullSecret()
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
	var secret *corev1.Secret

	defaultSecretName := os.Getenv(constants.DefaultImagePullSecretEnvVarName)
	if defaultSecretName == "" {
		// If default secret can't be found from env DEFAULT_IMAGE_PULL_SECRET, create an empty image pull secret
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: EmptyImagePullSecret,
			},
			Data: map[string][]byte{
				corev1.DockerConfigJsonKey: []byte("{}"),
			},
			Type: corev1.SecretTypeDockerConfigJson,
		}
	} else {
		ns := os.Getenv(constants.PodNamespaceEnvVarName)
		secret, err = clientHolder.KubeClient.CoreV1().Secrets(ns).Get(ctx, defaultSecretName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}

	return secret, nil
}
