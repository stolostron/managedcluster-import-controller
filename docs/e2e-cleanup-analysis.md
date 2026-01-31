# E2E Test Cleanup Analysis for Self-Managed Clusters

## Problem Summary

E2E tests for self-managed clusters (`local-cluster=true`) were failing during cleanup with timeout errors. This document analyzes two related problems:

1. **Problem 0**: Why `assertManagedClusterDeleted` doesn't work for self-managed clusters
2. **Problem 1**: Why simply adding klusterlet deletion causes CRD timeout

---

## Architecture Overview

For self-managed clusters (Hub = Spoke), the klusterlet deployment uses the ManifestWork mechanism:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Hub Cluster = Spoke Cluster                          │
│                                                                              │
│  ┌─────────────────────┐     ┌──────────────────────────────────────────┐   │
│  │  import-controller  │────▶│  ManifestWork (klusterlet-crds)          │   │
│  │                     │     │  ManifestWork (klusterlet)               │   │
│  └─────────────────────┘     │    └─▶ DeleteOption: Orphan              │   │
│                              └──────────────────────────────────────────┘   │
│                                           │                                  │
│                                           │ (work-agent applies)             │
│                                           ▼                                  │
│  ┌─────────────────────┐     ┌──────────────────────────────────────────┐   │
│  │  work-agent         │────▶│  AppliedManifestWork                     │   │
│  │  (in klusterlet-    │     │    └─▶ ownerRef ──▶ Klusterlet CRD/CR    │   │
│  │   agent pod)        │     └──────────────────────────────────────────┘   │
│  └─────────────────────┘                                                     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Code References

| Component | File | Description |
|-----------|------|-------------|
| ManifestWork Controller | [manifestwork_controller.go:194-196](../pkg/controller/manifestwork/manifestwork_controller.go#L194-L196) | Sets DeleteOption=Orphan for klusterlet ManifestWork |
| Resource Cleanup Controller | [resourcecleanup_controller.go](../pkg/controller/resourcecleanup/resourcecleanup_controller.go) | Cleans up ManifestWorks when cluster is deleted |
| Cluster Namespace Deletion | [clusternamespacedeletion/controller.go](../pkg/controller/clusternamespacedeletion/controller.go) | Deletes cluster namespace |
| work-agent (OCM) | [manifestwork_finalize_controller.go](https://github.com/open-cluster-management-io/ocm/blob/main/pkg/work/spoke/controllers/finalizercontroller/manifestwork_finalize_controller.go) | Handles ManifestWork deletion |
| Backup Eviction (OCM) | [unmanaged_appliedmanifestwork_controller.go](https://github.com/open-cluster-management-io/ocm/blob/main/pkg/work/spoke/controllers/finalizercontroller/unmanaged_appliedmanifestwork_controller.go) | Evicts orphaned AppliedManifestWork after 60 minutes |

---

## Root Cause: ManifestWork DeleteOption = Orphan

The klusterlet ManifestWork has a **DeleteOption** that determines what happens to applied resources:

```go
// pkg/controller/manifestwork/manifestwork_controller.go:194-196
DeleteOption: &workv1.DeleteOption{
    PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,  // Default: Orphan!
},

// Only use Foreground when there is no CRD yaml (line 204-212)
if len(crdYaml) == 0 {
    klwork.Spec.DeleteOption = &workv1.DeleteOption{
        PropagationPolicy: workv1.DeletePropagationPolicyTypeForeground,
    }
}
```

| DeleteOption | When Used | Effect on Klusterlet CR |
|--------------|-----------|------------------------|
| **Orphan** | CRD yaml exists (default) | Klusterlet CR is **preserved** when ManifestWork is deleted |
| Foreground | No CRD yaml | Klusterlet CR is **deleted** when ManifestWork is deleted |

**Why Orphan is Used:** CRDs have special deletion semantics (instances must be deleted first). Direct cascade deletion of CRDs can cause issues, so Orphan is intentional when CRD yaml exists.

---

## Problem 0: Why assertManagedClusterDeleted Doesn't Work

### What the Test Waits For

```go
// test/e2e/e2e_suite_test.go
func assertManagedClusterDeletedFromSpoke() {
    // Wait for namespace "open-cluster-management-agent" to be deleted
    // Wait for CRD "klusterlets.operator.open-cluster-management.io" to be deleted
}
```

### Why It Fails

When ManagedCluster is deleted:

1. **resourcecleanup_controller** deletes ManifestWorks
2. ManifestWork has `DeleteOption=Orphan` → Klusterlet CR is **orphaned** (not deleted)
3. Klusterlet CR has no DeletionTimestamp
4. Klusterlet Cleanup Controller does nothing (checks `DeletionTimestamp.IsZero()`)
5. klusterlet-agent keeps running → Namespace stays active
6. E2E test times out

### Why This Is a Flaky Error

The error is flaky because of a race condition between:
- **clusternamespacedeletion controller** (deletes namespace)
- **work-agent** (processes ManifestWork deletion)

#### clusternamespacedeletion Controller Analysis

Looking at `pkg/controller/clusternamespacedeletion/controller.go:112-176`:

```go
// Resources checked before namespace deletion:
// ✅ Addons (line 112-119)
// ✅ HostedClusters (line 121-131)
// ✅ ClusterDeployments (line 133-143)
// ✅ InfraEnvs (line 145-155)
// ✅ CAPI Clusters (line 157-166)
// ✅ Pods (line 168-176)
// ❌ ManifestWorks - NOT checked!

err = r.client.Delete(ctx, ns)  // Line 178: Deletes namespace
```

**The controller does NOT wait for ManifestWorks to be processed before deleting the namespace.**

#### work-agent Behavior on Cascade Deletion

When namespace is cascade deleted, the ManifestWork is **immediately deleted** (not just marked for deletion):

```go
// ocm/pkg/work/spoke/controllers/finalizercontroller/manifestwork_finalize_controller.go:75-94
func (m *ManifestWorkFinalizeController) sync(...) error {
    manifestWork, err := m.manifestWorkLister.Get(manifestWorkName)

    switch {
    case errors.IsNotFound(err):
        // ManifestWork is gone! Controller does nothing.
        return nil  // ← No cleanup happens!
    case !manifestWork.DeletionTimestamp.IsZero():
        // Only this path deletes AppliedManifestWork
        err := m.deleteAppliedManifestWork(ctx, manifestWork, appliedManifestWorkName)
    }
}
```

**Key insight:** work-agent **does receive** the DELETE event, but when it processes the event, the ManifestWork is already gone from the cache, so it returns without doing anything.

#### Backup Mechanism: 60-Minute Grace Period

OCM has a backup mechanism (`UnManagedAppliedWorkController`) that evicts orphaned AppliedManifestWork:

```go
// ocm/pkg/work/spoke/controllers/finalizercontroller/unmanaged_appliedmanifestwork_controller.go:112-116
_, err = m.manifestWorkLister.Get(appliedManifestWork.Spec.ManifestWorkName)
if errors.IsNotFound(err) {
    // evict the current appliedmanifestwork when its relating manifestwork is missing
    return m.evictAppliedManifestWork(ctx, controllerContext, appliedManifestWork)
}

// ocm/pkg/work/spoke/options.go:34
AppliedManifestWorkEvictionGracePeriod: 60 * time.Minute,  // Default: 60 minutes!
```

But in e2e tests:
1. We don't want to wait 60 minutes
2. work-agent is killed when klusterlet is deleted, so the backup mechanism can't run

### Success vs Failure Sequence

```
SUCCESS CASE (work-agent fast):
┌─────────────────────────────────────────────────────────────────┐
│ 1. ManagedCluster deleted                                       │
│ 2. ManifestWork marked for deletion (not cascade deleted yet)   │
│ 3. work-agent QUICKLY processes deletion                        │
│ 4. work-agent deletes AppliedManifestWork                       │
│ 5. Klusterlet CR cascade deleted via ownerRef                   │
│ 6. Klusterlet operator cleans up namespace                      │
│ 7. Test passes ✓                                                │
└─────────────────────────────────────────────────────────────────┘

FAILURE CASE (namespace cascade first):
┌─────────────────────────────────────────────────────────────────┐
│ 1. ManagedCluster deleted                                       │
│ 2. clusternamespacedeletion deletes namespace                   │
│ 3. Namespace cascade deletes ManifestWork IMMEDIATELY           │
│ 4. work-agent sync() sees NotFound → returns nil (no cleanup)   │
│ 5. AppliedManifestWork orphaned                                 │
│ 6. Klusterlet CR orphaned (DeleteOption=Orphan)                 │
│ 7. Namespace stays active                                       │
│ 8. Test fails ❌                                                │
└─────────────────────────────────────────────────────────────────┘
```

---

## Problem 1: CRD Timeout After Klusterlet Deletion

### The Wrong Fix

To fix Problem 0, adding klusterlet deletion directly:

```go
AfterEach(func() {
    hubClusterClient.ClusterV1().ManagedClusters().Delete(clusterName)
    hubOperatorClient.OperatorV1().Klusterlets().Delete("klusterlet")  // ❌ Wrong order!
    assertManagedClusterDeletedFromSpoke()  // Times out on CRD!
})
```

### Why It Fails

```
1. Delete Klusterlet CR
2. Klusterlet operator deletes klusterlet-agent (contains work-agent)
3. work-agent is DEAD
4. AppliedManifestWork still exists (no one to clean it up)
5. CRD has ownerRef to AppliedManifestWork
6. CRD cannot be deleted
7. Test times out on CRD deletion ❌
```

This is a **circular dependency**: work-agent must be alive to delete AppliedManifestWork, but deleting Klusterlet kills work-agent.

---

## Solution: Correct Cleanup Order

### Fixed Cleanup Sequence

```
1. Delete ManagedCluster (triggers resourcecleanup_controller)
2. Wait for hub cleanup (ManagedCluster gone from hub)
3. Clean up orphaned AppliedManifestWork (safety net)
4. Delete Klusterlet (now safe - work-agent's job is done or we cleaned AMW)
5. Wait for namespace deletion
6. Wait for CRD deletion
```

### Implementation: assertSelfManagedClusterDeleted

```go
// test/e2e/e2e_suite_test.go
func assertSelfManagedClusterDeleted(clusterName string) {
    // Step 1: Delete managed cluster
    hubClusterClient.ClusterV1().ManagedClusters().Delete(clusterName)

    // Step 2: Wait for hub cleanup
    assertManagedClusterDeletedFromHub(clusterName)

    // Step 3: Clean up orphaned AppliedManifestWork (safety net)
    // Uses label: import.open-cluster-management.io/klusterlet-works=true
    cleanupOrphanedAppliedManifestWork()

    // Step 4: Delete klusterlet
    hubOperatorClient.OperatorV1().Klusterlets().Delete("klusterlet")

    // Step 5-6: Wait for namespace and CRD deletion
    assertKlusterletNamespaceDeleted()
    assertKlusterletDeleted()
}
```

### Why It Works

| Step | Action | Result |
|------|--------|--------|
| 1 | Delete ManagedCluster | Triggers ManifestWork deletion |
| 2 | Wait for hub cleanup | ManagedCluster and namespace on hub deleted |
| 3 | Clean orphaned AMW | Safety net - removes AMW if work-agent didn't process it |
| 4 | Delete Klusterlet | Safe now - AMW already cleaned |
| 5-6 | Wait for NS/CRD | Klusterlet operator cleans up, CRD cascade deleted |

---

## Debug Commands

```bash
# Check ManifestWork with klusterlet label
kubectl get manifestwork -A -l import.open-cluster-management.io/klusterlet-works=true

# Check AppliedManifestWork
kubectl get appliedmanifestwork -l import.open-cluster-management.io/klusterlet-works=true -o yaml

# Check CRD ownerReference
kubectl get crd klusterlets.operator.open-cluster-management.io -o jsonpath='{.metadata.ownerReferences}'

# Check work-agent status
kubectl get pods -n open-cluster-management-agent -l app=klusterlet-agent

# Manual cleanup of orphaned AppliedManifestWork
kubectl delete appliedmanifestwork -l import.open-cluster-management.io/klusterlet-works=true
```

---

## Summary

### Root Cause Chain

```
ManifestWork DeleteOption = Orphan
    │
    ├─► Klusterlet CR orphaned when ManifestWork deleted
    │
    ├─► clusternamespacedeletion doesn't check ManifestWorks
    │       │
    │       └─► Race: namespace may cascade delete ManifestWork
    │               before work-agent processes it
    │
    └─► work-agent sees NotFound when processing DELETE event
            │
            └─► AppliedManifestWork orphaned
                    │
                    └─► CRD stuck (ownerRef to AMW)
```

### Problem Evolution

| # | Problem | Root Cause | Fix |
|---|---------|------------|-----|
| 0 | Namespace timeout | Orphan policy + race condition | Add explicit klusterlet deletion |
| 1 | CRD timeout | Deleting klusterlet kills work-agent | Correct order: delete MC first, clean AMW, then delete klusterlet |

### Key Insight

The **circular dependency** must be broken:
- work-agent must be alive to clean AppliedManifestWork
- Deleting Klusterlet kills work-agent
- Solution: Either let work-agent clean AMW first, OR manually clean orphaned AMW before deleting Klusterlet
