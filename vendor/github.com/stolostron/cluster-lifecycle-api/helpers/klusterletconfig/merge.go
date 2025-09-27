package klusterletconfig

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/util/sets"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
)

var klusterletConfigMergeFuncs map[string]func(base, override interface{}) (interface{}, error) = map[string]func(
	base, override interface{}) (interface{}, error){
	"Registries":                             override,
	"PullSecret":                             override,
	"NodePlacement":                          override,
	"HubKubeAPIServerProxyConfig":            override,
	"HubKubeAPIServerURL":                    override,
	"HubKubeAPIServerCABundle":               override,
	"AppliedManifestWorkEvictionGracePeriod": override,
	"WorkStatusSyncInterval":                 override,
	"InstallMode":                            override,
	"HubKubeAPIServerConfig":                 mergeHubKubeAPIServerConfig,
	"FeatureGates":                           mergeFeatureGates,
	"ClusterClaimConfiguration":              mergeClusterClaimConfiguration,
	"MultipleHubsConfig":                     override,
	"RegistrationDriver":                     override,
}

func override(base, toMerge interface{}) (interface{}, error) {
	// if toMerge is not a zero value, return it
	if !reflect.ValueOf(toMerge).IsZero() {
		return toMerge, nil
	}
	return base, nil
}

// MergeKlusterletConfigs merges multiple KlusterletConfigs into a single KlusterletConfig.
func MergeKlusterletConfigs(klusterletconfigs ...*klusterletconfigv1alpha1.KlusterletConfig) (
	*klusterletconfigv1alpha1.KlusterletConfig, error) {
	// filter out the nil item in the list
	var filtered []*klusterletconfigv1alpha1.KlusterletConfig
	for _, kc := range klusterletconfigs {
		if kc != nil {
			filtered = append(filtered, kc.DeepCopy())
		}
	}
	klusterletconfigs = filtered

	if len(klusterletconfigs) == 0 {
		return nil, nil
	}

	if len(klusterletconfigs) == 1 {
		return klusterletconfigs[0], nil
	}

	// convert the list of KlusterletConfigSpecs to a list of KlusterletConfigSpecs
	var specs []*klusterletconfigv1alpha1.KlusterletConfigSpec
	for _, kc := range klusterletconfigs {
		specs = append(specs, &kc.Spec)
	}

	// Merge the KlusterletConfigSpecs
	// The first item in the list is the base for the merge
	// Run merge function for each field in the KlusterletConfigSpec from the first to the last
	// Every time we take the merge result as the base for the next merge
	merged := specs[0]
	for s := 1; s < len(specs); s++ {
		v := reflect.ValueOf(merged).Elem()
		t := v.Type()

		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			fieldName := field.Name

			if mf, ok := klusterletConfigMergeFuncs[fieldName]; !ok {
				return nil, fmt.Errorf("merge function for field %s is not provided", fieldName)
			} else {
				base := reflect.ValueOf(merged).Elem().Field(i).Interface()
				toMerge := reflect.ValueOf(specs[s]).Elem().Field(i).Interface()
				mergedValue, err := mf(base, toMerge)
				if err != nil {
					return nil, err
				}
				v.Field(i).Set(reflect.ValueOf(mergedValue))
			}
		}
	}

	return &klusterletconfigv1alpha1.KlusterletConfig{
		Spec: *merged,
	}, nil
}

func mergeHubKubeAPIServerConfig(base, toMerge interface{}) (interface{}, error) {
	old, ok := base.(*klusterletconfigv1alpha1.KubeAPIServerConfig)
	if !ok {
		return nil, fmt.Errorf("base is not of type KubeAPIServerConfig")
	}
	new, ok := toMerge.(*klusterletconfigv1alpha1.KubeAPIServerConfig)
	if !ok {
		return nil, fmt.Errorf("toMerge is not of type KubeAPIServerConfig")
	}
	if old == nil {
		return new, nil
	}
	if new == nil {
		return old, nil
	}

	config := new.DeepCopy()
	if len(new.URL) == 0 {
		config.URL = old.URL
	}
	if len(new.ProxyURL) == 0 {
		config.ProxyURL = old.ProxyURL
	}

	if len(config.ServerVerificationStrategy) == 0 {
		config.ServerVerificationStrategy = old.ServerVerificationStrategy
	}

	for _, caBundle := range old.TrustedCABundles {
		if !containsCA(config.TrustedCABundles, caBundle) {
			config.TrustedCABundles = append(config.TrustedCABundles, caBundle)
		}
	}

	return config, nil
}

func mergeFeatureGates(base, toMerge interface{}) (interface{}, error) {
	old, ok := base.([]operatorv1.FeatureGate)
	if !ok {
		return nil, fmt.Errorf("base is not of type FeatureGate")
	}
	new, ok := toMerge.([]operatorv1.FeatureGate)
	if !ok {
		return nil, fmt.Errorf("toMerge is not of type FeatureGate")
	}

	newMap := sets.New[string]()
	for _, f := range new {
		newMap.Insert(f.Feature)
	}

	for _, f := range old {
		if !newMap.Has(f.Feature) {
			new = append(new, f)
		}
	}
	return new, nil
}

func mergeClusterClaimConfiguration(base, toMerge interface{}) (interface{}, error) {
	old, ok := base.(*klusterletconfigv1alpha1.ClusterClaimConfiguration)
	if !ok {
		return nil, fmt.Errorf("base is not of type ClusterClaimConfiguration")
	}
	new, ok := toMerge.(*klusterletconfigv1alpha1.ClusterClaimConfiguration)
	if !ok {
		return nil, fmt.Errorf("toMerge is not of type ClusterClaimConfiguration")
	}

	if old == nil {
		return new, nil
	}
	if new == nil {
		return old, nil
	}

	config := new.DeepCopy()
	if config.MaxCustomClusterClaims < old.MaxCustomClusterClaims {
		config.MaxCustomClusterClaims = old.MaxCustomClusterClaims
	}

	return config, nil
}

func containsCA(bundles []klusterletconfigv1alpha1.CABundle, bundle klusterletconfigv1alpha1.CABundle) bool {
	for _, b := range bundles {
		if b.Name == bundle.Name {
			return true
		}
	}
	return false
}
