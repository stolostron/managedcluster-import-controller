// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package constants

import "time"

const YamlSperator = "\n---\n"

/* #nosec */
const AutoImportSecretName string = "auto-import-secret"

// AutoImportRetryName is the secret data key of auto import retry
const AutoImportRetryName string = "autoImportRetry"

const PodNamespaceEnvVarName = "POD_NAMESPACE"

const ImportFinalizer string = "managedcluster-import-controller.open-cluster-management.io/cleanup"

const SelfManagedLabel string = "local-cluster"

const ClusterImportSecretLabel = "managedcluster-import-controller.open-cluster-management.io/import-secret"

const (
	CreatedViaAnnotation = "open-cluster-management/created-via"
	CreatedViaAI         = "assisted-installer"
	CreatedViaHive       = "hive"
	CreatedViaDiscovery  = "discovery"
)

/* #nosec */
const (
	ImportSecretNameSuffix         = "import"
	ImportSecretImportYamlKey      = "import.yaml"
	ImportSecretCRDSYamlKey        = "crds.yaml"
	ImportSecretCRDSV1YamlKey      = "crdsv1.yaml"
	ImportSecretCRDSV1beta1YamlKey = "crdsv1beta1.yaml"
)

const (
	// KlusterletDeployModeLabel describe the klusterlet deploy mode when importing a managed cluster.
	// If the value is "Hypershift-Detached", the ManagementClusterNameAnnotation annotation will be required,
	// we use ManagementClusterNameAnnotation to determine where to deploy the registration-agent and work-agent.
	KlusterletDeployModeLabel string = "import.open-cluster-management.io/klusterlet-deploy-mode"

	// ManagementClusterNameAnnotation is required in Hypershift-Detached mode, and the management cluster MUST be one
	// of the managed cluster of the hub. The value of the annotation should be the ManagedCluster name of management cluster.
	ManagementClusterNameAnnotation string = "import.open-cluster-management.io/management-cluster-name"
)

const (
	// KlusterletDeployModeDefault is the default deploy mode. the klusterlet will be deployed in the managed-cluster.
	KlusterletDeployModeDefault string = "Default"

	// KlusterletDeployModeDetached means deploying klusterlet outside. the klusterlet will be deployed outside of the managed-cluster.
	// This value is reserved for the general detached mode for klusterlet that we are not sure how to present for users for now.
	KlusterletDeployModeDetached string = "Detached"

	// KlusterletDeployModeHypershiftDetached means deploying klusterlet outside. the klusterlet will be deployed on the hypershift
	// management cluster.
	KlusterletDeployModeHypershiftDetached string = "Hypershift-Detached"
)

const (
	// // KlusterletSuffix is a suffix of the klusterlet manifestwork name.
	// KlusterletSuffix = "klusterlet"

	// HypershiftDetachedManifestworkSuffix is a suffix of the hypershift detached mode klusterlet manifestwork name.
	HypershiftDetachedKlusterletManifestworkSuffix = "hypershift-detached-klusterlet"

	// HypershiftDetachedManagedKubeconfigManifestworkSuffix is a suffix of the hypershift detached mode managed custer kubeconfig manifestwork name.
	HypershiftDetachedManagedKubeconfigManifestworkSuffix = "hypershift-detached-kubeconfig"

	// ManifestWorkFinalizer is used to delete all manifestworks before deleting a managed cluster.
	ManifestWorkFinalizer = "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup"

	// PostponeDeletionAnnotation is used to delete the manifest work with this annotation until 10 min after the cluster is deleted.
	PostponeDeletionAnnotation = "open-cluster-management/postpone-delete"

	// ManifestWorkPostponeDeleteTime is the postponed time to delete manifest work with postpone-delete annotation
	ManifestWorkPostponeDeleteTime = 10 * time.Minute
)
