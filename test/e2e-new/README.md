# E2E New Test Framework

This directory contains the refactored e2e tests. The original tests in `test/e2e/` are kept for reference.

## CI Status

| Job | Makefile Target | Label Filter | Cluster Setup |
|-----|----------------|--------------|---------------|
| `e2e-new-core` | `e2e-new-test-core` | `core && !agent-registration` | single |
| `e2e-new-misc` | `e2e-new-test-misc` | `!core && !hosted && !agent-registration` | single |
| `e2e-new-hosted` | `e2e-new-test-hosted` | `hosted` | dual |

## Framework Overview

### Key Components (`framework/`)

| File | Description |
|------|-------------|
| `hub.go` | `Hub` struct consolidating all 12+ K8s clients (replaces global variables) |
| `agent.go` | Centralized leader election handling: `EnsureAgentReady()`, `RestartAgentPods()` |
| `cluster.go` | `ClusterLifecycle` with scope-based smart teardown |
| `assertions.go` | ~40 assertion methods on `Hub` |

### ClusterLifecycle Scopes

| Scope | Factory | Teardown Behavior | Use When |
|-------|---------|-------------------|----------|
| `ScopeCreated` | `ForCreatedOnly(hub, name)` | Delete cluster + wait for hub cleanup only. No leader election. Fast. | Tests that only create ManagedCluster (metadata, RBAC, secret tests) |
| `ScopeImported` | `ForImportedMode(hub, name)` | `EnsureAgentReady()` + delete + hub cleanup + spoke cleanup | Agent deployed but not waiting for Available |
| `ScopeAvailable` | `ForDefaultMode(hub, name)` | Same as ScopeImported | Full import, wait for Available |
| `ScopeAvailable` (hosted) | `ForHostedMode(hub, name, hosting)` | Hosted-specific cleanup | Hosted mode clusters |

## File Mapping: Old → New

### managedcluster_test.go (Label: `core`, Scope: `ScopeCreated`)

Only creates ManagedCluster — no agent deployment. Fast teardown.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should create the meta object and the import secret | `e2e/managedcluster_test.go:40` | `e2e-new/managedcluster_test.go:43` | `assertManagedClusterImportSecretCreated` → `hub.AssertImportSecretCreated` |
| 2 | Should recover the meta objet of the managed cluster | `e2e/managedcluster_test.go:49` | `e2e-new/managedcluster_test.go:52` | `assertManagedClusterNameLabel` → `hub.AssertClusterNameLabel` |
| 3 | Should recover the label of the managed cluster namespace | `e2e/managedcluster_test.go:75` | `e2e-new/managedcluster_test.go:78` | `assertManagedClusterNamespaceLabel` → `hub.AssertClusterNamespaceLabel` |
| 4 | Should recover the required rbac | `e2e/managedcluster_test.go:97` | `e2e-new/managedcluster_test.go:100` | `assertManagedClusterRBAC` → `hub.AssertClusterRBAC` |
| 5 | Should recover the import secret | `e2e/managedcluster_test.go:114` | `e2e-new/managedcluster_test.go:117` | `assertManagedClusterImportSecret` → `hub.AssertImportSecret` |
| 6 | Should recover the cluster import config secret | `e2e/managedcluster_test.go:126` | `e2e-new/managedcluster_test.go:129` | `assertClusterImportConfigSecret` → `hub.AssertClusterImportConfigSecret` |

**Teardown**: Old: `assertManagedClusterDeleted()` → New: `framework.ForCreatedOnly()` + `cl.Teardown()` (no leader election needed)

---

### manuallyimport_test.go (Label: `core`, Scope: `ScopeAvailable`)

Deploys agent, waits for Available.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should import the cluster manually | `e2e/manuallyimport_test.go:39` | `e2e-new/manuallyimport_test.go:42` | `assertManagedClusterAvailable` → `hub.AssertClusterAvailable` |

**Teardown**: Old: `assertManagedClusterDeleted()` (with inline leader election) → New: `framework.ForDefaultMode()` + `cl.Teardown()` (leader election via `EnsureAgentReady()`)

---

### autoimport_test.go (Label: `core`, Scope: `ScopeAvailable`)

Auto-import via secret creation.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should import the cluster with auto-import-secret with kubeconfig | `e2e/autoimport_test.go:48` | `e2e-new/autoimport_test.go:52` | All `assert*` → `hub.Assert*` |
| 2 | Should not import the cluster if auto-import is disabled | `e2e/autoimport_test.go:73` | `e2e-new/autoimport_test.go:77` | |
| 3 | Should not recover the agent once joined if auto-import strategy is ImportOnly | `e2e/autoimport_test.go:113` | `e2e-new/autoimport_test.go:117` | |
| 4 | Should trigger auto-import with immediate-import annotation | `e2e/autoimport_test.go:172` | `e2e-new/autoimport_test.go:176` | |
| 5 | Should import the cluster with auto-import-secret with token | `e2e/autoimport_test.go:221` | `e2e-new/autoimport_test.go:225` | |
| 6 | Should keep the auto-import-secret after the cluster was imported | `e2e/autoimport_test.go:246` | `e2e-new/autoimport_test.go:250` | |
| 7 | Should only update the bootstrap secret | `e2e/autoimport_test.go:278` | `e2e-new/autoimport_test.go:282` | |
| 8 | Should auto import the cluster with config | `e2e/autoimport_test.go:333` | `e2e-new/autoimport_test.go:337` | |

**Teardown**: Old: `assertManagedClusterDeleted()` → New: `framework.ForDefaultMode()` + `cl.Teardown()`

---

### selfmanagedcluster_test.go (Label: `core`, Scope: `ScopeAvailable`)

Local-cluster (self-managed) tests.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Importing a local-cluster / Should import the local-cluster | `e2e/selfmanagedcluster_test.go:41` | `e2e-new/selfmanagedcluster_test.go:45` | `assertManagedClusterAvailable` → `hub.AssertClusterAvailable` |
| 2 | custom auto-import strategy / Should not recover the agent (ImportOnly) | `e2e/selfmanagedcluster_test.go:73` | `e2e-new/selfmanagedcluster_test.go:80` | |
| 3 | custom auto-import strategy / Should trigger auto-import with immediate-import | `e2e/selfmanagedcluster_test.go:111` | `e2e-new/selfmanagedcluster_test.go:118` | |
| 4 | self managed cluster label / Should import the self managed cluster | `e2e/selfmanagedcluster_test.go:156` | `e2e-new/selfmanagedcluster_test.go:167` | |

**Teardown**: Old: `assertManagedClusterDeleted()` → New: `framework.ForDefaultMode()` + `cl.Teardown()`

---

### cleanup_test.go (Label: `cleanup`, Scope: `ScopeAvailable`)

Tests cluster cleanup/detach. Uses mid-test deletion.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should delete addons and manifestWorks | `e2e/cleanup_test.go:53` | `e2e-new/cleanup_test.go:57` | `assertAgentLeaderElection()` + manual delete → `cl.DeleteCluster()` (auto leader election) |
| 2 | Should delete addons and manifestWorks by force | `e2e/cleanup_test.go:157` | `e2e-new/cleanup_test.go:155` | Same pattern |
| 3 | should keep the ns when infraenv exists | `e2e/cleanup_test.go:286` | `e2e-new/cleanup_test.go:279` | Same pattern |

**Key improvement**: Old code manually called `assertAgentLeaderElection()` before each delete. New code uses `cl.DeleteCluster()` which calls `EnsureAgentReady()` automatically.

---

### clusterdeployment_test.go (Label: `core`, Scope: `ScopeAvailable`)

ClusterDeployment integration tests.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | (main Describe setup — creates cluster + clusterdeployment) | `e2e/clusterdeployment_test.go:22` | `e2e-new/clusterdeployment_test.go:23` | `assertAgentLeaderElection` → `hub.EnsureAgentReady` |
| 2 | custom auto-import-strategy / Should not recover the agent (ImportOnly) | `e2e/clusterdeployment_test.go:92` | `e2e-new/clusterdeployment_test.go:95` | |
| 3 | immediate-import / Should trigger auto-import | `e2e/clusterdeployment_test.go:138` | `e2e-new/clusterdeployment_test.go:141` | |
| 4 | Should destroy the managed cluster | `e2e/clusterdeployment_test.go:168` | `e2e-new/clusterdeployment_test.go:171` | |
| 5 | Should detach the managed cluster | `e2e/clusterdeployment_test.go:190` | `e2e-new/clusterdeployment_test.go:193` | |

**Teardown**: Old: `assertManagedClusterDeleted()` → New: `framework.ForDefaultMode()` + `cl.Teardown()`

---

### hostedcluster_test.go (Label: `hosted`, Scope: `ForHostedMode`)

Hosted mode cluster tests with hosting cluster.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | auto-import-secret / Should import with kubeconfig | `e2e/hostedcluster_test.go:65` | `e2e-new/hostedcluster_test.go:73` | `assertHostedManagedClusterDeleted` → `cl.Teardown()` via `ForHostedMode` |
| 2 | manually / Should import by creating external managed kubeconfig | `e2e/hostedcluster_test.go:106` | `e2e-new/hostedcluster_test.go:118` | |
| 3 | manually / Should override the klusterlet namespace by annotation | `e2e/hostedcluster_test.go:141` | `e2e-new/hostedcluster_test.go:153` | |
| 4 | Detach multiple hosted clusters / should delete independently | `e2e/hostedcluster_test.go:184` | `e2e-new/hostedcluster_test.go:196` | |
| 5 | Cleanup / Should clean up the addons | `e2e/hostedcluster_test.go:263` | `e2e-new/hostedcluster_test.go:282` | |
| 6 | Cleanup / Should clean up the addons with finalizer | `e2e/hostedcluster_test.go:306` | `e2e-new/hostedcluster_test.go:325` | |

**Teardown**: Old: `assertHostedManagedClusterDeleted(name, hosting)` → New: `framework.ForHostedMode(hub, name, hosting)` + `cl.Teardown()`

---

### klusterletconfig_test.go (Label: `config`, Scope: `ScopeAvailable`)

KlusterletConfig customization tests.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should deploy the klusterlet with nodePlacement | `e2e/klusterletconfig_test.go:56` | `e2e-new/klusterletconfig_test.go:61` | `restartAgentPods` → `hub.RestartAgentPods` |
| 2 | Should deploy the klusterlet with proxy config | `e2e/klusterletconfig_test.go:142` | `e2e-new/klusterletconfig_test.go:147` | `assertBootstrapKubeconfig` → `hub.AssertBootstrapKubeconfig` |
| 3 | Should ignore the proxy config for self managed cluster | `e2e/klusterletconfig_test.go:237` | `e2e-new/klusterletconfig_test.go:242` | |
| 4 | Should deploy with custom server URL and CA bundle | `e2e/klusterletconfig_test.go:271` | `e2e-new/klusterletconfig_test.go:276` | `assertBootstrapKubeconfigConsistently` → `hub.AssertBootstrapKubeconfigConsistently` |
| 5 | Should deploy with custom server URL for self managed cluster | `e2e/klusterletconfig_test.go:347` | `e2e-new/klusterletconfig_test.go:352` | |
| 6 | Should deploy with customized namespace | `e2e/klusterletconfig_test.go:409` | `e2e-new/klusterletconfig_test.go:414` | `AssertKlusterletNamespace` → `hub.AssertKlusterletNamespace` |
| 7 | Should deploy with custom eviction grace period | `e2e/klusterletconfig_test.go:458` | `e2e-new/klusterletconfig_test.go:463` | `assertAppliedManifestWorkEvictionGracePeriod` → `hub.AssertAppliedManifestWorkEvictionGracePeriod` |
| 8 | Should deploy with featuregate | `e2e/klusterletconfig_test.go:503` | `e2e-new/klusterletconfig_test.go:508` | `assertFeatureGate` → `hub.AssertFeatureGate` |

**Teardown**: Old: `assertManagedClusterDeleted()` → New: `framework.ForDefaultMode()` + `cl.Teardown()`

---

### klusterletplacement_test.go (Label: `config`, Scope: `ScopeAvailable`)

Node placement tests.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should deploy the klusterlet without node placement | `e2e/klusterletplacement_test.go:37` | `e2e-new/klusterletplacement_test.go:40` | `assertKlusterletNodePlacement` → `hub.AssertKlusterletNodePlacement` |
| 2 | Should update the klusterlet node placement | `e2e/klusterletplacement_test.go:59` | `e2e-new/klusterletplacement_test.go:62` | |

**Teardown**: Old: `assertManagedClusterDeleted()` → New: `framework.ForDefaultMode()` + `cl.Teardown()`

---

### csr_test.go (Label: `config`, Scope: None)

No ManagedCluster created. No ClusterLifecycle needed.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should not approve the CSR with wrong labels | `e2e/csr_test.go:19` | `e2e-new/csr_test.go:19` | `hubKubeClient` → `hub.KubeClient` |

---

### imageregistry_test.go (Label: `config`, Scope: `ScopeCreated`)

Image registry pull secret test. No agent deployed.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should using customized image registry | `e2e/imageregistry_test.go:66` | `e2e-new/imageregistry_test.go:71` | `assertPullSecretDeleted` → `hub.AssertPullSecretDeleted` |

**Teardown**: Old: `assertManagedClusterDeleted()` → New: `framework.ForCreatedOnly()` + `cl.Teardown()` (fast, no leader election)

---

### agentregistration_test.go (Label: `agent-registration`, Scope: None)

Checks externally created cluster. No ClusterLifecycle needed.

| # | Test Case | Old File:Line | New File:Line | Changes |
|---|-----------|---------------|---------------|---------|
| 1 | Should have the managed cluster registrated | `e2e/agentregistration_test.go:16` | `e2e-new/agentregistration_test.go:16` | `hubClusterClient` → `hub.ClusterClient` |

---

## Assertion Mapping Reference

| Old Function (standalone) | New Method (`hub.*`) |
|--------------------------|---------------------|
| `assertManagedClusterFinalizer` | `hub.AssertClusterFinalizer` |
| `assertManagedClusterCreatedViaAnnotation` | `hub.AssertClusterCreatedVia` |
| `assertManagedClusterNameLabel` | `hub.AssertClusterNameLabel` |
| `assertManagedClusterNamespaceLabel` | `hub.AssertClusterNamespaceLabel` |
| `assertManagedClusterRBAC` | `hub.AssertClusterRBAC` |
| `assertManagedClusterNamespace` | `hub.AssertClusterNamespace` |
| `assertManagedClusterImportSecretCreated` | `hub.AssertImportSecretCreated` |
| `assertManagedClusterImportSecret` | `hub.AssertImportSecret` |
| `assertHostedManagedClusterImportSecret` | `hub.AssertHostedImportSecret` |
| `assertClusterImportConfigSecret` | `hub.AssertClusterImportConfigSecret` |
| `assertManagedClusterImportSecretApplied` | `hub.AssertImportSecretApplied` |
| `assertManagedClusterImportSecretNotApplied` | `hub.AssertImportSecretNotApplied` |
| `assertAutoImportSecretDeleted` | `hub.AssertAutoImportSecretDeleted` |
| `assertManagedClusterAvailable` | `hub.AssertClusterAvailable` |
| `assertManagedClusterAvailableUnknown` | `hub.AssertClusterAvailableUnknown` |
| `assertManagedClusterAvailableUnknownConsistently` | `hub.AssertClusterAvailableUnknownConsistently` |
| `assertManagedClusterOffline` | `hub.AssertClusterOffline` |
| `assertImmediateImportCompleted` | `hub.AssertImmediateImportCompleted` |
| `assertManagedClusterDeleted` | `cl.Teardown()` or `hub.AssertClusterDeleted` |
| `assertHostedManagedClusterDeleted` | `cl.Teardown()` (via `ForHostedMode`) |
| `assertManagedClusterDeletedFromHub` | `hub.AssertClusterDeletedFromHub` or `cl.WaitForClusterGone()` |
| `assertManagedClusterDeletedFromSpoke` | `hub.AssertClusterDeletedFromSpoke` |
| `assertHostedManagedClusterDeletedFromSpoke` | `hub.AssertHostedClusterDeletedFromSpoke` |
| `assertPullSecretDeleted` | `hub.AssertPullSecretDeleted` |
| `assertManagedClusterManifestWorks` | `hub.AssertManifestWorks` |
| `assertManagedClusterManifestWorksAvailable` | `hub.AssertManifestWorksAvailable` |
| `assertHostedKlusterletManifestWorks` | `hub.AssertHostedManifestWorks` |
| `assertHostedManagedClusterManifestWorksAvailable` | `hub.AssertHostedManifestWorksAvailable` |
| `assertManifestworkFinalizer` | `hub.AssertManifestWorkFinalizer` |
| `assertManagedClusterPriorityClass` | `hub.AssertPriorityClass` |
| `assertManagedClusterPriorityClassHosted` | `hub.AssertPriorityClassHosted` |
| `assertKlusterletNodePlacement` | `hub.AssertKlusterletNodePlacement` |
| `AssertKlusterletNamespace` | `hub.AssertKlusterletNamespace` |
| `assertAppliedManifestWorkEvictionGracePeriod` | `hub.AssertAppliedManifestWorkEvictionGracePeriod` |
| `assertFeatureGate` / `assertManagedClusterFeatureGate` | `hub.AssertFeatureGate` |
| `assertBootstrapKubeconfig` | `hub.AssertBootstrapKubeconfig` |
| `assertBootstrapKubeconfigConsistently` | `hub.AssertBootstrapKubeconfigConsistently` |
| `assertBootstrapKubeconfigWithProxyConfig` | `hub.AssertBootstrapKubeconfigWithProxy` |
| `assertNamespaceCreated` | `hub.AssertNamespaceCreated` |
| `assertAgentLeaderElection` | `hub.EnsureAgentReady()` (called automatically by `ClusterLifecycle`) |
| `restartAgentPods` | `hub.RestartAgentPods` |

## Client Mapping Reference

| Old Global Variable | New Hub Field |
|---------------------|---------------|
| `hubKubeClient` | `hub.KubeClient` |
| `hubDynamicClient` | `hub.DynamicClient` |
| `crdClient` | `hub.CRDClient` |
| `hubClusterClient` | `hub.ClusterClient` |
| `hubWorkClient` | `hub.WorkClient` |
| `hubOperatorClient` | `hub.OperatorClient` |
| `addonClient` | `hub.AddonClient` |
| `klusterletconfigClient` | `hub.KlusterletCfgClient` |
| `hubRuntimeClient` | `hub.RuntimeClient` |
| `recorder` | `hub.Recorder` |
| `restMapper` | `hub.Mapper` |
| `restConfig` | `hub.RestConfig` |
