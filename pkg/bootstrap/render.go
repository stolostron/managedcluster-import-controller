// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/stolostron/cluster-lifecycle-api/helpers/localcluster"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	apifeature "open-cluster-management.io/api/feature"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"open-cluster-management.io/ocm/pkg/operator/helpers/chart"
	"sigs.k8s.io/yaml"
)

//go:embed manifests
var ManifestFiles embed.FS

var hubFiles = []string{
	"manifests/hub/managedcluster-service-account.yaml",
	"manifests/hub/managedcluster-clusterrole.yaml",
	"manifests/hub/managedcluster-clusterrolebinding.yaml",
}

var additionalClusterRoleFiles = []string{
	"manifests/klusterlet/clusterrole_bootstrap.yaml",
	"manifests/klusterlet/clusterrole_aggregate.yaml",
}

type BootstrapKubeConfigSecret struct {
	Name       string
	KubeConfig string
}

type KlusterletManifestsConfig struct {
	chartConfig      *chart.KlusterletChartConfig
	managedCluster   *clusterv1.ManagedCluster
	klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
}

func NewKlusterletManifestsConfig(installMode operatorv1.InstallMode,
	clusterName string, bootstrapKubeConfig []byte) *KlusterletManifestsConfig {
	return &KlusterletManifestsConfig{
		chartConfig: newKlusterletChartConfig(installMode, clusterName, bootstrapKubeConfig),
	}
}

func newKlusterletChartConfig(installMode operatorv1.InstallMode,
	clusterName string, bootstrapKubeConfig []byte) *chart.KlusterletChartConfig {
	allowPrivilegeEscalation := false
	privileged := false
	runAsNonRoot := true
	readOnlyRootFilesystem := true

	chartConfig := &chart.KlusterletChartConfig{
		ReplicaCount: 1,
		Images: chart.ImagesConfig{
			ImagePullPolicy: corev1.PullIfNotPresent,
			ImageCredentials: chart.ImageCredentials{
				// set true by default
				CreateImageCredentials: true,
			},
		},
		CreateNamespace: true,
		PodSecurityContext: corev1.PodSecurityContext{
			RunAsNonRoot: &runAsNonRoot,
		},
		SecurityContext: corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			Privileged: &privileged,

			RunAsNonRoot:             &runAsNonRoot,
			ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
		},
		NodeSelector: map[string]string{},
		Tolerations:  []corev1.Toleration{},
		Affinity:     corev1.Affinity{},
		Klusterlet: chart.KlusterletConfig{
			ClusterName: clusterName,
			Mode:        installMode,
		},
		PriorityClassName:         "",
		EnableSyncLabels:          false,
		BootstrapHubKubeConfig:    string(bootstrapKubeConfig),
		ExternalManagedKubeConfig: "",
		NoOperator:                false,
	}

	if chartConfig.Klusterlet.Mode == operatorv1.InstallModeHosted ||
		chartConfig.Klusterlet.Mode == operatorv1.InstallModeSingletonHosted {
		chartConfig.CreateNamespace = false
	}

	return chartConfig
}

// WithKlusterletClusterAnnotations sets the klusterlet cluster annotations(klusterlet.spec.registrationConfiguration.clusterAnnotations).
// These annotations must begin with a prefix "agent.open-cluster-management.io*".
func (c *KlusterletManifestsConfig) WithKlusterletClusterAnnotations(a map[string]string) *KlusterletManifestsConfig {
	c.chartConfig.Klusterlet.RegistrationConfiguration.ClusterAnnotations = a
	return c
}

// WithManagedCluster sets the managed cluster.
func (c *KlusterletManifestsConfig) WithManagedCluster(mc *clusterv1.ManagedCluster) *KlusterletManifestsConfig {
	c.managedCluster = mc
	return c
}

func (c *KlusterletManifestsConfig) WithKlusterletConfig(klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig) *KlusterletManifestsConfig {
	c.klusterletConfig = klusterletConfig
	return c
}

func (c *KlusterletManifestsConfig) WithoutImagePullSecretGenerate() *KlusterletManifestsConfig {
	c.chartConfig.Images.ImageCredentials.CreateImageCredentials = false
	return c
}

func (c *KlusterletManifestsConfig) WithPriorityClassName(priorityClassName string) *KlusterletManifestsConfig {
	c.chartConfig.PriorityClassName = priorityClassName
	return c
}

// Generate returns the rendered klusterlet manifests in bytes. return manifests, crd, error.
func (c *KlusterletManifestsConfig) Generate(ctx context.Context,
	clientHolder *helpers.ClientHolder) ([]byte, []byte, error) {
	installMode := c.chartConfig.Klusterlet.Mode
	clusterName := c.chartConfig.Klusterlet.ClusterName

	// For image, image pull secret, nodePlacement, we use configurations in klusterletConfig over
	// configurations in managed cluster annotations.
	var kcRegistries []klusterletconfigv1alpha1.Registries
	var kcNodePlacement *operatorv1.NodePlacement
	var kcImagePullSecret corev1.ObjectReference
	var appliedManifestWorkEvictionGracePeriod string

	switch installMode {
	case operatorv1.InstallModeHosted, operatorv1.InstallModeSingletonHosted:
		// do nothing
	case operatorv1.InstallModeDefault, operatorv1.InstallModeSingleton:
		if c.klusterletConfig != nil {
			kcRegistries = c.klusterletConfig.Spec.Registries
			kcNodePlacement = c.klusterletConfig.Spec.NodePlacement
			kcImagePullSecret = c.klusterletConfig.Spec.PullSecret
			appliedManifestWorkEvictionGracePeriod = c.klusterletConfig.Spec.AppliedManifestWorkEvictionGracePeriod
		}
	default:
		return nil, nil, fmt.Errorf("invalid install mode: %s", installMode)
	}

	c.chartConfig.NoOperator = installNoOperator(installMode, c.klusterletConfig)

	var managedClusterAnnotations map[string]string
	if c.managedCluster != nil {
		managedClusterAnnotations = c.managedCluster.GetAnnotations()
	}

	// Images override
	klusterletAgentImages, err := getKlusterletAgentImages(kcRegistries, managedClusterAnnotations)
	if err != nil {
		return nil, nil, err
	}
	c.chartConfig.Images.Overrides.OperatorImage = klusterletAgentImages[constants.RegistrationOperatorImageEnvVarName]
	c.chartConfig.Images.Overrides.RegistrationImage = klusterletAgentImages[constants.RegistrationImageEnvVarName]
	c.chartConfig.Images.Overrides.WorkImage = klusterletAgentImages[constants.WorkImageEnvVarName]

	// NodeSelector
	var nodeSelector map[string]string
	if kcNodePlacement != nil && len(kcNodePlacement.NodeSelector) != 0 {
		nodeSelector = kcNodePlacement.NodeSelector
	} else {
		nodeSelector, err = helpers.GetNodeSelectorFromManagedClusterAnnotations(managedClusterAnnotations)
		if err != nil {
			return nil, nil, fmt.Errorf("get nodeSelector for cluster %s failed: %v", clusterName, err)
		}
	}
	if err := helpers.ValidateNodeSelector(nodeSelector); err != nil {
		return nil, nil, fmt.Errorf("invalid nodeSelector annotation %v", err)
	}
	c.chartConfig.NodeSelector = nodeSelector
	c.chartConfig.Klusterlet.NodePlacement.NodeSelector = nodeSelector

	// Tolerations
	var tolerations []corev1.Toleration
	if kcNodePlacement != nil && len(kcNodePlacement.Tolerations) != 0 {
		tolerations = kcNodePlacement.Tolerations
	} else {
		tolerations, err = helpers.GetTolerationsFromManagedClusterAnnotations(managedClusterAnnotations)
		if err != nil {
			return nil, nil, fmt.Errorf("get tolerations for cluster %s failed: %v", clusterName, err)
		}
	}
	if err := helpers.ValidateTolerations(tolerations); err != nil {
		return nil, nil, fmt.Errorf("invalid tolerations annotation %v", err)
	}
	c.chartConfig.Tolerations = tolerations
	c.chartConfig.Klusterlet.NodePlacement.Tolerations = tolerations

	c.chartConfig.Klusterlet.Name, c.chartConfig.Klusterlet.Namespace = getKlusterletNamespaceName(
		c.klusterletConfig, clusterName, managedClusterAnnotations, installMode)

	// WorkAgentConfiguration
	workAgentConfiguration := operatorv1.WorkAgentConfiguration{}
	if appliedManifestWorkEvictionGracePeriod == constants.AppliedManifestWorkEvictionGracePeriodInfinite {
		appliedManifestWorkEvictionGracePeriod = constants.AppliedManifestWorkEvictionGracePeriod100Years
	}
	if appliedManifestWorkEvictionGracePeriod != "" {
		appliedManifestWorkEvictionGracePeriodTimeDuration, err := time.ParseDuration(appliedManifestWorkEvictionGracePeriod)
		if err != nil {
			return nil, nil, fmt.Errorf("parse appliedManifestWorkEvictionGracePeriod %s failed: %v",
				appliedManifestWorkEvictionGracePeriod, err)
		}
		workAgentConfiguration.AppliedManifestWorkEvictionGracePeriod = &metav1.Duration{
			Duration: appliedManifestWorkEvictionGracePeriodTimeDuration,
		}
	}
	c.chartConfig.Klusterlet.WorkConfiguration = workAgentConfiguration

	// need to generate imagePullSecret
	if c.chartConfig.Images.ImageCredentials.CreateImageCredentials {
		imagePullSecret, err := getImagePullSecret(ctx, clientHolder, kcImagePullSecret, managedClusterAnnotations)
		if err != nil {
			return nil, nil, err
		}

		if imagePullSecret == nil {
			return nil, nil, fmt.Errorf("imagePullSecret is nil")
		}
		if len(imagePullSecret.Data[corev1.DockerConfigJsonKey]) == 0 {
			return nil, nil, fmt.Errorf("imagePullSecret.Data is empty")
		}

		c.chartConfig.Images.ImageCredentials.DockerConfigJson = string(imagePullSecret.Data[corev1.DockerConfigJsonKey])
	}

	// MultipleHubs
	// Doesn't affect on the local-cluster.
	// Using MultipleHubs can control the bootstrap kubeConfig secret/secrets easier.
	var localCluster bool
	if c.managedCluster != nil && localcluster.IsClusterSelfManaged(c.managedCluster) {
		localCluster = true
	}

	if !localCluster &&
		c.klusterletConfig != nil &&
		c.klusterletConfig.Spec.BootstrapKubeConfigs.Type == operatorv1.LocalSecrets {
		if c.klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets == nil {
			return nil, nil, fmt.Errorf("local secrets should be set")
		}

		c.chartConfig.Klusterlet.RegistrationConfiguration.FeatureGates = append(c.chartConfig.Klusterlet.RegistrationConfiguration.FeatureGates,
			operatorv1.FeatureGate{
				Feature: string(apifeature.MultipleHubs),
				Mode:    operatorv1.FeatureGateModeTypeEnable,
			})
		c.chartConfig.Klusterlet.RegistrationConfiguration.BootstrapKubeConfigs = *c.klusterletConfig.Spec.BootstrapKubeConfigs.DeepCopy()
		c.chartConfig.Klusterlet.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets = append(
			c.chartConfig.Klusterlet.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets, operatorv1.KubeConfigSecret{
				Name: constants.DefaultBootstrapHubKubeConfigSecretName + "-current-hub",
			})

		bootstrapKubeConfigSecrets, err := convertKubeConfigSecrets(ctx,
			c.klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets, clientHolder.KubeClient)
		if err != nil {
			return nil, nil, err
		}
		// add default bootstrap kubeconfig secret into the list:
		// TODO: deduplicate the bootstrap kubeconfig secrets @xuezhaojun
		bootstrapKubeConfigSecrets = append(bootstrapKubeConfigSecrets, chart.BootStrapKubeConfig{
			Name:       constants.DefaultBootstrapHubKubeConfigSecretName + "-current-hub",
			KubeConfig: c.chartConfig.BootstrapHubKubeConfig,
		})
		c.chartConfig.MultiHubBootstrapHubKubeConfigs = bootstrapKubeConfigSecrets
	}

	crds, objects, err := chart.RenderKlusterletChart(c.chartConfig, c.chartConfig.Klusterlet.Namespace)
	if err != nil {
		return nil, nil, err
	}

	crdBytes := AggregateObjects(crds)
	manifestsBytes := AggregateObjects(objects)

	if c.chartConfig.NoOperator {
		return manifestsBytes, nil, nil
	}

	additionalManifestsBytes, err := filesToTemplateBytes(additionalClusterRoleFiles, c.chartConfig)
	if err != nil {
		return nil, nil, err
	}
	manifestsBytes = append(manifestsBytes, additionalManifestsBytes...)
	return manifestsBytes, crdBytes, nil
}

func convertKubeConfigSecrets(ctx context.Context,
	kcs []operatorv1.KubeConfigSecret, kubeClient kubernetes.Interface) ([]chart.BootStrapKubeConfig, error) {
	var bootstrapKubeConfigSecrets []chart.BootStrapKubeConfig
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

		bootstrapKubeConfigSecrets = append(bootstrapKubeConfigSecrets, chart.BootStrapKubeConfig{
			Name:       s.Name,
			KubeConfig: string(secret.Data["kubeconfig"]),
		})
	}

	return bootstrapKubeConfigSecrets, nil
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

func AggregateObjects(objects [][]byte) []byte {
	var namespaces, otherObjs []string
	var klusterlet string
	for _, obj := range objects {
		unstructuredObj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(obj, unstructuredObj); err != nil {
			klog.Errorf("failed to unmarshal object %s: %v", string(obj), err)
			continue
		}
		switch unstructuredObj.GetKind() {
		case "Namespace":
			namespaces = append(namespaces, string(obj))
		case "Klusterlet":
			klusterlet = string(obj)
		case "":
			// do nothing
		default:
			otherObjs = append(otherObjs, string(obj))
		}
	}

	manifests := new(bytes.Buffer)
	for _, ns := range namespaces {
		manifests.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, ns))
	}

	sort.SliceStable(otherObjs, func(i, j int) bool {
		return otherObjs[i] < otherObjs[j]
	})

	for _, obj := range otherObjs {
		manifests.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, obj))
	}

	// put klusterlet CR behind in order to make sure the klusterlet CR can be applied after the CRD is applied successfully.
	if klusterlet != "" {
		manifests.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, klusterlet))
	}

	return manifests.Bytes()
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

func getKlusterletAgentImages(kcRegistries []klusterletconfigv1alpha1.Registries,
	clusterAnnotations map[string]string) (map[string]string, error) {
	agentImageNames := map[string]string{
		constants.RegistrationOperatorImageEnvVarName: os.Getenv(constants.RegistrationOperatorImageEnvVarName),
		constants.RegistrationImageEnvVarName:         os.Getenv(constants.RegistrationImageEnvVarName),
		constants.WorkImageEnvVarName:                 os.Getenv(constants.WorkImageEnvVarName)}

	agentImageEnvNames := []string{constants.RegistrationOperatorImageEnvVarName,
		constants.RegistrationImageEnvVarName, constants.WorkImageEnvVarName}

	for _, agentImageEnvName := range agentImageEnvNames {
		defaultImage := agentImageNames[agentImageEnvName]
		if defaultImage == "" {
			return nil, fmt.Errorf("environment variable %s not defined", agentImageEnvName)
		}
		if len(kcRegistries) != 0 {
			for i := 0; i < len(kcRegistries); i++ {
				registry := kcRegistries[i]
				name := imageOverride(registry.Source, registry.Mirror, defaultImage)
				if name != defaultImage {
					agentImageNames[agentImageEnvName] = name
				}
			}
		} else {
			var err error
			agentImageNames[agentImageEnvName], err = imageregistry.OverrideImageByAnnotation(clusterAnnotations, defaultImage)
			if err != nil {
				return nil, err
			}
		}
	}

	return agentImageNames, nil
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
