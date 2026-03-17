// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"crypto/tls"
	"time"

	ocinfrav1 "github.com/openshift/api/config/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetTLSConfigForServer returns TLS config for HTTPS server based on hub's OpenShift APIServer settings.
// Falls back to TLS 1.2 with no specific cipher suites on vanilla Kubernetes or if fetch fails.
func GetTLSConfigForServer(runtimeClient client.Client) *tls.Config {
	// Only on OpenShift hub
	if !DeployOnOCP {
		return &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	apiServer := &ocinfrav1.APIServer{}
	if err := runtimeClient.Get(ctx, client.ObjectKey{Name: "cluster"}, apiServer); err != nil {
		klog.V(4).Infof("Failed to get hub APIServer for TLS config, using TLS 1.2 fallback: %v", err)
		return &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	tlsConfig, err := buildTLSConfigFromProfile(apiServer.Spec.TLSSecurityProfile)
	if err != nil {
		klog.V(4).Infof("Failed to build TLS config from profile, using TLS 1.2 fallback: %v", err)
		return &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	return tlsConfig
}

// buildTLSConfigFromProfile converts OpenShift TLSSecurityProfile to crypto/tls.Config
// Based on the code examples from the TLS compliance hint document
func buildTLSConfigFromProfile(profile *ocinfrav1.TLSSecurityProfile) (*tls.Config, error) {
	profileSpec, err := getTLSProfileSpec(profile)
	if err != nil {
		return nil, err
	}

	minVersion := parseTLSVersion(string(profileSpec.MinTLSVersion))

	// Ensure minimum TLS version is at least 1.2 for security compliance
	// This satisfies gosec G402 security requirements
	if minVersion < tls.VersionTLS12 {
		klog.Warningf("TLS profile specified version %v which is below TLS 1.2, upgrading to TLS 1.2", profileSpec.MinTLSVersion)
		minVersion = tls.VersionTLS12
	}

	config := &tls.Config{
		MinVersion: minVersion,
	}

	// TLS 1.3 uses fixed cipher suites, don't set CipherSuites field
	if minVersion == tls.VersionTLS13 {
		config.MaxVersion = tls.VersionTLS13
	} else {
		// TLS 1.2 and below: parse cipher suites from profile
		cipherSuites := parseCipherSuites(profileSpec.Ciphers)
		if len(cipherSuites) > 0 {
			config.CipherSuites = cipherSuites
		}
	}

	return config, nil
}

// getTLSProfileSpec returns the TLSProfileSpec for the given profile type
func getTLSProfileSpec(profile *ocinfrav1.TLSSecurityProfile) (*ocinfrav1.TLSProfileSpec, error) {
	if profile == nil {
		// Default to Intermediate profile
		return ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType], nil
	}

	switch profile.Type {
	case ocinfrav1.TLSProfileOldType,
		ocinfrav1.TLSProfileIntermediateType,
		ocinfrav1.TLSProfileModernType:
		return ocinfrav1.TLSProfiles[profile.Type], nil
	case ocinfrav1.TLSProfileCustomType:
		if profile.Custom != nil {
			return &profile.Custom.TLSProfileSpec, nil
		}
		// Custom profile with no spec, fall back to Intermediate
		return ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType], nil
	default:
		// Unknown profile type, fall back to Intermediate
		return ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType], nil
	}
}

// parseTLSVersion converts OpenShift TLSProtocolVersion to crypto/tls constant
func parseTLSVersion(version string) uint16 {
	switch version {
	case "VersionTLS10", "TLSv1.0":
		return tls.VersionTLS10
	case "VersionTLS11", "TLSv1.1":
		return tls.VersionTLS11
	case "VersionTLS12", "TLSv1.2":
		return tls.VersionTLS12
	case "VersionTLS13", "TLSv1.3":
		return tls.VersionTLS13
	default:
		klog.V(4).Infof("Unknown TLS version %s, using TLS 1.2", version)
		return tls.VersionTLS12
	}
}

// parseCipherSuites converts OpenSSL-style and IANA cipher names to Go's crypto/tls constants.
// OpenShift TLS profiles use OpenSSL names for TLS 1.2 (e.g., "ECDHE-RSA-AES128-GCM-SHA256")
// and IANA names for TLS 1.3 (e.g., "TLS_AES_128_GCM_SHA256").
func parseCipherSuites(names []string) []uint16 {
	// Cipher map based on OpenShift TLS profiles and hint document examples
	cipherMap := map[string]uint16{
		// OpenSSL-style names (used in TLS 1.2 profiles)
		"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-RSA-AES256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-ECDSA-AES256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-RSA-CHACHA20-POLY1305":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		"ECDHE-ECDSA-CHACHA20-POLY1305": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		// Note: DHE-RSA cipher suites are not available in Go's crypto/tls package
		// "DHE-RSA-AES128-GCM-SHA256" and "DHE-RSA-AES256-GCM-SHA384" will be skipped
		// "DHE-RSA-AES128-GCM-SHA256": tls.TLS_DHE_RSA_WITH_AES_128_GCM_SHA256,
		// "DHE-RSA-AES256-GCM-SHA384": tls.TLS_DHE_RSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-RSA-AES128-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		"ECDHE-ECDSA-AES128-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		"ECDHE-RSA-AES128-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		"ECDHE-ECDSA-AES128-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		"ECDHE-RSA-AES256-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		"ECDHE-ECDSA-AES256-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		"AES128-GCM-SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		"AES256-GCM-SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		"AES128-SHA256":             tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		"AES128-SHA":                tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		"AES256-SHA":                tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		"DES-CBC3-SHA":              tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,

		// IANA-style names (used in TLS 1.3 profiles)
		"TLS_AES_128_GCM_SHA256":       tls.TLS_AES_128_GCM_SHA256,
		"TLS_AES_256_GCM_SHA384":       tls.TLS_AES_256_GCM_SHA384,
		"TLS_CHACHA20_POLY1305_SHA256": tls.TLS_CHACHA20_POLY1305_SHA256,
	}

	suites := make([]uint16, 0, len(names))
	for _, name := range names {
		if suite, ok := cipherMap[name]; ok {
			suites = append(suites, suite)
		} else {
			klog.V(4).Infof("Unknown cipher suite %s, skipping", name)
		}
	}
	return suites
}
