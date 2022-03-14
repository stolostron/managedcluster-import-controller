// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"fmt"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/features"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	corev1 "k8s.io/api/core/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

type importWorker interface {
	generateImportSecret(ctx context.Context, managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error)
}

type workerFactory struct {
	clientHolder *helpers.ClientHolder
}

func (f *workerFactory) newWorker(mode string) (importWorker, error) {
	switch mode {
	case constants.KlusterletDeployModeDefault:
		return &defaultWorker{
			clientHolder: f.clientHolder,
		}, nil
	case constants.KlusterletDeployModeHosted:
		if !features.DefaultMutableFeatureGate.Enabled(features.KlusterletHostedMode) {
			return nil, fmt.Errorf("featurn gate %s is not enabled", features.KlusterletHostedMode)
		}
		return &hostedWorker{
			clientHolder: f.clientHolder,
		}, nil
	default:
		return nil, fmt.Errorf("klusterlet deploy mode %s not supportted", mode)
	}
}

// KlusterletRenderConfig defines variables used in the klusterletFiles.
type KlusterletRenderConfig struct {
	KlusterletNamespace     string
	ManagedClusterNamespace string
	BootstrapKubeconfig     string
	RegistrationImageName   string
	WorkImageName           string
	NodeSelector            map[string]string
	InstallMode             string
}
