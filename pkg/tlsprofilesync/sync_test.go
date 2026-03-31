// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tlsprofilesync

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestBuildConfigMapData(t *testing.T) {
	tests := []struct {
		name            string
		profile         *configv1.TLSSecurityProfile
		wantProfileType string
		wantMinVersion  string
		wantHasCiphers  bool
	}{
		{
			name:            "nil profile defaults to Intermediate",
			profile:         nil,
			wantProfileType: "Intermediate",
			wantMinVersion:  "VersionTLS12",
			wantHasCiphers:  true,
		},
		{
			name: "Modern profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			wantProfileType: "Modern",
			wantMinVersion:  "VersionTLS13",
			wantHasCiphers:  true, // OpenShift Modern profile lists TLS 1.3 ciphers
		},
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			wantProfileType: "Intermediate",
			wantMinVersion:  "VersionTLS12",
			wantHasCiphers:  true,
		},
		{
			name: "Old profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			wantProfileType: "Old",
			wantMinVersion:  "VersionTLS10",
			wantHasCiphers:  true,
		},
		{
			name: "Custom profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS13,
						Ciphers:       []string{},
					},
				},
			},
			wantProfileType: "Custom",
			wantMinVersion:  "VersionTLS13",
			wantHasCiphers:  false,
		},
		{
			name: "Custom profile with nil custom falls back to Intermediate",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
			},
			wantProfileType: "Custom",
			wantMinVersion:  "VersionTLS12",
			wantHasCiphers:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildConfigMapData(tt.profile)

			if data["profileType"] != tt.wantProfileType {
				t.Errorf("profileType = %q, want %q", data["profileType"], tt.wantProfileType)
			}
			if data["minTLSVersion"] != tt.wantMinVersion {
				t.Errorf("minTLSVersion = %q, want %q", data["minTLSVersion"], tt.wantMinVersion)
			}
			hasCiphers := data["cipherSuites"] != ""
			if hasCiphers != tt.wantHasCiphers {
				t.Errorf("hasCiphers = %v, want %v (cipherSuites=%q)",
					hasCiphers, tt.wantHasCiphers, data["cipherSuites"])
			}
		})
	}
}

func TestBuildConfigMapData_CiphersAreIANA(t *testing.T) {
	data := buildConfigMapData(&configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileIntermediateType,
	})

	ciphers := data["cipherSuites"]
	if ciphers == "" {
		t.Fatal("expected cipher suites for Intermediate profile")
	}

	// IANA format ciphers start with "TLS_"
	for _, c := range splitNonEmpty(ciphers) {
		if !isIANACipher(c) {
			t.Errorf("cipher %q is not in IANA format (should start with TLS_)", c)
		}
	}
}

func isIANACipher(cipher string) bool {
	return len(cipher) > 4 && cipher[:4] == "TLS_"
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range splitComma(s) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitComma(s string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func TestSyncConfigMap_CreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	namespace := "test-ns"
	fakeClient := fake.NewSimpleClientset()

	reconciler := &tlsProfileSyncReconciler{
		kubeClient: fakeClient,
		namespace:  namespace,
	}

	// First sync should create the ConfigMap
	data := map[string]string{
		"minTLSVersion": "VersionTLS12",
		"cipherSuites":  "TLS_AES_128_GCM_SHA256",
		"profileType":   "Intermediate",
	}
	if err := reconciler.syncConfigMap(ctx, data); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	cm, err := fakeClient.CoreV1().ConfigMaps(namespace).Get(ctx, ConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get after create failed: %v", err)
	}
	if cm.Data["minTLSVersion"] != "VersionTLS12" {
		t.Errorf("minTLSVersion = %q, want VersionTLS12", cm.Data["minTLSVersion"])
	}

	// Second sync should update the ConfigMap
	data2 := map[string]string{
		"minTLSVersion": "VersionTLS13",
		"cipherSuites":  "",
		"profileType":   "Modern",
	}
	if err := reconciler.syncConfigMap(ctx, data2); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	cm, err = fakeClient.CoreV1().ConfigMaps(namespace).Get(ctx, ConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get after update failed: %v", err)
	}
	if cm.Data["minTLSVersion"] != "VersionTLS13" {
		t.Errorf("minTLSVersion = %q, want VersionTLS13", cm.Data["minTLSVersion"])
	}
	if cm.Data["profileType"] != "Modern" {
		t.Errorf("profileType = %q, want Modern", cm.Data["profileType"])
	}
}

func TestSyncConfigMap_ExistingConfigMap(t *testing.T) {
	ctx := context.Background()
	namespace := "test-ns"

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"minTLSVersion": "VersionTLS12",
			"cipherSuites":  "old-cipher",
			"profileType":   "Intermediate",
		},
	}
	fakeClient := fake.NewSimpleClientset(existing)

	reconciler := &tlsProfileSyncReconciler{
		kubeClient: fakeClient,
		namespace:  namespace,
	}

	data := map[string]string{
		"minTLSVersion": "VersionTLS13",
		"cipherSuites":  "",
		"profileType":   "Modern",
	}
	if err := reconciler.syncConfigMap(ctx, data); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	cm, err := fakeClient.CoreV1().ConfigMaps(namespace).Get(ctx, ConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if cm.Data["profileType"] != "Modern" {
		t.Errorf("profileType = %q, want Modern", cm.Data["profileType"])
	}
}
