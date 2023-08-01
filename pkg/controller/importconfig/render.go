package importconfig

import (
	corev1 "k8s.io/api/core/v1"
)

type DefaultRenderConfig struct {
	KlusterletRenderConfig
	RegistrationOperatorImage string
	ImagePullSecretConfig
}

// KlusterletRenderConfig defines variables used in the klusterletFiles.
type KlusterletRenderConfig struct {
	KlusterletNamespace     string
	ManagedClusterNamespace string
	BootstrapKubeconfig     string
	RegistrationImageName   string
	WorkImageName           string
	NodeSelector            map[string]string
	Tolerations             []corev1.Toleration
	InstallMode             string
	ClusterAnnotations      map[string]string
}

type ImagePullSecretConfig struct {
	UseImagePullSecret       bool
	ImagePullSecretName      string
	ImagePullSecretData      string
	ImagePullSecretConfigKey string
	ImagePullSecretType      corev1.SecretType
}

const (
	AgentRegistrationDefaultBootstrapSAName = "agent-registration-bootstrap"
)
