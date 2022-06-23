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
	bootStrapSecret, err := getBootstrapSecret(ctx, w.clientHolder.KubeClient, managedCluster)
	if err != nil {
		return nil, err
	}

	bootstrapKubeconfigData, err := createKubeconfigData(ctx, w.clientHolder, bootStrapSecret)
	if err != nil {
		return nil, err
	}

	imagePullSecret, err := getImagePullSecret(ctx, w.clientHolder, managedCluster)
	if err != nil {
		return nil, err
	}

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
			return nil, fmt.Errorf("there is invalid type of the data of pull secret %v/%v",
				imagePullSecret.GetNamespace(), imagePullSecret.GetName())
		}
	}

	registrationOperatorImageName, err := getImage(managedCluster, registrationOperatorImageEnvVarName)
	if err != nil {
		return nil, err
	}

	registrationImageName, err := getImage(managedCluster, registrationImageEnvVarName)
	if err != nil {
		return nil, err
	}

	workImageName, err := getImage(managedCluster, workImageEnvVarName)
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

	type DefaultRenderConfig struct {
		KlusterletRenderConfig
		UseImagePullSecret        bool
		ImagePullSecretName       string
		ImagePullSecretData       string
		ImagePullSecretConfigKey  string
		ImagePullSecretType       corev1.SecretType
		RegistrationOperatorImage string
	}
	config := DefaultRenderConfig{
		KlusterletRenderConfig: KlusterletRenderConfig{
			ManagedClusterNamespace: managedCluster.Name,
			KlusterletNamespace:     klusterletNamespace,
			BootstrapKubeconfig:     base64.StdEncoding.EncodeToString(bootstrapKubeconfigData),
			RegistrationImageName:   registrationImageName,
			WorkImageName:           workImageName,
			NodeSelector:            nodeSelector,
			Tolerations:             tolerations,
			InstallMode:             string(operatorv1.InstallModeDefault),
		},

		UseImagePullSecret:        useImagePullSecret,
		ImagePullSecretName:       managedClusterImagePullSecretName,
		ImagePullSecretData:       imagePullSecretDataBase64,
		ImagePullSecretType:       imagePullSecretType,
		ImagePullSecretConfigKey:  dockerConfigKey,
		RegistrationOperatorImage: registrationOperatorImageName,
	}

	var deploymentFiles = make([]string, 0)
	// deploy the klusterletOperatorFiles first, it contains the agent namespace, if not deploy
	// the namespace first, other namespace scope resources will fail.
	deploymentFiles = append(append(deploymentFiles, klusterletOperatorFiles...), klusterletFiles...)
	if useImagePullSecret {
		deploymentFiles = append(deploymentFiles, "manifests/klusterlet/image_pull_secret.yaml")
	}

	importYAML := new(bytes.Buffer)
	for _, file := range deploymentFiles {
		template, err := manifestFiles.ReadFile(file)
		if err != nil {
			// this should not happen, if happened, panic here
			panic(err)
		}
		raw := helpers.MustCreateAssetFromTemplate(file, template, config)
		importYAML.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(raw)))
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
			constants.ImportSecretImportYamlKey:      importYAML.Bytes(),
			constants.ImportSecretCRDSYamlKey:        crdsV1YAML.Bytes(),
			constants.ImportSecretCRDSV1YamlKey:      crdsV1YAML.Bytes(),
			constants.ImportSecretCRDSV1beta1YamlKey: crdsV1beta1YAML.Bytes(),
		},
	}

	return secret, nil
}
