// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
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

type hostedWorker struct {
	clientHolder *helpers.ClientHolder
}

var _ importWorker = &hostedWorker{}

func (w *hostedWorker) generateImportSecret(ctx context.Context, managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error) {
	bootstrapKubeconfigData, expiration, err := getBootstrapKubeConfigData(ctx, w.clientHolder, managedCluster)
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

	config := KlusterletRenderConfig{
		ManagedClusterNamespace: managedCluster.Name,
		KlusterletNamespace:     klusterletNamespace(managedCluster),
		BootstrapKubeconfig:     base64.StdEncoding.EncodeToString(bootstrapKubeconfigData),
		RegistrationImageName:   registrationImageName,
		WorkImageName:           workImageName,
		NodeSelector:            nodeSelector,
		Tolerations:             tolerations,
		InstallMode:             string(operatorv1.InstallModeHosted),
	}

	files := append([]string{}, klusterletFiles...)

	yamlcontent, err := filesToTemplateBytes(files, config)
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
			Annotations: map[string]string{
				constants.KlusterletDeployModeAnnotation: constants.KlusterletDeployModeHosted,
			},
		},
		Data: map[string][]byte{
			constants.ImportSecretImportYamlKey: yamlcontent,
		},
	}

	if len(expiration) != 0 {
		secret.Data[constants.ImportSecretTokenExpiration] = expiration
	}

	return secret, nil
}
