package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=klusterletconfigs
// +kubebuilder:resource:scope=Cluster

// KlusterletConfig contains the configuration of a klusterlet including the upgrade strategy, config overrides, proxy configurations etc.
type KlusterletConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of KlusterletConfig
	// +optional
	Spec KlusterletConfigSpec `json:"spec,omitempty"`

	// Status defines the observed state of KlusterletConfig
	// +optional
	Status KlusterletConfigStatus `json:"status,omitempty"`
}

// KlusterletConfigSpec defines the desired state of KlusterletConfig, usually provided by the user.
type KlusterletConfigSpec struct {
	// Registries includes the mirror and source registries. The source registry will be replaced by the Mirror.
	// +optional
	Registries []Registries `json:"registries,omitempty"`

	// PullSecret is the name of image pull secret.
	// +optional
	PullSecret corev1.ObjectReference `json:"pullSecret,omitempty"`

	// NodePlacement enables explicit control over the scheduling of the agent components.
	// If the placement is nil, the placement is not specified, it will be omitted.
	// If the placement is an empty object, the placement will match all nodes and tolerate nothing.
	// +optional
	NodePlacement *operatorv1.NodePlacement `json:"nodePlacement,omitempty"`

	// HubKubeAPIServerConfig specifies the settings required for connecting to the hub Kube API server.
	// If this field is present, the below deprecated fields will be ignored:
	// - HubKubeAPIServerProxyConfig
	// - HubKubeAPIServerURL
	// - HubKubeAPIServerCABundle
	// +optional
	HubKubeAPIServerConfig *KubeAPIServerConfig `json:"hubKubeAPIServerConfig,omitempty"`

	// HubKubeAPIServerProxyConfig holds proxy settings for connections between klusterlet/add-on agents
	// on the managed cluster and the kube-apiserver on the hub cluster.
	// Empty means no proxy settings is available.
	//
	// Deprecated and maintained for backward compatibility, use HubKubeAPIServerConfig.ProxyURL instead
	// +optional
	HubKubeAPIServerProxyConfig KubeAPIServerProxyConfig `json:"hubKubeAPIServerProxyConfig,omitempty"`

	// HubKubeAPIServerURL is the URL of the hub Kube API server.
	// If not present, the .status.apiServerURL of Infrastructure/cluster will be used as the default value.
	// e.g. `oc get infrastructure cluster -o jsonpath='{.status.apiServerURL}'`
	//
	// Deprecated and maintained for backward compatibility, use HubKubeAPIServerConfig.URL instead
	// +optional
	HubKubeAPIServerURL string `json:"hubKubeAPIServerURL,omitempty"`

	// HubKubeAPIServerCABundle is the CA bundle to verify the server certificate of the hub kube API
	// against. If not present, CA bundle will be determined with the logic below:
	// 1). Use the certificate of the named certificate configured in APIServer/cluster if FQDN matches;
	// 2). Otherwise use the CA certificates from kube-root-ca.crt ConfigMap in the cluster namespace;
	//
	// Deprecated and maintained for backward compatibility, use HubKubeAPIServerConfig.ServerVarificationStrategy
	// and HubKubeAPIServerConfig.TrustedCABundles instead
	// +optional
	HubKubeAPIServerCABundle []byte `json:"hubKubeAPIServerCABundle,omitempty"`

	// AppliedManifestWorkEvictionGracePeriod is the eviction grace period the work agent will wait before
	// evicting the AppliedManifestWorks, whose corresponding ManifestWorks are missing on the hub cluster, from
	// the managed cluster. If not present, the default value of the work agent will be used. If its value is
	// set to "INFINITE", it means the AppliedManifestWorks will never been evicted from the managed cluster.
	// +optional
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$|^INFINITE$`
	AppliedManifestWorkEvictionGracePeriod string `json:"appliedManifestWorkEvictionGracePeriod,omitempty"`

	// InstallMode is the mode to install the klusterlet
	InstallMode *InstallMode `json:"installMode,omitempty"`

	// MultipleHubsConfig contains configuration specific to multiple hub scenarios
	// +optional
	MultipleHubsConfig *MultipleHubsConfig `json:"multipleHubsConfig,omitempty"`

	// FeatureGates is the list of feature gate for the klusterlet agent.
	// If it is set empty, default feature gates will be used.
	FeatureGates []operatorv1.FeatureGate `json:"featureGates,omitempty"`

	// ClusterClaimConfiguration represents the configuration of ClusterClaim
	// Effective only when the `ClusterClaim` feature gate is enabled.
	// +optional
	ClusterClaimConfiguration *ClusterClaimConfiguration `json:"clusterClaimConfiguration,omitempty"`

	// WorkStatusSyncInterval is the interval for the work agent to check the status of ManifestWorks.
	// Larger value means less frequent status sync and less api calls to the managed cluster, vice versa.
	// The value(x) should be: 5s <= x <= 1h.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(s|m|h))+$"
	WorkStatusSyncInterval *metav1.Duration `json:"workStatusSyncInterval,omitempty"`
}

// KlusterletConfigStatus defines the observed state of KlusterletConfig.
type KlusterletConfigStatus struct {
}

type InstallMode struct {
	// InstallModeType is the type of install mode.
	// +kubebuilder:default=default
	// +kubebuilder:validation:Enum=default;noOperator
	Type InstallModeType `json:"type,omitempty"`

	// NoOperator is the setting of klusterlet installation when install type is noOperator.
	NoOperator *NoOperator `json:"noOperator,omitempty"`
}

type InstallModeType string

const (
	// InstallModeDefault is the default mode to install klusterlet, the name of the Klusterlet resource
	// is klusterlet and the klusterlet namespace is open-cluster-management-agent.
	InstallModeDefault InstallModeType = "default"
	// InstallModeNoOperator is to install klusterlet without installing klusterlet operator. The name of
	// the klusterlet is by default klusterlet and can be set to klusterlet-{KlusterletNamePostFix}. The
	// install namespace of the klusterlet is open-cluster-management-{KlusterletNamePostFix}
	InstallModeNoOperator InstallModeType = "noOperator"

	// GenBootstrapKubeConfigStrategyDefault represents the default strategy for bootstrap kubeconfig generation
	GenBootstrapKubeConfigStrategyDefault string = "Default"
	// GenBootstrapKubeConfigStrategyIncludeCurrentHub represents the strategy to include current hub when generating bootstrap kubeconfig
	GenBootstrapKubeConfigStrategyIncludeCurrentHub string = "IncludeCurrentHub"
)

type NoOperator struct {
	// Postfix is the postfix of the klusterlet name. The name of the klusterlet is "klusterlet" if
	// it is not set, and "klusterlet-{Postfix}". The install namespace is "open-cluster-management-agent"
	// if it is not set, and "open-cluster-management-{Postfix}".
	// +kubebuilder:validation:MaxLength=33
	// +kubebuilder:validation:Pattern=^[-a-z0-9]*[a-z0-9]$
	Postfix string `json:"postfix,omitempty"`
}

type Registries struct {
	// Mirror is the mirrored registry of the Source. Will be ignored if Mirror is empty.
	// +kubebuilder:validation:Required
	// +required
	Mirror string `json:"mirror"`

	// Source is the source registry. All image registries will be replaced by Mirror if Source is empty.
	// +optional
	Source string `json:"source"`
}

// KubeAPIServerConfig specifies the custom configuration for the Hub kube API server
type KubeAPIServerConfig struct {
	// URL is the endpoint of the hub Kube API server.
	// If not present, the .status.apiServerURL of Infrastructure/cluster will be used as the default value.
	// e.g. `oc get infrastructure cluster -o jsonpath='{.status.apiServerURL}'`
	// +optional
	URL string `json:"url,omitempty"`

	// ServerVerificationStrategy is the strategy used for verifying the server certification;
	// The value could be "UseSystemTruststore", "UseAutoDetectedCABundle", "UseCustomCABundles", empty.
	//
	// When this strategy is not set or value is empty; if there is only one klusterletConfig configured for a cluster,
	// the strategy is eaual to "UseAutoDetectedCABundle", if there are more than one klusterletConfigs, the empty
	// strategy will be overrided by other non-empty strategies.
	//
	// +kubebuilder:validation:Enum=UseSystemTruststore;UseAutoDetectedCABundle;UseCustomCABundles
	// +optional
	ServerVerificationStrategy ServerVerificationStrategy `json:"serverVerificationStrategy,omitempty"`

	// TrustedCABundles refers to a collection of user-provided CA bundles used for verifying the server
	// certificate of the hub Kubernetes API
	// If the ServerVerificationStrategy is set to "UseSystemTruststore", this field will be ignored.
	// Otherwise, the CA certificates from the configured bundles will be appended to the klusterlet CA bundle.
	// +listType:=map
	// +listMapKey:=name
	// +optional
	TrustedCABundles []CABundle `json:"trustedCABundles,omitempty"`

	// ProxyURL is the URL to the proxy to be used for all requests made by client
	// If an HTTPS proxy server is configured, you may also need to add the necessary CA certificates to
	// TrustedCABundles.
	// +optional
	ProxyURL string `json:"proxyURL,omitempty"`
}

// CABundle is a user-provided CA bundle
type CABundle struct {
	// Name is the identifier used to reference the CA bundle; Do not use "auto-detected" as the name
	// since it is the reserved name for the auto-detected CA bundle.
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name,omitempty"`

	// CABundle refers to a ConfigMap with label "import.open-cluster-management.io/ca-bundle"
	// containing the user-provided CA bundle
	// The key of the CA data could be "ca-bundle.crt", "ca.crt", or "tls.crt".
	// +kubebuilder:validation:Required
	// +required
	CABundle ConfigMapReference `json:"caBundle,omitempty"`
}

// ServerVerificationStrategy represents the strategy used for the server certificate varification
type ServerVerificationStrategy string

const (
	// ServerVerificationStrategyUseSystemTruststore is the strategy that utilizes CA certificates in the system
	// truststore of the Operating System to validate the server certificate.
	ServerVerificationStrategyUseSystemTruststore ServerVerificationStrategy = "UseSystemTruststore"

	// ServerVerificationStrategyUseAutoDetectedCABundle is the strategy that automatically detects CA certificates
	// for the hub Kube API server and uses them to validate the server certificate.
	ServerVerificationStrategyUseAutoDetectedCABundle ServerVerificationStrategy = "UseAutoDetectedCABundle"

	// ServerVerificationStrategyUseCustomCABundles is the strategy that uses CA certificates from a custom CA bundle
	// to validate the server certificate.
	ServerVerificationStrategyUseCustomCABundles ServerVerificationStrategy = "UseCustomCABundles"
)

type ConfigMapReference struct {
	// name is the metadata.name of the referenced config map
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`

	// name is the metadata.namespace of the referenced config map
	// +kubebuilder:validation:Required
	// +required
	Namespace string `json:"namespace"`
}

// KubeAPIServerProxyConfig describes the proxy settings for the connections to a kube-apiserver
type KubeAPIServerProxyConfig struct {
	// HTTPProxy is the URL of the proxy for HTTP requests
	// +optional
	HTTPProxy string `json:"httpProxy,omitempty"`

	// HTTPSProxy is the URL of the proxy for HTTPS requests
	// HTTPSProxy will be chosen if both HTTPProxy and HTTPSProxy are set.
	// +optional
	HTTPSProxy string `json:"httpsProxy,omitempty"`

	// CABundle is a CA certificate bundle to verify the proxy server.
	// It will be ignored if only HTTPProxy is set;
	// And it is required when HTTPSProxy is set and self signed CA certificate is used
	// by the proxy server.
	// +optional
	CABundle []byte `json:"caBundle,omitempty"`
}

// ClusterClaimConfiguration represents the configuration of ClusterClaim
type ClusterClaimConfiguration struct {
	// Maximum number of custom ClusterClaims allowed.
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=20
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:Maximum:=100
	// +required
	MaxCustomClusterClaims int32 `json:"maxCustomClusterClaims"`
}

// MultipleHubsConfig contains configuration specific to multiple hub scenarios
type MultipleHubsConfig struct {
	// GenBootstrapKubeConfigStrategy controls the strategy for generating bootstrap kubeconfig files.
	// Default - Generate bootstrap kubeconfigs only with the BootstrapKubeConfigs configured in KlusterletConfig.
	// IncludeCurrentHub - When generating bootstrap kubeconfigs, automatically include the current hub's kubeconfig.
	// +optional
	// +kubebuilder:default:=Default
	// +kubebuilder:validation:Enum=Default;IncludeCurrentHub
	GenBootstrapKubeConfigStrategy string `json:"genBootstrapKubeConfigStrategy,omitempty"`

	// BootstrapKubeConfigs is the list of bootstrap kubeconfigs for multiple hubs
	// +optional
	BootstrapKubeConfigs operatorv1.BootstrapKubeConfigs `json:"bootstrapKubeConfigs,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KlusterletConfigList contains a list of KlusterletConfig.
type KlusterletConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KlusterletConfig `json:"items"`
}
