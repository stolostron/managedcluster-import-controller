// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

// getImagePullSecret get image pull secret from env
func getImagePullSecret(ctx context.Context, clientHolder *helpers.ClientHolder, clusterAnnotations map[string]string) (*corev1.Secret, error) {
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

	defaultSecretName := os.Getenv(constants.DefaultImagePullSecretEnvVarName)
	if defaultSecretName == "" {
		// Ignore the image pull secret, it can't be found from env DEFAULT_IMAGE_PULL_SECRET
		return nil, nil
	}

	ns := os.Getenv(constants.PodNamespaceEnvVarName)
	secret, err := clientHolder.KubeClient.CoreV1().Secrets(ns).Get(ctx, defaultSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}
