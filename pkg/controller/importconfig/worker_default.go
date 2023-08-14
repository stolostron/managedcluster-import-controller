// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

type defaultWorker struct {
	clientHolder *helpers.ClientHolder
}

var _ importWorker = &defaultWorker{}

func (w *defaultWorker) generateImportSecret(ctx context.Context, managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error) {
	bootstrapKubeconfigData, expiration, err := getBootstrapKubeConfigData(ctx, w.clientHolder, managedCluster)
	if err != nil {
		return nil, err
	}

	imagePullSecret, err := getImagePullSecret(ctx, w.clientHolder, managedCluster)
	if err != nil {
		return nil, err
	}

	imagePullSecretConfig, err := getImagePullSecretConfig(imagePullSecret)
	if err != nil {
		return nil, err
	}

	registrationOperatorImageName, err := getImage(registrationOperatorImageEnvVarName, managedCluster.GetAnnotations())
	if err != nil {
		return nil, err
	}

	registrationImageName, err := getImage(registrationImageEnvVarName, managedCluster.GetAnnotations())
	if err != nil {
		return nil, err
	}

	workImageName, err := getImage(workImageEnvVarName, managedCluster.GetAnnotations())
	if err != nil {
		return nil, err
	}

	nodeSelector, err := helpers.GetNodeSelector(managedCluster)
	if err != nil {
		return nil, err
	}

	tolerations, err := helpers.GetTolerations(managedCluster)
	if err != nil {
		return nil, err
	}

	config := DefaultRenderConfig{
		KlusterletRenderConfig: KlusterletRenderConfig{
			ManagedClusterNamespace: managedCluster.Name,
			KlusterletNamespace:     klusterletNamespace(managedCluster),
			BootstrapKubeconfig:     base64.StdEncoding.EncodeToString(bootstrapKubeconfigData),
			RegistrationImageName:   registrationImageName,
			WorkImageName:           workImageName,
			NodeSelector:            nodeSelector,
			Tolerations:             tolerations,
			InstallMode:             string(operatorv1.InstallModeDefault),
		},

		ImagePullSecretConfig:     imagePullSecretConfig,
		RegistrationOperatorImage: registrationOperatorImageName,
	}

	var deploymentFiles = make([]string, 0)
	// deploy the klusterletOperatorFiles first, it contains the agent namespace, if not deploy
	// the namespace first, other namespace scope resources will fail.
	deploymentFiles = append(append(deploymentFiles, klusterletOperatorFiles...), klusterletFiles...)
	if imagePullSecretConfig.UseImagePullSecret {
		deploymentFiles = append(deploymentFiles, "manifests/klusterlet/image_pull_secret.yaml")
	}

	crdsV1beta1YAML := new(bytes.Buffer)
	crdsV1beta1, err := manifestFiles.ReadFile(klusterletCrdsV1beta1File)
	if err != nil {
		return nil, err
	}
	crdsV1beta1YAML.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(crdsV1beta1)))

	crdsV1YAML := new(bytes.Buffer)
	crdsV1, err := manifestFiles.ReadFile(klusterletCrdsV1File)
	if err != nil {
		return nil, err
	}
	crdsV1YAML.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(crdsV1)))

	yamlcontent, err := filesToTemplateBytes(deploymentFiles, config)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.ImportSecretNameSuffix),
			Namespace: managedCluster.Name,
			Labels: map[string]string{
				constants.ClusterImportSecretLabel: "",
			},
		},
		Data: map[string][]byte{
			constants.ImportSecretImportYamlKey:      yamlcontent,
			constants.ImportSecretCRDSYamlKey:        crdsV1YAML.Bytes(),
			constants.ImportSecretCRDSV1YamlKey:      crdsV1YAML.Bytes(),
			constants.ImportSecretCRDSV1beta1YamlKey: crdsV1beta1YAML.Bytes(),
		},
	}

	if len(expiration) != 0 {
		secret.Data[constants.ImportSecretTokenExpiration] = expiration
	}

	return secret, nil
}
