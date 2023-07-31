package importconfig

import corev1 "k8s.io/api/core/v1"

type DefaultRenderConfig struct {
	KlusterletRenderConfig
	RegistrationOperatorImage string

	UseImagePullSecret       bool
	ImagePullSecretName      string
	ImagePullSecretData      string
	ImagePullSecretConfigKey string
	ImagePullSecretType      corev1.SecretType

	ClusterAnnotations map[string]string
}
