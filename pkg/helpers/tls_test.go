// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"crypto/tls"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetTLSConfigForServer_Modern(t *testing.T) {
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	DeployOnOCP = true

	apiServer := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{
			TLSSecurityProfile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileModernType,
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = ocinfrav1.Install(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(apiServer).
		Build()

	tlsConfig := GetTLSConfigForServer(client)
	if tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("Expected TLS 1.3 for Modern profile, got %v", tlsConfig.MinVersion)
	}
	if tlsConfig.MaxVersion != tls.VersionTLS13 {
		t.Errorf("Expected MaxVersion TLS 1.3 for Modern profile, got %v", tlsConfig.MaxVersion)
	}
	// TLS 1.3 should not set CipherSuites
	if len(tlsConfig.CipherSuites) != 0 {
		t.Errorf("Expected no cipher suites for TLS 1.3, got %v", tlsConfig.CipherSuites)
	}
}

func TestGetTLSConfigForServer_Intermediate(t *testing.T) {
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	DeployOnOCP = true

	apiServer := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{
			TLSSecurityProfile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileIntermediateType,
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = ocinfrav1.Install(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(apiServer).
		Build()

	tlsConfig := GetTLSConfigForServer(client)
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2 for Intermediate profile, got %v", tlsConfig.MinVersion)
	}
	// Should have cipher suites for TLS 1.2
	if len(tlsConfig.CipherSuites) == 0 {
		t.Errorf("Expected cipher suites for TLS 1.2 Intermediate profile, got none")
	}
}

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected uint16
	}{
		{"VersionTLS10", "VersionTLS10", tls.VersionTLS10},
		{"VersionTLS11", "VersionTLS11", tls.VersionTLS11},
		{"VersionTLS12", "VersionTLS12", tls.VersionTLS12},
		{"VersionTLS13", "VersionTLS13", tls.VersionTLS13},
		{"TLSv1.0", "TLSv1.0", tls.VersionTLS10},
		{"TLSv1.1", "TLSv1.1", tls.VersionTLS11},
		{"TLSv1.2", "TLSv1.2", tls.VersionTLS12},
		{"TLSv1.3", "TLSv1.3", tls.VersionTLS13},
		{"Unknown", "TLSv999", tls.VersionTLS12}, // Should fall back to TLS 1.2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTLSVersion(tt.version)
			if result != tt.expected {
				t.Errorf("parseTLSVersion(%s) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name     string
		ciphers  []string
		expected []uint16
	}{
		{
			name:     "OpenSSL style",
			ciphers:  []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES128-GCM-SHA256"},
			expected: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		},
		{
			name:     "IANA style (TLS 1.3)",
			ciphers:  []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"},
			expected: []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384},
		},
		{
			name:     "Unknown cipher",
			ciphers:  []string{"UNKNOWN_CIPHER"},
			expected: []uint16{},
		},
		{
			name:     "Mixed known and unknown",
			ciphers:  []string{"ECDHE-RSA-AES128-GCM-SHA256", "UNKNOWN_CIPHER", "TLS_AES_128_GCM_SHA256"},
			expected: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_AES_128_GCM_SHA256},
		},
		{
			name:     "Empty list",
			ciphers:  []string{},
			expected: []uint16{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCipherSuites(tt.ciphers)
			if len(result) != len(tt.expected) {
				t.Errorf("parseCipherSuites(%v) returned %d ciphers, want %d", tt.ciphers, len(result), len(tt.expected))
				return
			}
			for i, cipher := range result {
				if cipher != tt.expected[i] {
					t.Errorf("parseCipherSuites(%v)[%d] = %v, want %v", tt.ciphers, i, cipher, tt.expected[i])
				}
			}
		})
	}
}

func TestGetTLSProfileSpec(t *testing.T) {
	tests := []struct {
		name        string
		profile     *ocinfrav1.TLSSecurityProfile
		expectedMin string
	}{
		{
			name:        "Nil profile - defaults to Intermediate",
			profile:     nil,
			expectedMin: "VersionTLS12",
		},
		{
			name: "Modern profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileModernType,
			},
			expectedMin: "VersionTLS13",
		},
		{
			name: "Intermediate profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileIntermediateType,
			},
			expectedMin: "VersionTLS12",
		},
		{
			name: "Old profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileOldType,
			},
			expectedMin: "VersionTLS10",
		},
		{
			name: "Custom profile with spec",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
				Custom: &ocinfrav1.CustomTLSProfile{
					TLSProfileSpec: ocinfrav1.TLSProfileSpec{
						MinTLSVersion: ocinfrav1.VersionTLS13,
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					},
				},
			},
			expectedMin: "VersionTLS13",
		},
		{
			name: "Custom profile without spec - defaults to Intermediate",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
			},
			expectedMin: "VersionTLS12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := getTLSProfileSpec(tt.profile)
			if err != nil {
				t.Errorf("getTLSProfileSpec() error = %v", err)
				return
			}
			if string(spec.MinTLSVersion) != tt.expectedMin {
				t.Errorf("getTLSProfileSpec() MinTLSVersion = %v, want %v", spec.MinTLSVersion, tt.expectedMin)
			}
		})
	}
}

func TestBuildTLSConfigFromProfile_Modern(t *testing.T) {
	profile := &ocinfrav1.TLSSecurityProfile{
		Type: ocinfrav1.TLSProfileModernType,
	}

	config, err := buildTLSConfigFromProfile(profile)
	if err != nil {
		t.Errorf("buildTLSConfigFromProfile() error = %v", err)
		return
	}

	if config.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %v, want %v", config.MinVersion, tls.VersionTLS13)
	}

	if config.MaxVersion != tls.VersionTLS13 {
		t.Errorf("MaxVersion = %v, want %v", config.MaxVersion, tls.VersionTLS13)
	}

	// TLS 1.3 should not have CipherSuites set
	if len(config.CipherSuites) != 0 {
		t.Errorf("TLS 1.3 should not have CipherSuites, got %v", config.CipherSuites)
	}
}

func TestBuildTLSConfigFromProfile_Intermediate(t *testing.T) {
	profile := &ocinfrav1.TLSSecurityProfile{
		Type: ocinfrav1.TLSProfileIntermediateType,
	}

	config, err := buildTLSConfigFromProfile(profile)
	if err != nil {
		t.Errorf("buildTLSConfigFromProfile() error = %v", err)
		return
	}

	if config.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want %v", config.MinVersion, tls.VersionTLS12)
	}

	// Intermediate profile should have cipher suites
	if len(config.CipherSuites) == 0 {
		t.Errorf("Intermediate profile should have cipher suites")
	}
}

func TestBuildTLSConfigFromProfile_SecurityUpgrade(t *testing.T) {
	// Test that insecure TLS versions (1.0, 1.1) are automatically upgraded to TLS 1.2
	tests := []struct {
		name        string
		minVersion  ocinfrav1.TLSProtocolVersion
		expectedMin uint16
	}{
		{
			name:        "TLS 1.0 upgraded to 1.2",
			minVersion:  ocinfrav1.VersionTLS10,
			expectedMin: tls.VersionTLS12,
		},
		{
			name:        "TLS 1.1 upgraded to 1.2",
			minVersion:  ocinfrav1.VersionTLS11,
			expectedMin: tls.VersionTLS12,
		},
		{
			name:        "TLS 1.2 unchanged",
			minVersion:  ocinfrav1.VersionTLS12,
			expectedMin: tls.VersionTLS12,
		},
		{
			name:        "TLS 1.3 unchanged",
			minVersion:  ocinfrav1.VersionTLS13,
			expectedMin: tls.VersionTLS13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
				Custom: &ocinfrav1.CustomTLSProfile{
					TLSProfileSpec: ocinfrav1.TLSProfileSpec{
						MinTLSVersion: tt.minVersion,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			}

			config, err := buildTLSConfigFromProfile(profile)
			if err != nil {
				t.Errorf("buildTLSConfigFromProfile() error = %v", err)
				return
			}

			if config.MinVersion != tt.expectedMin {
				t.Errorf("MinVersion = %v, want %v", config.MinVersion, tt.expectedMin)
			}
		})
	}
}
