// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package constants

const YamlSperator = "\n---\n"

/* #nosec */
const AutoImportSecretName string = "auto-import-secret"

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
