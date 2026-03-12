# E2E Test Authoring Guidelines

## Rule: Assert Leader Election After Any klusterlet-agent Rollout

**Any test step that triggers a klusterlet-agent deployment rollout MUST ensure
the agent completes leader election before proceeding to steps that depend on the
agent being functional (e.g., deleting a ManagedCluster).**

### Root cause: initial import always triggers a rolling update

The import controller creates the klusterlet deployment, then updates it ~4
seconds later when the CSR is approved and the bootstrap token is generated. This
second reconciliation changes the deployment spec, triggering a rolling update.
The old pod may have set `ManagedClusterConditionAvailable=True` before being
terminated, giving a false sense of readiness while the new pod is still
performing leader election.

### Why this matters

When klusterlet-agent rolls out, the new pod must complete leader election before
it can function. If subsequent test steps proceed before leader election
completes, the following race condition can occur:

1. A rollout starts a new klusterlet-agent pod.
2. The old pod is terminated; the new pod begins leader election.
3. The test proceeds (e.g., deletes `ManagedCluster`) while the new pod is still
   waiting for the leader lease.
4. `orphanCleanup()` force-deletes ManifestWorks **and** the `workRoleBinding`
   (hub RBAC for the work agent).
5. The new work agent acquires leadership but gets HTTP 403 when listing
   ManifestWorks because hub RBAC has been removed.
6. `AppliedManifestWork` finalizer blocks `Klusterlet` CR deletion, which blocks
   `open-cluster-management-agent` namespace deletion, causing test timeout.

### Systematic protection

The leader election check is embedded in three centralized helpers. Each one:

1. Lists klusterlet-agent pods in `open-cluster-management-agent` namespace.
2. **Deletes all agent pods** to force a restart — this accelerates the rollout
   by avoiding waiting for the old pod's graceful shutdown and any
   CrashLoopBackOff delays.
3. **Deletes the `klusterlet-agent-lock` lease** so the new pod can immediately
   acquire leadership without waiting for the old lease to expire.
4. Calls `assertAgentLeaderElection()` to wait for the new pod to become leader.

The check is skipped when no agent pods exist (e.g., tests without klusterlet
deployment, or custom namespaces via KlusterletConfig NoOperator mode).

**Protected helpers:**

| Helper | When the check runs |
|--------|-------------------|
| `assertManagedClusterDeleted()` | Before deleting the ManagedCluster in AfterEach cleanup |
| `assertManagedClusterAvailable()` | Before checking cluster availability |
| `assertManagedClusterOffline()` | Before checking cluster offline status |

### How `assertAgentLeaderElection()` works

1. Lists klusterlet-agent pods and filters out Terminating pods (non-zero
   `DeletionTimestamp`) to avoid being blocked by old pods in graceful shutdown.
2. Waits until there is exactly one non-terminating pod.
3. Checks that the `klusterlet-agent-lock` lease's `HolderIdentity` matches the
   current pod name.
4. Timeout: 180 seconds (worst-case non-graceful acquisition is ~163s).

### When explicit `assertAgentLeaderElection()` is still needed

The centralized checks cover most cases. Explicit calls are only needed when a
test deletes the ManagedCluster in its **test body** (not via AfterEach), since
`assertManagedClusterDeleted()` is not called in that path. Examples:

- `cleanup_test.go` — all three test cases call `assertAgentLeaderElection()`
  before deleting the ManagedCluster in the test body.
- `clusterdeployment_test.go` — the "destroy" and "detach" tests call it before
  deleting.

### Other fixes in this PR

- **klusterletconfig_test.go**: Add `restartAgentPods()` after reverting invalid
  server URL to escape CrashLoopBackOff (pod may take 10+ minutes to retry).
- **klusterletconfig_test.go**: Tolerate `NotFound` error in `restartAgentPods()`
  when pod is already deleted by Deployment controller during rolling update.

### Leader election timing reference

| Parameter      | Value |
|----------------|-------|
| Lease duration | 137s  |
| Renew deadline | 107s  |
| Retry period   | 26s   |

- **Graceful** (old pod releases lease): new pod acquires within ~26s.
- **Non-graceful** (old pod terminated without releasing): up to 163s (~2m43s).
