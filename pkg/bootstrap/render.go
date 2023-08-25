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

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

var klusterletOperatorFiles = []string{
	"manifests/klusterlet/namespace.yaml",
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

type RenderConfig struct {
	KlusterletRenderConfig
	ImagePullSecretConfig
}

// KlusterletRenderConfig defines variables used in the klusterletFiles.
type KlusterletRenderConfig struct {
	KlusterletNamespace       string
	ManagedClusterNamespace   string
	BootstrapKubeconfig       string
	RegistrationOperatorImage string
	RegistrationImageName     string
	WorkImageName             string
	NodeSelector              map[string]string
	Tolerations               []corev1.Toleration
	InstallMode               string
	ClusterAnnotations        map[string]string
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

	// In the managed cluster annotations, it contains nodeSelectors and tolerations for the klusterlet deployment.
	ManagedClusterAnnotations map[string]string

	generateImagePullSecret bool // by default is true, in hosted mode, it will be set false
}

func NewKlusterletManifestsConfig(installMode operatorv1.InstallMode,
	clusterName, klusterletNamespace string, bootstrapKubeconfig []byte) *KlusterletManifestsConfig {
	return &KlusterletManifestsConfig{
		InstallMode:             installMode,
		ClusterName:             clusterName,
		KlusterletNamespace:     klusterletNamespace,
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

// WithManagedClusterAnnotations sets the managed cluster annotations.
// The managed cluster annotations contains information like: image pull secret, nodeSelector, tolerations, etc.
// We need to extract these information from the managed cluster annotations to render the klusterlet manifests.
func (c *KlusterletManifestsConfig) WithManagedClusterAnnotations(a map[string]string) *KlusterletManifestsConfig {
	c.ManagedClusterAnnotations = a
	return c
}

func (c *KlusterletManifestsConfig) WithImagePullSecretGenerate(g bool) *KlusterletManifestsConfig {
	c.generateImagePullSecret = g
	return c
}

// Generate returns the rendered klusterlet manifests in bytes.
func (b *KlusterletManifestsConfig) Generate(ctx context.Context, clientHolder *helpers.ClientHolder) ([]byte, error) {
	// TODO: Get KlusterletConfig, the image pull secret, image override and nodePlacement will use the KlusterletConfig first then the annotations. @xuezhaojun

	// Files depends on the install mode
	var files []string
	switch b.InstallMode {
	case operatorv1.InstallModeHosted:
		files = append(files, klusterletFiles...)
	case operatorv1.InstallModeDefault:
		files = append(files, klusterletOperatorFiles...)
		files = append(files, klusterletFiles...)
	default:
		return nil, fmt.Errorf("invalid install mode: %s", b.InstallMode)
	}

	// Images override
	registrationOperatorImageName, err := getImage(constants.RegistrationOperatorImageEnvVarName, b.ManagedClusterAnnotations)
	if err != nil {
		return nil, err
	}

	registrationImageName, err := getImage(constants.RegistrationImageEnvVarName, b.ManagedClusterAnnotations)
	if err != nil {
		return nil, err
	}

	workImageName, err := getImage(constants.WorkImageEnvVarName, b.ManagedClusterAnnotations)
	if err != nil {
		return nil, err
	}

	// NodeSelector and Tolerations
	nodeSelector, err := helpers.GetNodeSelector(b.ManagedClusterAnnotations)
	if err != nil {
		return nil, fmt.Errorf("Get nodeSelector for cluster %s failed: %v", b.ClusterName, err)
	}

	tolerations, err := helpers.GetTolerations(b.ManagedClusterAnnotations)
	if err != nil {
		return nil, fmt.Errorf("Get tolerations for cluster %s failed: %v", b.ClusterName, err)
	}

	renderConfig := RenderConfig{
		KlusterletRenderConfig: KlusterletRenderConfig{
			ManagedClusterNamespace: b.ClusterName,
			KlusterletNamespace:     b.KlusterletNamespace,
			InstallMode:             string(b.InstallMode),

			// BootstrapKubeConfig
			BootstrapKubeconfig: base64.StdEncoding.EncodeToString(b.BootstrapKubeconfig),

			// Images
			RegistrationOperatorImage: registrationOperatorImageName,
			RegistrationImageName:     registrationImageName,
			WorkImageName:             workImageName,

			// NodePlacement
			NodeSelector: nodeSelector,
			Tolerations:  tolerations,

			// KlusterletClusterAnnotations
			ClusterAnnotations: b.KlusterletClusterAnnotations,
		},
	}

	// If need to generate imagePullSecret
	if b.generateImagePullSecret {
		// Image pull secret, need to add `manifests/klusterlet/image_pull_secret.yaml` to files if imagePullSecret is not nil

		imagePullSecret, err := getImagePullSecret(ctx, clientHolder, b.ManagedClusterAnnotations)
		if err != nil {
			return nil, err
		}

		if imagePullSecret != nil {
			files = append(files, "manifests/klusterlet/image_pull_secret.yaml")
		}

		imagePullSecretConfig, err := getImagePullSecretConfig(imagePullSecret)
		if err != nil {
			return nil, err
		}

		renderConfig.ImagePullSecretConfig = imagePullSecretConfig
	}

	// Render the klusterlet manifests
	manifestsBytes, err := filesToTemplateBytes(files, renderConfig)
	if err != nil {
		return nil, err
	}

	return manifestsBytes, nil
}

func GenerateKlusterletCRDsV1() ([]byte, error) {
	return filesToTemplateBytes([]string{klusterletCrdsV1File}, nil)
}

func GenerateKlusterletCRDsV1Beta1() ([]byte, error) {
	return filesToTemplateBytes([]string{klusterletCrdsV1beta1File}, nil)
}

func GenerateHubBootstrapRBACObjects(managedClusterName string) ([]runtime.Object, error) {
	return filesToObjects(hubFiles, struct {
		ManagedClusterName          string
		ManagedClusterNamespace     string
		BootstrapServiceAccountName string
	}{
		ManagedClusterName:          managedClusterName,
		ManagedClusterNamespace:     managedClusterName,
		BootstrapServiceAccountName: GetBootstrapSAName(managedClusterName),
	})
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

func filesToObjects(files []string, config interface{}) ([]runtime.Object, error) {
	objects := []runtime.Object{}
	for _, file := range files {
		template, err := ManifestFiles.ReadFile(file)
		if err != nil {
			return nil, err
		}

		objects = append(objects, helpers.MustCreateObjectFromTemplate(file, template, config))
	}
	return objects, nil
}

func getImage(envName string, clusterAnnotations map[string]string) (string, error) {
	defaultImage := os.Getenv(envName)
	if defaultImage == "" {
		return "", fmt.Errorf("environment variable %s not defined", envName)
	}

	return imageregistry.OverrideImageByAnnotation(clusterAnnotations, defaultImage)
}
