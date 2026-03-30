// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"crypto/tls"
	"time"

	ocinfrav1 "github.com/openshift/api/config/v1"
	tlsprofile "github.com/stolostron/cluster-lifecycle-api/helpers/tlsprofile"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetTLSConfigForServer returns TLS config for HTTPS server based on hub's OpenShift APIServer settings.
// Falls back to TLS 1.2 with no specific cipher suites on vanilla Kubernetes or if fetch fails.
// Uses client.Reader to avoid dependency on manager's cache (can be called before manager starts).
// Delegates TLS profile conversion to cluster-lifecycle-api/helpers/tlsprofile.
func GetTLSConfigForServer(runtimeClient client.Reader) *tls.Config {
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

	return tlsprofile.ConvertTLSProfileToConfig(apiServer.Spec.TLSSecurityProfile)
}
