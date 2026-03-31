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

func TestGetTLSConfigForServer_NotOCP(t *testing.T) {
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	DeployOnOCP = false

	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	tlsConfig := GetTLSConfigForServer(client)
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2 fallback for non-OCP, got %v", tlsConfig.MinVersion)
	}
}

func TestGetTLSConfigForServer_Old(t *testing.T) {
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	DeployOnOCP = true

	apiServer := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{
			TLSSecurityProfile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileOldType,
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
	// Old profile allows TLS 1.0 per cluster admin configuration
	if tlsConfig.MinVersion != tls.VersionTLS10 {
		t.Errorf("Expected TLS 1.0 for Old profile, got %v", tlsConfig.MinVersion)
	}
	if len(tlsConfig.CipherSuites) == 0 {
		t.Errorf("Expected cipher suites for Old profile, got none")
	}
}

func TestGetTLSConfigForServer_Custom(t *testing.T) {
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	DeployOnOCP = true

	apiServer := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{
			TLSSecurityProfile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
				Custom: &ocinfrav1.CustomTLSProfile{
					TLSProfileSpec: ocinfrav1.TLSProfileSpec{
						MinTLSVersion: ocinfrav1.VersionTLS12,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
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
		t.Errorf("Expected TLS 1.2 for Custom profile, got %v", tlsConfig.MinVersion)
	}
	if len(tlsConfig.CipherSuites) != 1 {
		t.Errorf("Expected 1 cipher suite for Custom profile, got %d", len(tlsConfig.CipherSuites))
	}
}

func TestGetTLSConfigForServer_NilProfile(t *testing.T) {
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	DeployOnOCP = true

	// APIServer with no TLS profile set - should default to Intermediate
	apiServer := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{},
	}

	scheme := runtime.NewScheme()
	_ = ocinfrav1.Install(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(apiServer).
		Build()

	tlsConfig := GetTLSConfigForServer(client)
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2 (Intermediate default) for nil profile, got %v", tlsConfig.MinVersion)
	}
}

func TestGetTLSConfigForServer_MissingAPIServer(t *testing.T) {
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	DeployOnOCP = true

	scheme := runtime.NewScheme()
	_ = ocinfrav1.Install(scheme)

	// No APIServer object created
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	tlsConfig := GetTLSConfigForServer(client)
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2 fallback when APIServer missing, got %v", tlsConfig.MinVersion)
	}
}
