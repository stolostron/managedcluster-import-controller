// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"os"

	configv1 "github.com/openshift/api/config/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupTLSProfileWatcher sets up a watcher for TLS profile changes on OpenShift.
// When the TLS profile changes, the watcher triggers a graceful shutdown (os.Exit(0))
// so the pod restarts with the new TLS configuration.
//
// This ensures the agent-registration server picks up TLS profile changes without
// requiring manual pod restart.
//
// Returns nil on vanilla Kubernetes (no-op). On OpenShift, adds a runnable to the manager
// that will start the watcher after the manager's cache is started.
func SetupTLSProfileWatcher(ctx context.Context, mgr ctrl.Manager) error {
	// Only on OpenShift hub
	if !DeployOnOCP {
		klog.V(4).Info("Not running on OpenShift, skipping TLS profile watcher setup")
		return nil
	}

	// Add watcher as a runnable that starts with the manager (after cache is ready)
	return mgr.Add(&tlsProfileWatcherRunnable{mgr: mgr})
}

// tlsProfileWatcherRunnable implements manager.Runnable to set up the TLS watcher
// after the manager's cache has started
type tlsProfileWatcherRunnable struct {
	mgr ctrl.Manager
}

// NeedLeaderElection returns false so the watcher runs on all pods, not just the leader.
// This is necessary because each pod has its own agent-registration server that needs
// to restart when TLS profile changes (similar to webhook servers).
func (r *tlsProfileWatcherRunnable) NeedLeaderElection() bool {
	return false
}

func (r *tlsProfileWatcherRunnable) Start(ctx context.Context) error {
	// Wait for cache to be synced
	if !r.mgr.GetCache().WaitForCacheSync(ctx) {
		return nil // Context cancelled, manager is shutting down
	}

	klog.Info("Setting up TLS profile watcher after cache sync")

	// Fetch initial TLS profile
	profile, err := tlspkg.FetchAPIServerTLSProfile(ctx, r.mgr.GetClient())
	if err != nil {
		klog.Errorf("Failed to fetch initial TLS profile for watcher: %v", err)
		return err
	}

	klog.Infof("Initial TLS profile: minVersion=%v, ciphers=%d",
		profile.MinTLSVersion, len(profile.Ciphers))

	// Create watcher with callback that exits the process on profile change
	watcher := &tlspkg.SecurityProfileWatcher{
		Client:                r.mgr.GetClient(),
		InitialTLSProfileSpec: profile,
		OnProfileChange: func(ctx context.Context, oldSpec, newSpec configv1.TLSProfileSpec) {
			klog.Infof("TLS profile changed, triggering shutdown to reload: minVersion %v->%v, ciphers %d->%d",
				oldSpec.MinTLSVersion, newSpec.MinTLSVersion,
				len(oldSpec.Ciphers), len(newSpec.Ciphers))
			// Exit cleanly so the deployment controller restarts the pod with new config
			os.Exit(0)
		},
	}

	// Set up the watcher with the manager
	if err := watcher.SetupWithManager(r.mgr); err != nil {
		klog.Errorf("Failed to setup TLS profile watcher: %v", err)
		return err
	}

	klog.Info("TLS profile watcher successfully configured")

	// Block until context is done (keep the runnable alive)
	<-ctx.Done()
	return nil
}
