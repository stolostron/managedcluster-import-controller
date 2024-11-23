// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// NOSONAR:S2068
package constants

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

const YamlSperator = "\n---\n"

/* #nosec */
const AutoImportSecretName string = "auto-import-secret"

const (
	// AutoImportRetryName is the secret data key of auto import retry
	AutoImportRetryName string = "autoImportRetry"

	// AnnotationAutoImportCurrentRetry is the annotation key of auto import secret used to indicate
	// the current retry times of auto importing a managed cluster
	AnnotationAutoImportCurrentRetry = "managedcluster-import-controller.open-cluster-management.io/current-retry"

	// AnnotationKeepingAutoImportSecret is the annotation key of auto import secret used to indicate
	// keeping this secret after the cluster is imported successfully
	AnnotationKeepingAutoImportSecret = "managedcluster-import-controller.open-cluster-management.io/keeping-auto-import-secret"

	// LabelAutoImportRestore is the label key of auto import secret used for backup restore case
	LabelAutoImportRestore = "cluster.open-cluster-management.io/restore-auto-import-secret"
)

/* #nosec */
const (
	RegistrationOperatorImageEnvVarName = "REGISTRATION_OPERATOR_IMAGE"
	RegistrationImageEnvVarName         = "REGISTRATION_IMAGE"
	WorkImageEnvVarName                 = "WORK_IMAGE"
	DefaultImagePullSecretEnvVarName    = "DEFAULT_IMAGE_PULL_SECRET" // #nosec G101
)

const PodNamespaceEnvVarName = "POD_NAMESPACE"

const ImportFinalizer string = "managedcluster-import-controller.open-cluster-management.io/cleanup"

const SelfManagedLabel string = "local-cluster"

const (
	AppliedManifestWorkEvictionGracePeriodInfinite string = "INFINITE"
	AppliedManifestWorkEvictionGracePeriod100Years string = "876000h" // 100 * 365 * 24h
)

const (
	ClusterImportSecretLabel = "managedcluster-import-controller.open-cluster-management.io/import-secret"
	KlusterletWorksLabel     = "import.open-cluster-management.io/klusterlet-works"
	HostedClusterLabel       = "import.open-cluster-management.io/hosted-cluster"
)

const (
	CreatedViaAnnotation = "open-cluster-management/created-via"
	CreatedViaAI         = "assisted-installer"
	CreatedViaHive       = "hive"
	CreatedViaDiscovery  = "discovery"
	CreatedViaHypershift = "hypershift"
)

// NOSONAR-START
/* #nosec */
const (
	ImportSecretNameSuffix             = "import"
	ImportSecretImportYamlKey          = "import.yaml"
	ImportSecretCRDSYamlKey            = "crds.yaml"        // #nosec G101
	ImportSecretCRDSV1YamlKey          = "crdsv1.yaml"      // #nosec G101
	ImportSecretCRDSV1beta1YamlKey     = "crdsv1beta1.yaml" // #nosec G101
	ImportSecretTokenExpiration        = "expiration"
	DefaultSecretTokenExpirationSecond = 360 * 24 * 60 * 60 // 360 days
	ImportSecretTokenCreation          = "creation"
	DefaultSecretTokenRefreshThreshold = 360 * 24 * time.Hour / 5 // 72 days
)

// NOSONAR-END

const (
	// KlusterletDeployModeAnnotation describe the klusterlet deploy mode when importing a managed cluster.
	// If the value is "Hosted", the HostingClusterNameAnnotation annotation will be required,
	// we use HostingClusterNameAnnotation to determine where to deploy the registration-agent and work-agent.
	KlusterletDeployModeAnnotation string = "import.open-cluster-management.io/klusterlet-deploy-mode"

	// HostingClusterNameAnnotation is required in Hosted mode, and the hosting cluster MUST be one
	// of the managed cluster of the hub. The value of the annotation should be the ManagedCluster name of
	// the hosting cluster.
	HostingClusterNameAnnotation string = "import.open-cluster-management.io/hosting-cluster-name"

	// KlusterletNamespaceAnnotation is used to customize the namespace to deploy the agent on the managed
	// cluster. The namespace must have a prefix of "open-cluster-management-", and if it is not set,
	// the namespace of "open-cluster-management-agent" is used to deploy agent.
	// In the Hosted mode, this namespace still exists on the managed cluster to contain
	// necessary resources, like service accounts, roles and rolebindings.
	KlusterletNamespaceAnnotation string = "import.open-cluster-management.io/klusterlet-namespace"
)

const (
	// HostedManifestworkSuffix is a suffix of the hosted mode klusterlet manifestwork name.
	HostedKlusterletManifestworkSuffix = "hosted-klusterlet"

	// HostedManagedKubeconfigManifestworkSuffix is a suffix of the hosted mode managed custer kubeconfig manifestwork name.
	HostedManagedKubeconfigManifestworkSuffix = "hosted-kubeconfig"

	// ManifestWorkFinalizer is used to delete all manifestworks before deleting a managed cluster.
	ManifestWorkFinalizer = "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup"

	// PostponeDeletionAnnotation is used to delete the manifest work with this annotation until 10 min after the cluster is deleted.
	PostponeDeletionAnnotation = "open-cluster-management/postpone-delete"

	// ManifestWorkPostponeDeleteTime is the postponed time to delete manifest work with postpone-delete annotation
	ManifestWorkPostponeDeleteTime = 10 * time.Minute

	DefaultKlusterletNamespace = "open-cluster-management-agent"
)

const (
	KlusterletSuffix     = "klusterlet"
	KlusterletCRDsSuffix = "klusterlet-crds"
)

const (
	// ConditionManagedClusterImportSucceeded is the condition type of managed cluster to indicate whether the managed
	// cluster is imported successfully
	ConditionManagedClusterImportSucceeded = "ManagedClusterImportSucceeded"

	ConditionReasonManagedClusterWaitForImporting = "ManagedClusterWaitForImporting"
	ConditionReasonManagedClusterImporting        = "ManagedClusterImporting"
	ConditionReasonManagedClusterImportFailed     = "ManagedClusterImportFailed"
	ConditionReasonManagedClusterImported         = "ManagedClusterImported"

	ConditionReasonManagedClusterDetaching      = "ManagedClusterDetaching"
	ConditionReasonManagedClusterForceDetaching = "ManagedClusterForceDetaching"
)

const (
	EventReasonManagedClusterImportFailed = "Failed"
	EventReasonManagedClusterImported     = "Imported"
	EventReasonManagedClusterImporting    = "Importing"
	EventReasonManagedClusterWait         = "WaitForImporting"

	EventReasonManagedClusterDetaching      = "Detaching"
	EventReasonManagedClusterForceDetaching = "ForceDetaching"
)

/* #nosec */
const (
	AutoImportSecretKubeConfig    corev1.SecretType = "auto-import/kubeconfig" // #nosec G101
	AutoImportSecretKubeConfigKey string            = "kubeconfig"             // #nosec G101

	AutoImportSecretKubeToken     corev1.SecretType = "auto-import/kubetoken" // #nosec G101
	AutoImportSecretKubeServerKey string            = "server"
	AutoImportSecretKubeTokenKey  string            = "token"

	AutoImportSecretRosaConfig                corev1.SecretType = "auto-import/rosa"
	AutoImportSecretRosaConfigAPIURLKey       string            = "api_url"
	AutoImportSecretRosaConfigAPITokenKey     string            = "api_token"
	AutoImportSecretRosaConfigTokenURLKey     string            = "token_url"
	AutoImportSecretRosaConfigClusterIDKey    string            = "cluster_id"
	AutoImportSecretRosaConfigClientIDKey     string            = "client_id"
	AutoImportSecretRosaConfigClientSecretKey string            = "client_secret"
	AutoImportSecretRosaConfigRetryTimesKey   string            = "retry_times"
	AutoImportSecretRosaConfigAuthMethodKey   string            = "auth_method"
	// The definitions of the auth methods follow the same approach as in discovery:
	// https://github.com/stolostron/discovery/blob/13cb209687bf963b58232eb96b25cf0d20d111ec/controllers/discoveryconfig_controller.go#L251
	// TODO: @xuezhaojun, in long term, the offline-token should be removed, and only use service-account, see more details in Jira 10404.
	AutoImportSecretRosaConfigAuthMethodOfflineToken   string = "offline-token"
	AutoImportSecretRosaConfigAuthMethodServiceAccount string = "service-account"
)

const (
	DefaultKlusterletPriorityClassName = "klusterlet-critical"
)

const (
	GlobalKlusterletConfigName = "global"
)

const (
	ComponentName = "managedcluster-import-controller"
)

const (
	DefaultBootstrapHubKubeConfigSecretName = "bootstrap-hub-kubeconfig" // #nosec G101
)

const (
	// CSRClusterNameLabel is the label key of the managed cluster name in the CSR
	CSRClusterNameLabel = "open-cluster-management.io/cluster-name"

	// If a managed cluster is from the agent-registration, the username of the CSR will be this
	AgentRegistrationBootstrapUser = "system:serviceaccount:multicluster-engine:agent-registration-bootstrap"
)
