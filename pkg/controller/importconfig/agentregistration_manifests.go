package importconfig

import (
	"context"
	"encoding/base64"
	"os"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

func GenerateKlusterletCRDsV1() ([]byte, error) {
	return filesToTemplateBytes([]string{klusterletCrdsV1File}, nil)
}

func GenerateKlusterletCRDsV1Beta1() ([]byte, error) {
	return filesToTemplateBytes([]string{klusterletCrdsV1beta1File}, nil)
}

func GenerateAgentRegistrationManifests(ctx context.Context, clientHolder *helpers.ClientHolder, clusterID string) ([]byte, error) {
	// Bootstrap kubeconfig
	bootstrapKubeconfigData, _, err := createKubeConfig(ctx, clientHolder, AgentRegistrationDefaultBootstrapSAName,
		os.Getenv(constants.PodNamespaceEnvVarName), 7*24*3600) // 7 days
	if err != nil {
		return nil, err
	}

	// Image Pull
	imagePullSecret, err := getDefaultImagePullSecret(ctx, clientHolder)
	if err != nil {
		return nil, err
	}

	imagePullSecretConfig, err := getImagePullSecretConfig(imagePullSecret)
	if err != nil {
		return nil, err
	}

	var files []string
	files = append(append(files, klusterletOperatorFiles...), klusterletFiles...)
	if imagePullSecretConfig.UseImagePullSecret {
		files = append(files, "manifests/klusterlet/image_pull_secret.yaml")
	}

	// Images
	registrationOperatorImageName, err := getImage(registrationOperatorImageEnvVarName, nil)
	if err != nil {
		return nil, err
	}
	registrationImageName, err := getImage(registrationImageEnvVarName, nil)
	if err != nil {
		return nil, err
	}
	workImageName, err := getImage(workImageEnvVarName, nil)
	if err != nil {
		return nil, err
	}

	config := DefaultRenderConfig{
		KlusterletRenderConfig: KlusterletRenderConfig{
			ManagedClusterNamespace: clusterID,
			KlusterletNamespace:     defaultKlusterletNamespace,            // TODO: get via `klusterletNamespace`
			InstallMode:             string(operatorv1.InstallModeDefault), // TODO: only support default mode by now
			BootstrapKubeconfig:     base64.StdEncoding.EncodeToString(bootstrapKubeconfigData),
			RegistrationImageName:   registrationImageName,
			WorkImageName:           workImageName,
			NodeSelector:            make(map[string]string), // TODO: get via klusterletConfig
			Tolerations: []corev1.Toleration{ // TODO: get via klusterletConfig
				{
					Effect:   corev1.TaintEffectNoSchedule,
					Key:      "node-role.kubernetes.io/infra",
					Operator: corev1.TolerationOpExists,
				},
			},
			ClusterAnnotations: map[string]string{"agent.open-cluster-management.io/create-with-default-klusterletaddonconfig": "true"},
		},
		RegistrationOperatorImage: registrationOperatorImageName,
		// Image Pull
		ImagePullSecretConfig: imagePullSecretConfig,
	}

	return filesToTemplateBytes(files, config)
}
