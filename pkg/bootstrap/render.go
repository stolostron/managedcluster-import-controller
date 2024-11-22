// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/stolostron/cluster-lifecycle-api/helpers/localcluster"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	apifeature "open-cluster-management.io/api/feature"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

//go:embed manifests
var ManifestFiles embed.FS

const managedClusterImagePullSecretName = "open-cluster-management-image-pull-credentials"

const (
	klusterletCrdsV1File      = "manifests/klusterlet/crds/klusterlets.crd.v1.yaml"
	klusterletCrdsV1beta1File = "manifests/klusterlet/crds/klusterlets.crd.v1beta1.yaml"
)

var hubFiles = []string{
	"manifests/hub/managedcluster-service-account.yaml",
	"manifests/hub/managedcluster-clusterrole.yaml",
	"manifests/hub/managedcluster-clusterrolebinding.yaml",
}

var klusterletNamespaceFile = "manifests/klusterlet/namespace.yaml"

var klusterletOperatorFiles = []string{
	"manifests/klusterlet/service_account.yaml",
	"manifests/klusterlet/cluster_role.yaml",
	"manifests/klusterlet/clusterrole_bootstrap.yaml",
	"manifests/klusterlet/clusterrole_aggregate.yaml",
	"manifests/klusterlet/cluster_role_binding.yaml",
	"manifests/klusterlet/operator.yaml",
}

var klusterletFiles = []string{
	"manifests/klusterlet/bootstrap_secret.yaml",
	"manifests/klusterlet/klusterlet.yaml",
}

// TODO: The clusterrole_priority_class.yaml is only for upgrade case and should be removed
// after two or three releases.
var priorityClassFiles = []string{
	"manifests/klusterlet/clusterrole_priority_class.yaml",
	"manifests/klusterlet/priority_class.yaml",
}

type RenderConfig struct {
	KlusterletRenderConfig
	ImagePullSecretConfig
}

type BootstrapKubeConfigSecret struct {
	Name       string
	KubeConfig string
}

// KlusterletRenderConfig defines variables used in the klusterletFiles.
type KlusterletRenderConfig struct {
	KlusterletName            string
	KlusterletNamespace       string
	ManagedClusterNamespace   string
	RegistrationOperatorImage string
	RegistrationImageName     string
	WorkImageName             string
	ImageName                 string
	PriorityClassName         string
	InstallMode               string

	NodeSelector  map[string]string
	Tolerations   []corev1.Toleration
	NodePlacement *operatorv1.NodePlacement

	RegistrationConfiguration *operatorv1.RegistrationConfiguration
	WorkConfiguration         *operatorv1.WorkAgentConfiguration

	MultipleHubsEnabled              bool
	DefaultBootstrapKubeConfigSecret BootstrapKubeConfigSecret
	BootstrapKubeConfigSecrets       []BootstrapKubeConfigSecret
}

type ImagePullSecretConfig struct {
	UseImagePullSecret       bool
	ImagePullSecretName      string
	ImagePullSecretData      string
	ImagePullSecretConfigKey string
	ImagePullSecretType      corev1.SecretType
}

type KlusterletManifestsConfig struct {
	InstallMode operatorv1.InstallMode

	ClusterName                  string
	KlusterletNamespace          string
	KlusterletClusterAnnotations map[string]string
	BootstrapKubeconfig          []byte

	// PriorityClassName is the name of the PriorityClass used by the klusterlet and operator
	PriorityClassName string

	// Used to determine whether mc is a localcluster.
	ManagedCluster *clusterv1.ManagedCluster

	klusterletconfig *klusterletconfigv1alpha1.KlusterletConfig

	generateImagePullSecret bool // by default is true, in hosted mode, it will be set false
}

func NewKlusterletManifestsConfig(installMode operatorv1.InstallMode,
	clusterName string, bootstrapKubeconfig []byte) *KlusterletManifestsConfig {
	return &KlusterletManifestsConfig{
		InstallMode:             installMode,
		ClusterName:             clusterName,
		BootstrapKubeconfig:     bootstrapKubeconfig,
		generateImagePullSecret: true,
	}
}

// WithKlusterletClusterAnnotations sets the klusterlet cluster annotations(klusterlet.spec.registrationConfiguration.clusterAnnotations).
// These annotations must begin with a prefix "agent.open-cluster-management.io*".
func (c *KlusterletManifestsConfig) WithKlusterletClusterAnnotations(a map[string]string) *KlusterletManifestsConfig {
	c.KlusterletClusterAnnotations = a
	return c
}

// WithManagedClusterLabels sets the managed cluster.
func (c *KlusterletManifestsConfig) WithManagedCluster(mc *clusterv1.ManagedCluster) *KlusterletManifestsConfig {
	c.ManagedCluster = mc
	return c
}

func (c *KlusterletManifestsConfig) WithKlusterletConfig(klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) *KlusterletManifestsConfig {
	c.klusterletconfig = klusterletConfig
	return c
}

func (c *KlusterletManifestsConfig) WithImagePullSecretGenerate(g bool) *KlusterletManifestsConfig {
	c.generateImagePullSecret = g
	return c
}

func (c *KlusterletManifestsConfig) WithPriorityClassName(priorityClassName string) *KlusterletManifestsConfig {
	c.PriorityClassName = priorityClassName
	return c
}

// Generate returns the rendered klusterlet manifests in bytes.
func (b *KlusterletManifestsConfig) Generate(ctx context.Context, clientHolder *helpers.ClientHolder) ([]byte, error) {
	// Files depends on the install mode
	var files []string
	switch b.InstallMode {
	case operatorv1.InstallModeHosted, operatorv1.InstallModeSingletonHosted:
		files = append(files, klusterletFiles...)
	case operatorv1.InstallModeDefault, operatorv1.InstallModeSingleton:
		if b.PriorityClassName == constants.DefaultKlusterletPriorityClassName {
			files = append(files, priorityClassFiles...)
		}
		files = append(files, klusterletNamespaceFile)
		if !installNoOperator(b.InstallMode, b.klusterletconfig) {
			files = append(files, klusterletOperatorFiles...)
		}
		files = append(files, klusterletFiles...)
	default:
		return nil, fmt.Errorf("invalid install mode: %s", b.InstallMode)
	}

	// For image, image pull secret, nodeplacement, we use configurations in klusterletconfg over configurations in managed cluster annotations.
	var kcRegistries []klusterletconfigv1alpha1.Registries
	var kcNodePlacement *operatorv1.NodePlacement
	var kcImagePullSecret corev1.ObjectReference
	var appliedManifestWorkEvictionGracePeriod string
	if b.klusterletconfig != nil {
		if b.InstallMode == operatorv1.InstallModeDefault || b.InstallMode == operatorv1.InstallModeSingleton {
			kcRegistries = b.klusterletconfig.Spec.Registries
			kcNodePlacement = b.klusterletconfig.Spec.NodePlacement
			kcImagePullSecret = b.klusterletconfig.Spec.PullSecret
		}
		appliedManifestWorkEvictionGracePeriod = b.klusterletconfig.Spec.AppliedManifestWorkEvictionGracePeriod
	}

	var managedClusterAnnotations map[string]string
	if b.ManagedCluster != nil {
		managedClusterAnnotations = b.ManagedCluster.GetAnnotations()
	}

	var localCluster bool
	if b.ManagedCluster != nil && localcluster.IsClusterSelfManaged(b.ManagedCluster) {
		localCluster = true
	}

	// Images override
	registrationOperatorImageName, err := getImage(constants.RegistrationOperatorImageEnvVarName,
		kcRegistries, managedClusterAnnotations)
	if err != nil {
		return nil, err
	}

	registrationImageName, err := getImage(constants.RegistrationImageEnvVarName,
		kcRegistries, managedClusterAnnotations)
	if err != nil {
		return nil, err
	}

	workImageName, err := getImage(constants.WorkImageEnvVarName,
		kcRegistries, managedClusterAnnotations)
	if err != nil {
		return nil, err
	}

	// NodeSelector
	var nodeSelector map[string]string
	if kcNodePlacement != nil && len(kcNodePlacement.NodeSelector) != 0 {
		nodeSelector = kcNodePlacement.NodeSelector
	} else {
		nodeSelector, err = helpers.GetNodeSelectorFromManagedClusterAnnotations(managedClusterAnnotations)
		if err != nil {
			return nil, fmt.Errorf("Get nodeSelector for cluster %s failed: %v", b.ClusterName, err)
		}
	}
	if err := helpers.ValidateNodeSelector(nodeSelector); err != nil {
		return nil, fmt.Errorf("invalid nodeSelector annotation %v", err)
	}

	// Tolerations
	var tolerations []corev1.Toleration
	if kcNodePlacement != nil && len(kcNodePlacement.Tolerations) != 0 {
		tolerations = kcNodePlacement.Tolerations
	} else {
		tolerations, err = helpers.GetTolerationsFromManagedClusterAnnotations(managedClusterAnnotations)
		if err != nil {
			return nil, fmt.Errorf("Get tolerations for cluster %s failed: %v", b.ClusterName, err)
		}
	}
	if err := helpers.ValidateTolerations(tolerations); err != nil {
		return nil, fmt.Errorf("invalid tolerations annotation %v", err)
	}

	klusterletName, klusterletNamespace := getKlusterletNamespaceName(
		b.klusterletconfig, b.ClusterName, managedClusterAnnotations, b.InstallMode)

	// WorkAgentConfiguration
	workAgentConfiguration := &operatorv1.WorkAgentConfiguration{}
	if appliedManifestWorkEvictionGracePeriod == constants.AppliedManifestWorkEvictionGracePeriodInfinite {
		appliedManifestWorkEvictionGracePeriod = constants.AppliedManifestWorkEvictionGracePeriod100Years
	}
	if appliedManifestWorkEvictionGracePeriod != "" {
		appliedManifestWorkEvictionGracePeriodTimeDuration, err := time.ParseDuration(appliedManifestWorkEvictionGracePeriod)
		if err != nil {
			return nil, fmt.Errorf("parse appliedManifestWorkEvictionGracePeriod %s failed: %v",
				appliedManifestWorkEvictionGracePeriod, err)
		}
		workAgentConfiguration.AppliedManifestWorkEvictionGracePeriod = &metav1.Duration{
			Duration: appliedManifestWorkEvictionGracePeriodTimeDuration,
		}
	}

	// RegistrationConfiguration
	registrationConfiguration := &operatorv1.RegistrationConfiguration{
		ClusterAnnotations: b.KlusterletClusterAnnotations,
	}

	renderConfig := RenderConfig{
		KlusterletRenderConfig: KlusterletRenderConfig{
			ManagedClusterNamespace: b.ClusterName,
			KlusterletName:          klusterletName,
			KlusterletNamespace:     klusterletNamespace,
			InstallMode:             string(b.InstallMode),

			// Images
			RegistrationOperatorImage: registrationOperatorImageName,
			RegistrationImageName:     registrationImageName,
			WorkImageName:             workImageName,
			ImageName:                 registrationOperatorImageName,
			DefaultBootstrapKubeConfigSecret: BootstrapKubeConfigSecret{
				Name:       constants.DefaultBootstrapHubKubeConfigSecretName,
				KubeConfig: base64.StdEncoding.EncodeToString(b.BootstrapKubeconfig),
			},

			// PriorityClassName
			PriorityClassName: b.PriorityClassName,

			// NodeSelector and Tolerations used in operator
			NodeSelector: nodeSelector,
			Tolerations:  tolerations,

			// NodePlacement used in klusterlet
			NodePlacement: &operatorv1.NodePlacement{
				NodeSelector: nodeSelector,
				Tolerations:  tolerations,
			},

			// WorkAgetnConfiguration
			WorkConfiguration: workAgentConfiguration,

			// RegistrationConfiguration
			RegistrationConfiguration: registrationConfiguration,
		},
	}

	// If need to generate imagePullSecret
	if b.generateImagePullSecret {
		// Image pull secret, need to add `manifests/klusterlet/image_pull_secret.yaml` to files if imagePullSecret is not nil

		imagePullSecret, err := getImagePullSecret(ctx, clientHolder, kcImagePullSecret, managedClusterAnnotations)
		if err != nil {
			return nil, err
		}

		if imagePullSecret == nil {
			return nil, fmt.Errorf("imagePullSecret is nil")
		}
		files = append(files, "manifests/klusterlet/image_pull_secret.yaml")

		imagePullSecretConfig, err := getImagePullSecretConfig(imagePullSecret)
		if err != nil {
			return nil, err
		}

		renderConfig.ImagePullSecretConfig = imagePullSecretConfig
	}

	// MultipleHubs
	// Doesn't affect on the local-cluster.
	// Using MultipleHubs can controls the bootstrap kubeconfig secret/secrets easier.
	if !localCluster &&
		b.klusterletconfig != nil &&
		b.klusterletconfig.Spec.BootstrapKubeConfigs.Type == operatorv1.LocalSecrets {

		registrationConfiguration.FeatureGates = append(registrationConfiguration.FeatureGates,
			operatorv1.FeatureGate{
				Feature: string(apifeature.MultipleHubs),
				Mode:    operatorv1.FeatureGateModeTypeEnable,
			})
		registrationConfiguration.BootstrapKubeConfigs = b.klusterletconfig.Spec.BootstrapKubeConfigs
		registrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets = append(
			registrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets, operatorv1.KubeConfigSecret{
				Name: constants.DefaultBootstrapHubKubeConfigSecretName + "-current-hub",
			})

		bootstrapKubeConfigSecrets, err := convertKubeConfigSecrets(ctx,
			b.klusterletconfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets, clientHolder.KubeClient)
		if err != nil {
			return nil, err
		}
		// add default bootstrap kubeconfig secret into the list:
		// TODO: deduplicate the bootstrap kubeconfig secrets @xuezhaojun
		bootstrapKubeConfigSecrets = append(bootstrapKubeConfigSecrets, BootstrapKubeConfigSecret{
			Name:       constants.DefaultBootstrapHubKubeConfigSecretName + "-current-hub",
			KubeConfig: base64.StdEncoding.EncodeToString(b.BootstrapKubeconfig),
		})

		renderConfig.MultipleHubsEnabled = true
		renderConfig.BootstrapKubeConfigSecrets = bootstrapKubeConfigSecrets
	}

	// Render the klusterlet manifests
	manifestsBytes, err := filesToTemplateBytes(files, renderConfig)
	if err != nil {
		return nil, err
	}

	return manifestsBytes, nil
}

func convertKubeConfigSecrets(ctx context.Context,
	kcs []operatorv1.KubeConfigSecret, kubeClient kubernetes.Interface) ([]BootstrapKubeConfigSecret, error) {
	var bootstrapKubeConfigSecrets []BootstrapKubeConfigSecret
	for _, s := range kcs {
		ns := os.Getenv(constants.PodNamespaceEnvVarName)
		secret, err := kubeClient.CoreV1().Secrets(ns).Get(ctx, s.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		// check whether 'kubeconfig' key exists in the secret
		if _, ok := secret.Data["kubeconfig"]; !ok {
			return nil, fmt.Errorf("kubeconfig key not found in secret %s", s.Name)
		}

		bootstrapKubeConfigSecrets = append(bootstrapKubeConfigSecrets, BootstrapKubeConfigSecret{
			Name:       s.Name,
			KubeConfig: base64.StdEncoding.EncodeToString(secret.Data["kubeconfig"]),
		})
	}

	return bootstrapKubeConfigSecrets, nil
}

func (b *KlusterletManifestsConfig) GenerateKlusterletCRDsV1() ([]byte, error) {
	if installNoOperator(b.InstallMode, b.klusterletconfig) {
		return []byte{}, nil
	}
	return filesToTemplateBytes([]string{klusterletCrdsV1File}, nil)
}

func (b *KlusterletManifestsConfig) GenerateKlusterletCRDsV1Beta1() ([]byte, error) {
	if installNoOperator(b.InstallMode, b.klusterletconfig) {
		return []byte{}, nil
	}
	return filesToTemplateBytes([]string{klusterletCrdsV1beta1File}, nil)
}

func GenerateHubBootstrapRBACObjects(managedClusterName string) ([]runtime.Object, error) {
	return helpers.FilesToObjects(hubFiles, struct {
		ManagedClusterName          string
		ManagedClusterNamespace     string
		BootstrapServiceAccountName string
	}{
		ManagedClusterName:          managedClusterName,
		ManagedClusterNamespace:     managedClusterName,
		BootstrapServiceAccountName: GetBootstrapSAName(managedClusterName),
	}, &ManifestFiles)
}

func filesToTemplateBytes(files []string, config interface{}) ([]byte, error) {
	manifests := new(bytes.Buffer)
	for _, file := range files {
		b, err := ManifestFiles.ReadFile(file)
		if err != nil {
			return nil, err
		}

		if config != nil {
			b = helpers.MustCreateAssetFromTemplate(file, b, config)
		}
		manifests.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(b)))
	}
	return manifests.Bytes(), nil
}

// installNoOperator return true if operator is not to be installed.
func installNoOperator(mode operatorv1.InstallMode, config *klusterletconfigv1alpha1.KlusterletConfig) bool {
	if mode == operatorv1.InstallModeHosted || mode == operatorv1.InstallModeSingletonHosted {
		return true
	}
	if config == nil || config.Spec.InstallMode == nil {
		return false
	}
	if config.Spec.InstallMode.Type == klusterletconfigv1alpha1.InstallModeNoOperator {
		return true
	}
	return false
}

// getKlusterletName returns klusterlet by default, and klusterlet-{cluster name} in hosted mode,
// and klusterlet-{postfix} if install mode in config is set with postfix.
func getKlusterletNamespaceName(
	config *klusterletconfigv1alpha1.KlusterletConfig,
	clusterName string, annotation map[string]string, mode operatorv1.InstallMode) (string, string) {
	klusterletName := constants.KlusterletSuffix
	klusterletNamespace := constants.DefaultKlusterletNamespace
	if mode == operatorv1.InstallModeHosted || mode == operatorv1.InstallModeSingletonHosted {
		klusterletName = fmt.Sprintf("%s-%s", constants.KlusterletSuffix, clusterName)
		klusterletNamespace = fmt.Sprintf("open-cluster-management-%s", clusterName)
		if len(klusterletNamespace) > 57 {
			klusterletNamespace = klusterletNamespace[:57]
		}
	}

	if v, ok := annotation[constants.KlusterletNamespaceAnnotation]; ok {
		klusterletNamespace = v
	}

	if config == nil || config.Spec.InstallMode == nil {
		return klusterletName, klusterletNamespace
	}
	if config.Spec.InstallMode.Type != klusterletconfigv1alpha1.InstallModeNoOperator {
		return klusterletName, klusterletNamespace
	}
	if config.Spec.InstallMode.NoOperator == nil {
		return klusterletName, klusterletNamespace
	}

	klusterletName = fmt.Sprintf("%s-%s", klusterletName, config.Spec.InstallMode.NoOperator.Postfix)
	klusterletNamespace = fmt.Sprintf("open-cluster-management-%s", config.Spec.InstallMode.NoOperator.Postfix)

	return klusterletName, klusterletNamespace
}

func getImage(envName string, kcRegistries []klusterletconfigv1alpha1.Registries, clusterAnnotations map[string]string) (string, error) {
	defaultImage := os.Getenv(envName)
	if defaultImage == "" {
		return "", fmt.Errorf("environment variable %s not defined", envName)
	}

	if len(kcRegistries) != 0 {
		overrideImageName := defaultImage
		for i := 0; i < len(kcRegistries); i++ {
			registry := kcRegistries[i]
			name := imageOverride(registry.Source, registry.Mirror, defaultImage)
			if name != defaultImage {
				overrideImageName = name
			}
		}
		return overrideImageName, nil
	}

	return imageregistry.OverrideImageByAnnotation(clusterAnnotations, defaultImage)
}

// imageOverride is a copy from /pkg/helpers/imageregistry/client.go
func imageOverride(source, mirror, imageName string) string {
	source = strings.TrimSuffix(source, "/")
	mirror = strings.TrimSuffix(mirror, "/")
	imageSegments := strings.Split(imageName, "/")
	imageNameTag := imageSegments[len(imageSegments)-1]
	if source == "" {
		if mirror == "" {
			return imageNameTag
		}
		return fmt.Sprintf("%s/%s", mirror, imageNameTag)
	}

	if !strings.HasPrefix(imageName, source) {
		return imageName
	}

	trimSegment := strings.TrimPrefix(imageName, source)
	return fmt.Sprintf("%s%s", mirror, trimSegment)
}
