# E2E Tests

This directory contains end-to-end tests for the managed cluster import controller.

## Important Guidelines for Writing E2E Tests

### Self-Managed Cluster Cleanup Rule

**CRITICAL**: When writing or modifying e2e tests that use `CreateManagedClusterWithShortLeaseDuration` with `local-cluster=true` label, you **MUST** use `forceCleanupSelfManagedClusterResources()` in the `AfterEach` cleanup function instead of `assertManagedClusterDeleted()`.

```go
// CORRECT - for tests using CreateManagedClusterWithShortLeaseDuration
ginkgo.AfterEach(func() {
    forceCleanupSelfManagedClusterResources(clusterName)
})

// INCORRECT - will cause flaky test failures
ginkgo.AfterEach(func() {
    assertManagedClusterDeleted(clusterName)  // DO NOT use this for short lease duration tests
})
```

## Why Force Delete Causes Resource Residue

> **Reference**: This issue was identified and fixed in [PR #90](https://github.com/xuezhaojun/managedcluster-import-controller/pull/90). See the PR description for additional context and discussion.

### Background

For self-managed clusters (Hub=Spoke, with `local-cluster=true` label), the import controller creates ManifestWorks to deploy the Klusterlet and its CRD on the managed cluster. When a cluster is deleted, these resources need to be cleaned up properly.

### Normal Cleanup Chain

Under normal circumstances, the cleanup follows this chain:

```
Delete ManagedCluster
    ↓
Delete klusterlet ManifestWork (DeleteOption=Orphan)
    ↓
Delete klusterlet-crds ManifestWork (DeleteOption=Foreground)
    ↓
Work-agent deletes Klusterlet CRD from managed cluster
    ↓
Klusterlet CR is cascade deleted (due to CRD deletion)
    ↓
Klusterlet Operator cleanup is triggered
    ↓
Agent namespace (open-cluster-management-agent) is deleted
```

### What Happens with Force Delete

When using `CreateManagedClusterWithShortLeaseDuration`, the cluster's `LeaseDurationSeconds` is set to 10 seconds. This causes:

1. After 10 seconds without heartbeat, the cluster becomes **unavailable**
2. The `resourcecleanup_controller` detects the unavailable state via `IsClusterUnavailable()`
3. `clusterNeedForceDelete()` returns `true`
4. `ForceDeleteManifestWork()` is called, which:
   - Removes finalizers from ManifestWorks on the hub
   - Deletes ManifestWorks from the hub
   - **BUT does NOT delete resources on the managed cluster**

This breaks the cleanup chain:

```
Force Delete triggered
    ↓
ManifestWorks force deleted from hub (finalizers removed)
    ↓
CRD is NOT deleted from managed cluster (work-agent didn't process it)
    ↓
Klusterlet CR still exists
    ↓
Klusterlet Operator cleanup is NOT triggered
    ↓
Agent namespace remains! ← PROBLEM
```

### The Solution: forceCleanupSelfManagedClusterResources()

The `forceCleanupSelfManagedClusterResources()` function manually triggers the cleanup chain that force delete breaks:

1. Delete ManagedCluster (if still exists)
2. Delete Klusterlet CR → triggers Klusterlet Operator's `managedReconcile.clean()`
3. Wait for agent namespace deletion (done by Klusterlet Operator)
4. Delete Klusterlet CRD
5. Wait for cluster namespace deletion

### Code Reference

The force delete logic is in:
- `pkg/controller/managedcluster/resourcecleanup_controller.go` - `clusterNeedForceDelete()`, `ForceDeleteManifestWork()`
- `pkg/helpers/managedclusterhelper.go` - `IsClusterUnavailable()`

The Klusterlet Operator cleanup logic is in:
- OCM Registration Operator: `klusterletCleanupController` → `managedReconcile.clean()`

## Test Organization

Tests are organized by their cluster creation method:

| Cluster Creation Method | AfterEach Cleanup Function | Use Case |
|------------------------|---------------------------|----------|
| `CreateManagedCluster` | `assertManagedClusterDeleted` | Normal tests without force delete |
| `CreateManagedClusterWithShortLeaseDuration` + `local-cluster=true` | `forceCleanupSelfManagedClusterResources` | Tests that need to test force delete behavior or recovery scenarios |

## Running E2E Tests

```bash
# Run all e2e tests
make e2e-test

# Run specific test suite
make e2e-test GINKGO_FOCUS="Importing a managed cluster"
```
