// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package framework

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterScope defines the expected state of the cluster at the end of setup.
// Teardown uses this to determine what cleanup is needed.
type ClusterScope int

const (
	// ScopeCreated means the ManagedCluster was created but no agent was deployed.
	// Teardown only needs to delete the cluster and wait for namespace cleanup.
	// No leader election handling is needed. This is fast and never flaky.
	ScopeCreated ClusterScope = iota

	// ScopeImported means the agent was deployed and import was applied.
	// Teardown needs leader election handling + full spoke cleanup.
	ScopeImported

	// ScopeAvailable means the cluster reached Available state.
	// Same teardown as ScopeImported.
	ScopeAvailable
)

// ClusterLifecycle manages the lifecycle of a ManagedCluster in tests.
// It provides smart teardown that handles leader election, cleanup ordering,
// and scope-appropriate wait logic.
type ClusterLifecycle struct {
	hub            *Hub
	name           string
	scope          ClusterScope
	hostedMode     bool
	hostingCluster string
}

// ForDefaultMode creates a lifecycle for a default-mode cluster where
// the agent is deployed (ScopeImported or ScopeAvailable).
func ForDefaultMode(hub *Hub, name string) *ClusterLifecycle {
	return &ClusterLifecycle{
		hub:   hub,
		name:  name,
		scope: ScopeAvailable,
	}
}

// ForCreatedOnly creates a lifecycle for a cluster where NO agent is deployed.
// This is for tests that only need the ManagedCluster resource (metadata, secret tests).
// Teardown is fast: no leader election, no agent namespace cleanup.
func ForCreatedOnly(hub *Hub, name string) *ClusterLifecycle {
	return &ClusterLifecycle{
		hub:   hub,
		name:  name,
		scope: ScopeCreated,
	}
}

// ForHostedMode creates a lifecycle for a hosted-mode cluster.
func ForHostedMode(hub *Hub, name, hostingCluster string) *ClusterLifecycle {
	return &ClusterLifecycle{
		hub:            hub,
		name:           name,
		scope:          ScopeAvailable,
		hostedMode:     true,
		hostingCluster: hostingCluster,
	}
}

// ForImportedMode creates a lifecycle for a cluster that is imported but we
// don't wait for Available status.
func ForImportedMode(hub *Hub, name string) *ClusterLifecycle {
	return &ClusterLifecycle{
		hub:   hub,
		name:  name,
		scope: ScopeImported,
	}
}

// Name returns the cluster name.
func (cl *ClusterLifecycle) Name() string {
	return cl.name
}

// Teardown handles all cleanup for the managed cluster.
// Based on the scope, it performs the appropriate cleanup steps:
//   - ScopeCreated: delete cluster + wait for hub cleanup only
//   - ScopeImported/ScopeAvailable: ensure agent ready + delete cluster + full cleanup
//   - HostedMode: hosted-specific cleanup
func (cl *ClusterLifecycle) Teardown() {
	if cl.hostedMode {
		cl.hub.AssertHostedClusterDeleted(cl.name, cl.hostingCluster)
		return
	}

	if cl.scope >= ScopeImported {
		cl.hub.EnsureAgentReady()
	}

	ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", cl.name), func() {
		err := cl.hub.ClusterClient.ClusterV1().ManagedClusters().Delete(
			context.TODO(), cl.name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	cl.hub.AssertClusterDeletedFromHub(cl.name)

	if cl.scope >= ScopeImported {
		cl.hub.AssertClusterDeletedFromSpoke()
	}
}

// DeleteCluster deletes the ManagedCluster with proper leader election handling.
// Use this when deleting a cluster in the test body (not in AfterEach).
// This is the recommended way to delete a cluster mid-test.
func (cl *ClusterLifecycle) DeleteCluster() {
	if cl.scope >= ScopeImported && !cl.hostedMode {
		cl.hub.EnsureAgentReady()
	}

	ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", cl.name), func() {
		err := cl.hub.ClusterClient.ClusterV1().ManagedClusters().Delete(
			context.TODO(), cl.name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})
}

// SetScope updates the scope, allowing tests to escalate the expected state.
// For example, a test may start with ScopeCreated but after triggering an import,
// can escalate to ScopeAvailable so teardown handles leader election.
func (cl *ClusterLifecycle) SetScope(scope ClusterScope) {
	cl.scope = scope
}

// WaitForClusterGone waits for the ManagedCluster to be fully deleted from hub.
func (cl *ClusterLifecycle) WaitForClusterGone() {
	cl.hub.AssertClusterDeletedFromHub(cl.name)
}

// WaitForFullCleanup waits for hub cleanup + spoke cleanup.
func (cl *ClusterLifecycle) WaitForFullCleanup() {
	cl.hub.AssertClusterDeletedFromHub(cl.name)
	cl.hub.AssertClusterDeletedFromSpoke()
}
