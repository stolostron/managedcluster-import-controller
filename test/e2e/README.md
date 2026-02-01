# E2E Tests

This directory contains end-to-end tests for the managed cluster import controller.

## Important Guidelines for Writing E2E Tests

### Common E2E Test Issues and Solutions

This section documents the common issues encountered in e2e tests and how to avoid them.

#### Issue 1: Force Delete Breaks Cleanup Chain

**Problem**: For self-managed clusters with short lease duration, force delete is triggered when the cluster becomes unavailable. Force delete breaks the normal cleanup chain because ManifestWorks are deleted from hub without actually cleaning up resources on the managed cluster.

**Solution**: Use `forceCleanupSelfManagedClusterResources()` in AfterEach for tests using `CreateManagedClusterWithShortLeaseDuration`.

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

#### Issue 2: Short Lease Duration Causes Flaky Failures

**Problem**: Using `CreateManagedClusterWithShortLeaseDuration` (10s lease) in tests can cause flaky failures:
- Recovery tests: Klusterlet doesn't have enough time to stabilize after re-deployment
- Destroy/detach tests: Short lease expires during test execution

**Symptom**:
```
Error: assert managed cluster available failed
Condition: ManagedClusterConditionAvailable Unknown
Reason: ManagedClusterLeaseUpdateStopped - Registration agent stopped updating its lease
```

**Solution**: Use `CreateManagedCluster` (60s lease) unless the test specifically requires short lease behavior.

#### Issue 3: Immediate-Import While Namespace Terminating

**Problem**: When removing klusterlet to simulate offline state, the agent namespace enters terminating state. If immediate-import annotation is added before namespace is fully deleted, the controller fails to create resources.

**Symptom**:
```
Error: secrets "bootstrap-hub-kubeconfig" is forbidden: unable to create new content
in namespace open-cluster-management-agent because it is being terminated
```

**Solution**: Wait for agent namespace to be fully deleted before adding immediate-import annotation.

```go
ginkgo.By("Should become offline after removing klusterlet", func() {
    err := util.RemoveKlusterlet(hubOperatorClient, "klusterlet")
    gomega.Expect(err).ToNot(gomega.HaveOccurred())
    assertManagedClusterAvailableUnknown(managedClusterName)
})

// CRITICAL: Wait for namespace deletion before triggering immediate-import
ginkgo.By("Wait for agent namespace to be deleted", func() {
    gomega.Eventually(func() error {
        _, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), "open-cluster-management-agent", metav1.GetOptions{})
        if errors.IsNotFound(err) {
            return nil
        }
        if err != nil {
            return err
        }
        return fmt.Errorf("namespace open-cluster-management-agent still exists")
    }, 5*time.Minute, 5*time.Second).Should(gomega.Succeed())
})

ginkgo.By("Should recover with immediate-import annotation", func() {
    err := util.SetImmediateImportAnnotation(hubClusterClient, managedClusterName, "")
    gomega.Expect(err).ToNot(gomega.HaveOccurred())
    assertManagedClusterAvailable(managedClusterName)
})
```

#### Issue 4: Agent Heartbeats After Klusterlet Removal

**Problem**: When testing that a cluster stays offline (Unknown) after removing klusterlet, the agent pod may still be running and sending heartbeats until the klusterlet operator finishes cleanup. This causes the cluster to become Available again unexpectedly.

**Symptom**:
```
Error: assert managed cluster available unknown consistently failed
cluster conditions: [...{ManagedClusterConditionAvailable True ...}]
```

**Solution**: Wait for agent namespace to be fully deleted after removing klusterlet before checking cluster status consistency.

```go
ginkgo.By("Should become offline after removing klusterlet", func() {
    err := util.RemoveKlusterlet(hubOperatorClient, "klusterlet")
    gomega.Expect(err).ToNot(gomega.HaveOccurred())
    assertManagedClusterAvailableUnknown(managedClusterName)
})

// CRITICAL: Wait for namespace deletion to ensure agent stops sending heartbeats
ginkgo.By("Wait for agent namespace to be deleted", func() {
    gomega.Eventually(func() error {
        _, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), "open-cluster-management-agent", metav1.GetOptions{})
        if errors.IsNotFound(err) {
            return nil
        }
        if err != nil {
            return err
        }
        return fmt.Errorf("namespace open-cluster-management-agent still exists")
    }, 5*time.Minute, 5*time.Second).Should(gomega.Succeed())
})

ginkgo.By("Should stay offline", func() {
    assertManagedClusterAvailableUnknownConsistently(managedClusterName, 30*time.Second)
})
```

---

## Self-Managed Cluster Cleanup Rule

**CRITICAL**: When writing or modifying e2e tests that use `CreateManagedClusterWithShortLeaseDuration` with `local-cluster=true` label, you **MUST** use `forceCleanupSelfManagedClusterResources()` in the `AfterEach` cleanup function instead of `assertManagedClusterDeleted()`.

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
