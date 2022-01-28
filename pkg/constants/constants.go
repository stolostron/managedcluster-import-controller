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
	// If the value is "Detached", the ManagementClusterNameLabel label will be required, we use
	// ManagementClusterNameLabel to determine where to deploy the registration-agent and work-agent.
	KlusterletDeployModeLabel string = "managedcluster-import-controller.open-cluster-management.io/klusterlet-deploy-mode"

	// ManagementClusterNameLabel is required in Detached mode, and the management cluster MUST be one
	// of the managed cluster of the hub. The value of the label should be the ManagedCluster name of
	// management cluster.
	ManagementClusterNameLabel string = "managedcluster-import-controller.open-cluster-management.io/management-cluster-name"
)
