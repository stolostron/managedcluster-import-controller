package klusterletconfig

import (
	"reflect"
	"testing"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

func TestCheckAllFiledHasMergeFunc(test *testing.T) {
	// TestCheckAllFiledHasMergeFunc tests if all fields in the KlusterletConfigSpec have a corresponding merge function in the merge.go file.
	// When adding a new field to the KlusterletConfigSpec, if the merge function is not provided, it will fail in the unit tests.

	kcSpec := &klusterletconfigv1alpha1.KlusterletConfigSpec{}
	v := reflect.ValueOf(kcSpec).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldName := field.Name

		if _, ok := klusterletConfigMergeFuncs[fieldName]; !ok {
			test.Errorf("merge function for field %s is not provided", fieldName)
		}
	}
}

func TestMergeKlusterletConfigs(test *testing.T) {
	testcases := []struct {
		name     string
		kcs      []*klusterletconfigv1alpha1.KlusterletConfig
		expected *klusterletconfigv1alpha1.KlusterletConfig
	}{
		{
			name: "override strategy: override the base value if next KlusterletConfig in the list has a non-zero value",
			kcs: []*klusterletconfigv1alpha1.KlusterletConfig{
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						Registries: []klusterletconfigv1alpha1.Registries{
							{
								Mirror: "mirror",
								Source: "source",
							},
						},
						PullSecret: corev1.ObjectReference{
							Name: "pull-secret",
						},
						NodePlacement: &operatorv1.NodePlacement{
							NodeSelector: map[string]string{
								"key": "value",
							},
						},
						HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
							HTTPProxy:  "http://proxy",
							HTTPSProxy: "https://proxy",
						},
						HubKubeAPIServerURL:      "https://hub",
						HubKubeAPIServerCABundle: []byte("ca-bundle"),
					},
				},
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						Registries: []klusterletconfigv1alpha1.Registries{
							{
								Mirror: "mirror2",
								Source: "source2",
							},
						},
						PullSecret: corev1.ObjectReference{
							Name: "pull-secret2",
						},
						NodePlacement: &operatorv1.NodePlacement{
							NodeSelector: map[string]string{
								"key2": "value2",
							},
						},
						HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
							HTTPProxy:  "http://proxy2",
							HTTPSProxy: "https://proxy2",
						},
						HubKubeAPIServerURL:      "https://hub2",
						HubKubeAPIServerCABundle: []byte("ca-bundle2"),
					},
				},
			},
			expected: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					Registries: []klusterletconfigv1alpha1.Registries{
						{
							Mirror: "mirror2",
							Source: "source2",
						},
					},
					PullSecret: corev1.ObjectReference{
						Name: "pull-secret2",
					},
					NodePlacement: &operatorv1.NodePlacement{
						NodeSelector: map[string]string{
							"key2": "value2",
						},
					},
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPProxy:  "http://proxy2",
						HTTPSProxy: "https://proxy2",
					},
					HubKubeAPIServerURL:      "https://hub2",
					HubKubeAPIServerCABundle: []byte("ca-bundle2"),
				},
			},
		},
		{
			name: "override strategy: flow to the next if the first is zero",
			kcs: []*klusterletconfigv1alpha1.KlusterletConfig{
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						Registries:                  nil,
						PullSecret:                  corev1.ObjectReference{},
						NodePlacement:               nil,
						HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{},
						HubKubeAPIServerURL:         "",
						HubKubeAPIServerCABundle:    nil,
					},
				},
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						Registries: []klusterletconfigv1alpha1.Registries{
							{
								Mirror: "mirror2",
								Source: "source2",
							},
						},
						PullSecret: corev1.ObjectReference{
							Name: "pull-secret2",
						},
						NodePlacement: &operatorv1.NodePlacement{
							NodeSelector: map[string]string{
								"key2": "value2",
							},
						},
						HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
							HTTPProxy:  "http://proxy2",
							HTTPSProxy: "https://proxy2",
						},
						HubKubeAPIServerURL:      "https://hub2",
						HubKubeAPIServerCABundle: []byte("ca-bundle2"),
					},
				},
			},
			expected: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					Registries: []klusterletconfigv1alpha1.Registries{
						{
							Mirror: "mirror2",
							Source: "source2",
						},
					},
					PullSecret: corev1.ObjectReference{
						Name: "pull-secret2",
					},
					NodePlacement: &operatorv1.NodePlacement{
						NodeSelector: map[string]string{
							"key2": "value2",
						},
					},
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPProxy:  "http://proxy2",
						HTTPSProxy: "https://proxy2",
					},
					HubKubeAPIServerURL:      "https://hub2",
					HubKubeAPIServerCABundle: []byte("ca-bundle2"),
				},
			},
		},
		{
			name: "migrate deprecated fields to HubKubeAPIServerConfig when both are present",
			kcs: []*klusterletconfigv1alpha1.KlusterletConfig{
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
							ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
						},
					},
				},
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerURL: "https://hub-api-server",
						HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
							HTTPSProxy: "https://proxy-server",
						},
						HubKubeAPIServerCABundle: []byte("ca-bundle-data"),
					},
				},
			},
			expected: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						URL:                        "https://hub-api-server",
						ProxyURL:                   "https://proxy-server",
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
					},
					HubKubeAPIServerURL: "https://hub-api-server",
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://proxy-server",
					},
					HubKubeAPIServerCABundle: []byte("ca-bundle-data"),
				},
			},
		},
		{
			name: "do not override HubKubeAPIServerConfig fields if already set",
			kcs: []*klusterletconfigv1alpha1.KlusterletConfig{
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
							URL:                        "https://existing-hub",
							ProxyURL:                   "https://existing-proxy",
							ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
						},
					},
				},
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerURL: "https://deprecated-hub",
						HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
							HTTPSProxy: "https://deprecated-proxy",
						},
					},
				},
			},
			expected: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						URL:                        "https://existing-hub",
						ProxyURL:                   "https://existing-proxy",
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
					},
					HubKubeAPIServerURL: "https://deprecated-hub",
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://deprecated-proxy",
					},
				},
			},
		},
		{
			name: "prefer HTTPSProxy over HTTPProxy when migrating",
			kcs: []*klusterletconfigv1alpha1.KlusterletConfig{
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{},
					},
				},
				{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
							HTTPProxy:  "http://http-proxy",
							HTTPSProxy: "https://https-proxy",
						},
					},
				},
			},
			expected: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ProxyURL: "https://https-proxy",
					},
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPProxy:  "http://http-proxy",
						HTTPSProxy: "https://https-proxy",
					},
				},
			},
		},
	}

	for _, testcase := range testcases {
		test.Run(testcase.name, func(test *testing.T) {
			merged, err := MergeKlusterletConfigs(testcase.kcs...)
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(merged, testcase.expected) {
				test.Errorf("expected: %v, got: %v", testcase.expected, merged)
			}
		})
	}
}

func TestMergeFeatureGates(t *testing.T) {
	testcases := []struct {
		name   string
		old    *klusterletconfigv1alpha1.KlusterletConfig
		new    *klusterletconfigv1alpha1.KlusterletConfig
		expect []operatorv1.FeatureGate
	}{
		{
			name: "empty new",
			old: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{
						{
							Feature: "feature1",
							Mode:    operatorv1.FeatureGateModeTypeEnable,
						},
					},
				},
			},
			new: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{},
				},
			},
			expect: []operatorv1.FeatureGate{
				{
					Feature: "feature1",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				},
			},
		},
		{
			name: "empty old",
			old: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{},
				},
			},
			new: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{
						{
							Feature: "feature2",
							Mode:    operatorv1.FeatureGateModeTypeEnable,
						},
					},
				},
			},
			expect: []operatorv1.FeatureGate{
				{
					Feature: "feature2",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				},
			},
		},
		{
			name: "merge the two",
			old: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{
						{
							Feature: "feature1",
							Mode:    operatorv1.FeatureGateModeTypeEnable,
						},
						{
							Feature: "feature2",
							Mode:    operatorv1.FeatureGateModeTypeEnable,
						},
					},
				},
			},
			new: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{
						{
							Feature: "feature2",
							Mode:    operatorv1.FeatureGateModeTypeDisable,
						},
					},
				},
			},
			expect: []operatorv1.FeatureGate{
				{
					Feature: "feature2",
					Mode:    operatorv1.FeatureGateModeTypeDisable,
				},
				{
					Feature: "feature1",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				},
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			merged, err := mergeFeatureGates(testcase.old.Spec.FeatureGates, testcase.new.Spec.FeatureGates)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(merged, testcase.expect) {
				t.Errorf("expected: %v, got: %v", testcase.expect, merged)
			}
		})
	}
}

func TestMergeClusterClaimConfiguration(t *testing.T) {
	testcases := []struct {
		name   string
		old    *klusterletconfigv1alpha1.KlusterletConfig
		new    *klusterletconfigv1alpha1.KlusterletConfig
		expect *klusterletconfigv1alpha1.ClusterClaimConfiguration
	}{
		{
			name: "empty new",
			old: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					ClusterClaimConfiguration: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
						MaxCustomClusterClaims: 10,
					},
				},
			},
			new: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{},
			},
			expect: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
				MaxCustomClusterClaims: 10,
			},
		},
		{
			name: "empty old",
			old: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{},
			},
			new: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					ClusterClaimConfiguration: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
						MaxCustomClusterClaims: 10,
					},
				},
			},
			expect: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
				MaxCustomClusterClaims: 10,
			},
		},
		{
			name: "merge the two, old max",
			old: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					ClusterClaimConfiguration: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
						MaxCustomClusterClaims: 15,
					},
				},
			},
			new: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					ClusterClaimConfiguration: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
						MaxCustomClusterClaims: 10,
					},
				},
			},
			expect: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
				MaxCustomClusterClaims: 15,
			},
		},
		{
			name: "merge the two, new max",
			old: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					ClusterClaimConfiguration: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
						MaxCustomClusterClaims: 15,
					},
				},
			},
			new: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					ClusterClaimConfiguration: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
						MaxCustomClusterClaims: 20,
					},
				},
			},
			expect: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
				MaxCustomClusterClaims: 20,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			merged, err := mergeClusterClaimConfiguration(testcase.old.Spec.ClusterClaimConfiguration,
				testcase.new.Spec.ClusterClaimConfiguration)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(merged, testcase.expect) {
				t.Errorf("expected: %v, got: %v", testcase.expect, merged)
			}
		})
	}
}

func TestMergeHubKubeAPIServerConfig(test *testing.T) {
	testcases := []struct {
		name     string
		old      *klusterletconfigv1alpha1.KubeAPIServerConfig
		new      *klusterletconfigv1alpha1.KubeAPIServerConfig
		expected *klusterletconfigv1alpha1.KubeAPIServerConfig
	}{
		{
			name: "old is nil",
			old:  nil,
			new: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy",
			},
			expected: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy",
			},
		},
		{
			name: "new is nil",
			old: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy",
			},
			new: nil,
			expected: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy",
			},
		},
		{
			name: "override all",
			old: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy",
			},
			new: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://new-apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle-new",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns2",
							Name:      "n2",
						},
					},
				},
				ProxyURL: "http://proxy-new",
			},
			expected: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://new-apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle-new",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns2",
							Name:      "n2",
						},
					},
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy-new",
			},
		},
		{
			name: "override all except ServerVerificationStrategy",
			old: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy",
			},
			new: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://new-apiserver.com",
				ServerVerificationStrategy: "",
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle-new",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns2",
							Name:      "n2",
						},
					},
				},
				ProxyURL: "http://proxy-new",
			},
			expected: &klusterletconfigv1alpha1.KubeAPIServerConfig{
				URL:                        "http://new-apiserver.com",
				ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
				TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
					{
						Name: "ca-bundle-new",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns2",
							Name:      "n2",
						},
					},
					{
						Name: "ca-bundle",
						CABundle: klusterletconfigv1alpha1.ConfigMapReference{
							Namespace: "ns1",
							Name:      "n1",
						},
					},
				},
				ProxyURL: "http://proxy-new",
			},
		},
	}

	for _, tc := range testcases {
		test.Run(tc.name, func(test *testing.T) {
			merged, err := mergeHubKubeAPIServerConfig(tc.old, tc.new)
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(merged, tc.expected) {
				test.Errorf("expected: %v, got: %v", tc.expected, merged)
			}
		})
	}
}
