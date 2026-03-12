# TLS Profile Compliance Design for Server Foundation Repos

**JIRA:** [ACM-26882: [ACM] Central TLS Profile consistency](https://issues.redhat.com/browse/ACM-26882)
**Document Status:** Design Document
**Last Updated:** 2026-03-12

---

## Table of Contents

1. [Overview](#overview)
2. [Scenario Summary](#scenario-summary)
3. [Solution Overview](#solution-overview)
4. [Stolostron Scenarios](#stolostron-scenarios)
5. [OCM Hub Scenarios](#ocm-hub-scenarios)
6. [OCM Spoke Scenarios](#ocm-spoke-scenarios)
7. [Implementation Details](#implementation-details)
8. [Compliance Verification](#compliance-verification)

---

## Overview

### Purpose

This design provides a unified approach for server foundation repositories to implement TLS profile compliance for OpenShift 4.22 GA. Components must **dynamically fetch and apply TLS configuration** rather than hardcoding TLS settings, critical for **Post-Quantum Cryptography (PQC) readiness**.

### Challenge for OCM

OCM repos are **upstream Kubernetes projects** that cannot depend on OpenShift-specific APIs like `APIServer.spec.tlsSecurityProfile`.

### Deployment Relationships

| Deployer | Deploys | Scenario |
|---|---|---|
| **Hub:** backplane-operator | cluster-manager-operator | 3 |
| **Hub:** backplane-operator | cluster-proxy-addon-manager | 5 |
| **Hub:** cluster-manager-operator | registration/work/placement-controller | 4 |
| **Spoke:** import-controller | klusterlet-operator | 6 |
| **Spoke:** klusterlet-operator | klusterlet/registration/work-agent | 7 |
| **Spoke:** addon managers | addon agents | 8 |

---

## Scenario Summary

| Scenario | Component | Platform | Sidecar | ConfigMap Pattern | Solution |
|---|---|---|---|---|---|
| **1** | Stolostron Hub | OpenShift | ✅ | Direct consumption | Refer to OpenShift hint doc |
| **2** | Stolostron Spoke | OpenShift | ✅ | Direct consumption | Refer to OpenShift hint doc |
| **3** | OCM Hub - cluster-manager-operator | OpenShift/K8s | ✅/❌ | Watches + restarts | Sidecar + ConfigMap |
| **4** | OCM Hub - ocm-hub-components | OpenShift/K8s | ❌ | Operator creates ConfigMap | Operator propagation |
| **5** | OCM Hub - addon-manager | OpenShift/K8s | ✅/❌ | Watches + restarts | Sidecar + ConfigMap |
| **6** | OCM Spoke - klusterlet-operator | OpenShift/K8s | ✅/❌ | Watches + restarts | Sidecar + ConfigMap |
| **7** | OCM Spoke - klusterlet-agent | OpenShift/K8s | ❌ | Operator creates ConfigMap | Operator propagation |
| **8** | OCM Spoke - addon-agent | OpenShift/K8s | ❌ | Operator copies ConfigMap | ConfigMap copy pattern |
| **9** | cluster-proxy components (self-deployed by cluster proxy manager/agent) | OpenShift/K8s | TBD | TBD | TBD |

---

## Solution Overview

### Sidecar + ConfigMap Pattern

**How this resolves the upstream challenge:** The sidecar (downstream code) handles OpenShift-specific API access (`APIServer.spec.tlsSecurityProfile`), translating it into a standard Kubernetes ConfigMap. Upstream OCM components only use standard Kubernetes client-go to watch and read ConfigMaps, maintaining full portability across any Kubernetes distribution while enabling OpenShift integration when deployed by Stolostron.

```
┌──────────────────────────────────────────────────────────────────┐
│ OpenShift Platform (Stolostron deployment):                      │
│                                                                  │
│ Sidecar Container → Watches APIServer TLS profile                │
│                  → Creates/Updates ConfigMap "ocm-tls-profile"   │
│                                                                  │
│ Component Container → Watches ConfigMap "ocm-tls-profile"        │
│                    → Applies new TLS config                      │
│                    → Restarts itself on ConfigMap change         │
│                                                                  │
│ Operators → Read ConfigMap from their namespace                  │
│          → Create/Update ConfigMap in managed component ns       │
│                                                                  │
│ Result: Dynamic TLS profile (Modern/Intermediate/Custom)         │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│ Kubernetes Platform (Upstream deployment):                       │
│                                                                  │
│ No sidecar → No ConfigMap                                        │
│ Components → Use TLS 1.2 fallback (hardcoded safe default)       │
│                                                                  │
│ Result: Static TLS 1.2 configuration                             │
└──────────────────────────────────────────────────────────────────┘
```

### Key Principles

1. **Upstream Portability**: OCM repos remain OpenShift-agnostic
2. **Sidecar Injection**: Downstream (Stolostron) repos inject sidecar when deploying on OpenShift
3. **ConfigMap Propagation**: Operators create ConfigMaps in managed component namespaces
4. **Restart on Change**: Components watch ConfigMap and restart when TLS config changes
5. **Safe Fallback**: Components use TLS 1.2 when ConfigMap not available (vanilla Kubernetes)
6. **Addon Flexibility**: Scenarios 5 & 8 are **for reference only**. Addon squads may implement their own solution.

---

## Stolostron Scenarios

### Scenario 1 & 2: Stolostron Hub & Spoke (OpenShift)

**Solution:** Refer to [Hint for resolving TLS non-compliance tickets Code Examples](https://docs.google.com/document/d/1234567890)

**Approach:** Use OpenShift library-go TLS helpers to watch `APIServer.spec.tlsSecurityProfile` directly (no sidecar needed)

---

## OCM Hub Scenarios

### Scenario 3: OCM Hub - cluster-manager-operator

**Namespace:** `multicluster-engine` | **Deployed by:** backplane-operator

```
┌─────────────────────────────────────────────────────────────────┐
│ Hub OpenShift Cluster                                           │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ APIServer.spec.tlsSecurityProfile: Modern                   │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Watches                                  │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Pod: cluster-manager-operator (deployed by backplane-op)    │ │
│ │ ┌───────────────────┐  ┌──────────────────────────────────┐ │ │
│ │ │ Container:        │  │ Sidecar:                         │ │ │
│ │ │ registration-     │  │ tls-profile-sync                 │ │ │
│ │ │ operator          │  │                                  │ │ │
│ │ │                   │  │ • Watches APIServer              │ │ │
│ │ │ • Watches         │  │ • Creates/Updates ConfigMap      │ │ │
│ │ │   ConfigMap       │  │                                  │ │ │
│ │ │ • Restarts on     │  │                                  │ │ │
│ │ │   change          │  │                                  │ │ │
│ │ └─────────┬─────────┘  └───────────┬ ─────────────────────┘ │ │
│ └───────────┼────────────────────────┼─────────────── ────────┘ │
│             │                        │                          │
│             │                        ▼                          │
│             │   ┌─────────────────────────────────────────┐     │
│             │   │ ConfigMap: ocm-tls-profile              │     │
│             └──▶│ Namespace: multicluster-engine          │     │
│                 │                                         │     │
│                 │ data:                                   │     │
│                 │   minTLSVersion: "VersionTLS13"         │     │
│                 │   cipherSuites: ""                      │     │
│                 │   profileType: "Modern"                 │     │
│                 └─────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:**
- Sidecar: backplane-operator injects `tls-profile-sync` → watches `APIServer.spec.tlsSecurityProfile` → creates `multicluster-engine/ocm-tls-profile`
- Component: Watches ConfigMap → restarts on change → TLS 1.2 fallback if not found

---

### Scenario 4: OCM Hub - ocm-hub-components

**Components:** registration-controller, work-controller, placement-controller
**Namespace:** `open-cluster-management-hub` | **Deployed by:** cluster-manager-operator

```
┌─────────────────────────────────────────────────────────────────┐
│ Hub Cluster                                                     │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ cluster-manager-operator (Scenario 3)                       │ │
│ │ Namespace: multicluster-engine                              │ │
│ │                                                             │ │
│ │ • Reads: multicluster-engine/ocm-tls-profile                │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Creates ConfigMap in hub component ns    │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ ConfigMap: ocm-tls-profile                                  │ │
│ │ Namespace: open-cluster-management-hub                      │ │
│ │                                                             │ │
│ │ data:                                                       │ │
│ │   minTLSVersion: "VersionTLS13"                             │ │
│ │   cipherSuites: ""                                          │ │
│ │   profileType: "Modern"                                     │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Shared by all hub components             │
│                      │ (all in same namespace)                  │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Components in open-cluster-management-hub namespace:        │ │
│ │                                                             │ │
│ │ • registration-controller → watches ConfigMap, restarts     │ │
│ │ • work-controller → watches ConfigMap, restarts             │ │
│ │ • placement-controller → watches ConfigMap, restarts        │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:**
- Operator: cluster-manager-operator watches `multicluster-engine/ocm-tls-profile` → creates `open-cluster-management-hub/ocm-tls-profile`
- Components: Watch ConfigMap in their namespace → restart on change → TLS 1.2 fallback

---

### Scenario 5: OCM Hub - addon-manager

> **Note:** For reference only. Addon squads may implement their own solution.

**Components:** cluster-proxy-addon-manager, submariner-addon-manager, etc.

**5a: Same Namespace** (cluster-proxy-addon-manager in `multicluster-engine`)

```
┌─────────────────────────────────────────────────────────────────┐
│ Hub OpenShift Cluster                                           │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ ConfigMap: ocm-tls-profile (from Scenario 3)                │ │
│ │ Namespace: multicluster-engine                              │ │
│ │                                                             │ │
│ │ Created by cluster-manager-operator sidecar                 │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Shared in same namespace                 │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Pod: cluster-proxy-addon-manager                            │ │
│ │ Namespace: multicluster-engine                              │ │
│ │                                                             │ │
│ │ ┌───────────────────────────────────────────────────────┐   │ │
│ │ │ Container: cluster-proxy-addon-manager                │   │ │
│ │ │                                                       │   │ │
│ │ │ • No sidecar needed (same namespace)                  │   │ │
│ │ │ • Watches multicluster-engine/ocm-tls-profile         │   │ │
│ │ │ • Restarts on ConfigMap change                        │   │ │
│ │ └───────────────────────────────────────────────────────┘   │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:** No sidecar needed; watches shared ConfigMap in `multicluster-engine` → restarts on change

**5b: Different Namespace** (e.g., submariner-addon-manager in `submariner-operator`)

```
┌─────────────────────────────────────────────────────────────────┐
│ Hub OpenShift Cluster                                           │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ APIServer.spec.tlsSecurityProfile                           │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Watches                                  │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Pod: submariner-addon-manager (by backplane-op)             │ │
│ │ Namespace: submariner-operator                              │ │
│ │ ┌───────────────────┐  ┌──────────────────────────────────┐ │ │
│ │ │ Container:        │  │ Sidecar:                         │ │ │
│ │ │ addon-manager     │  │ tls-profile-sync                 │ │ │
│ │ │                   │  │                                  │ │ │
│ │ │ • Watches         │  │ • Watches APIServer              │ │ │
│ │ │   ConfigMap       │  │ • Creates/Updates ConfigMap      │ │ │
│ │ │ • Restarts on     │  │                                  │ │ │
│ │ │   change          │  │                                  │ │ │
│ │ └─────────┬─────────┘  └────────────┬─────────────────────┘ │ │
│ └───────────┼─────────────────────────┼───────────────────────┘ │
│             │                         │                         │
│             │                         ▼                         │
│             │   ┌─────────────────────────────────────────┐     │
│             │   │ ConfigMap: ocm-tls-profile              │     │
│             └──▶│ Namespace: submariner-operator          │     │
│                 │                                         │     │
│                 │ data:                                   │     │
│                 │   minTLSVersion: "VersionTLS13"         │     │
│                 │   cipherSuites: ""                      │     │
│                 │   profileType: "Modern"                 │     │
│                 └─────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:**
- Sidecar: backplane-operator injects `tls-profile-sync` → creates ConfigMap in addon namespace
- Component: Watches ConfigMap → restarts on change

---

## OCM Spoke Scenarios

### Scenario 6: OCM Spoke - klusterlet-operator

**Namespace:** `open-cluster-management-agent` | **Deployed by:** import-controller

```
┌─────────────────────────────────────────────────────────────────┐
│ Managed OpenShift Cluster                                       │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ APIServer.spec.tlsSecurityProfile (LOCAL cluster!)          │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Watches                                  │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Pod: klusterlet-operator (deployed by import-controller)    │ │
│ │ ┌───────────────────┐  ┌──────────────────────────────────┐ │ │
│ │ │ Container:        │  │ Sidecar:                         │ │ │
│ │ │ registration-     │  │ tls-profile-sync                 │ │ │
│ │ │ operator          │  │                                  │ │ │
│ │ │                   │  │ • Watches LOCAL APIServer        │ │ │
│ │ │ • Watches         │  │ • Creates/Updates ConfigMap      │ │ │
│ │ │   ConfigMap       │  │                                  │ │ │
│ │ │ • Restarts on     │  │                                  │ │ │
│ │ │   change          │  │                                  │ │ │
│ │ └─────────┬─────────┘  └───────────┬──────────────────────┘ │ │
│ └───────────┼────────────────────────┼────────────────────────┘ │
│             │                        │                          │
│             │                        ▼                          │
│             │   ┌─────────────────────────────────────────┐     │
│             │   │ ConfigMap: ocm-tls-profile (SOURCE)     │     │
│             └──▶│ Namespace: open-cluster-management-agent│     │
│                 │                                         │     │
│                 │ data:                                   │     │
│                 │   minTLSVersion: "VersionTLS12"         │     │
│                 │   cipherSuites: "..."                   │     │
│                 │   profileType: "Intermediate"           │     │
│                 └─────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:**
- Sidecar: import-controller injects `tls-profile-sync` → watches **managed cluster's** APIServer → creates ConfigMap
- Component: Watches ConfigMap → restarts on change

**Important:** Each managed cluster uses its **OWN** TLS profile, not the hub's!

---

### Scenario 7: OCM Spoke - klusterlet-agent

**Components:** klusterlet-agent (singleton), registration-agent, work-agent (default mode)
**Deployed by:** klusterlet-operator

**7a: Default Mode** (Same namespace: `open-cluster-management-agent`)

```
┌─────────────────────────────────────────────────────────────────┐
│ Managed Cluster (Default Mode)                                  │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ klusterlet-operator (Scenario 6)                            │ │
│ │ • Reads: open-cluster-management-agent/ocm-tls-profile      │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Already in same namespace                │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ ConfigMap: ocm-tls-profile                                  │ │
│ │ Namespace: open-cluster-management-agent (same as operator) │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Shared by klusterlet components          │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Components in open-cluster-management-agent namespace:      │ │
│ │                                                             │ │
│ │ Singleton mode:                                             │ │
│ │ • klusterlet-agent → watches ConfigMap, restarts            │ │
│ │                                                             │ │
│ │ Default mode:                                               │ │
│ │ • registration-agent → watches ConfigMap, restarts          │ │
│ │ • work-agent → watches ConfigMap, restarts                  │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:** No ConfigMap copy needed; all components in same namespace share ConfigMap → watch → restart on change

**7b: Hosted Mode** (Different namespace: `klusterlet-<cluster-name>`)

```
┌─────────────────────────────────────────────────────────────────┐
│ Hosting Cluster (could be Hub or dedicated hosting cluster)     │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ ConfigMap: ocm-tls-profile (SOURCE)                         │ │
│ │ Namespace: open-cluster-management-agent                    │ │
│ │                                                             │ │
│ │ Created by klusterlet-operator sidecar                      │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │                                          │
│                      │ klusterlet-operator runs controller      │
│                      │ to copy ConfigMap to hosted namespace    │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ ConfigMap: ocm-tls-profile                                  │ │
│ │ Namespace: klusterlet-<managed-cluster-name>                │ │
│ │                                                             │ │
│ │ data:                                                       │ │
│ │   minTLSVersion: "VersionTLS12"                             │ │
│ │   cipherSuites: "..."                                       │ │
│ │   profileType: "Intermediate"                               │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Read by hosted klusterlet agents         │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Components in klusterlet-<managed-cluster-name> namespace:  │ │
│ │                                                             │ │
│ │ Singleton mode:                                             │ │
│ │ • klusterlet-agent → watches ConfigMap, restarts            │ │
│ │                                                             │ │
│ │ Default mode:                                               │ │
│ │ • registration-agent → watches ConfigMap, restarts          │ │
│ │ • work-agent → watches ConfigMap, restarts                  │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:**
- Operator: klusterlet-operator copies ConfigMap from `open-cluster-management-agent` to hosted namespace
- Components: Watch ConfigMap in their namespace → restart on change

**Note:** TLS profile comes from **hosting cluster's** APIServer, not managed cluster's APIServer.

---

### Scenario 8: OCM Spoke - addon-agent

> **Note:** For reference only. Addon squads may implement their own solution.

**Components:** app-addon-agent, policy-addon-agent, observability-addon-agent, cluster-proxy-addon-agent, etc.

```
┌─────────────────────────────────────────────────────────────────┐
│ Managed Cluster                                                 │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ ConfigMap: ocm-tls-profile (SOURCE)                         │ │
│ │ Namespace: open-cluster-management-agent                    │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │                                          │
│                      │ klusterlet-operator runs                 │
│                      │ AddonTLSConfigController                 │
│                      │ (similar to AddonPullImageSecretCtrl)    │
│                      │                                          │
│                      ▼                                          │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ Controller watches addon namespaces and copies ConfigMap: │   │
│ │                                                           │   │
│ │ • Label: addon.open-cluster-management.io/namespace=true  │   │
│ │ • On new addon namespace → copy ConfigMap                 │   │
│ │ • On source ConfigMap update → update all copies          │   │
│ └─────────────┬────────────┬────────────┬───────────────────┘   │
│               │            │            │                       │
│               ▼            ▼            ▼                       │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ ConfigMap Copies in Addon Namespaces:                       │ │
│ │                                                             │ │
│ │ • addon-app/ocm-tls-profile                                 │ │
│ │ • addon-policy/ocm-tls-profile                              │ │
│ │ • addon-observability/ocm-tls-profile                       │ │
│ │ • addon-cluster-proxy/ocm-tls-profile                       │ │
│ └────────────┬────────────┬────────────┬──────────────────────┘ │
│              │            │            │                        │
│              ▼            ▼            ▼                        │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Addon Agents watch ConfigMap in their namespace:            │ │
│ │                                                             │ │
│ │ • app-addon-agent → restarts on change                      │ │
│ │ • policy-addon-agent → restarts on change                   │ │
│ │ • observability-addon-agent → restarts on change            │ │
│ │ • cluster-proxy-addon-agent → restarts on change            │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:**
- Controller: `AddonTLSConfigController` in klusterlet-operator copies ConfigMap to addon namespaces (similar to `AddonPullImageSecretController`)
- Agents: Watch ConfigMap in their namespace → restart on change

**Benefits:** Namespace isolation, simpler RBAC, consistent with existing pattern

---

## Implementation Details

### ConfigMap Format

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ocm-tls-profile
  namespace: <varies by scenario>
data:
  minTLSVersion: "VersionTLS13"
  cipherSuites: "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,..."
  profileType: "Modern"  # Modern, Intermediate, Old, or Custom
```

### Component Restart Logic

```go
func main() {
    tlsConfig := readTLSConfigFromConfigMap()
    go watchConfigMapAndRestart()
    startComponent(tlsConfig)
}

func watchConfigMapAndRestart() {
    watcher := watchConfigMap("ocm-tls-profile")
    for event := range watcher.ResultChan() {
        if event.Type == watch.Modified {
            os.Exit(0)  // Kubernetes restarts the pod
        }
    }
}

func readTLSConfigFromConfigMap() *tls.Config {
    cm, err := client.CoreV1().ConfigMaps(namespace).Get("ocm-tls-profile")
    if err != nil {
        return &tls.Config{MinVersion: tls.VersionTLS12}  // Fallback
    }
    return parseTLSConfig(cm)
}
```

### New Components

| Component | Repository | Owner | Purpose |
|---|---|---|---|
| tls-profile-sync sidecar | stolostron/import-controller->tls-profile-sync | Downstream | Watches OpenShift APIServer, creates ConfigMap |
| Shared TLS library/helpers | open-cluster-management-io/sdk-go | Upstream | ConfigMap parsing, fallback logic, TLS config helpers |
| AddonTLSConfigController | open-cluster-management-io/registration-operator | Upstream | Copies ConfigMap to addon namespaces (in klusterlet-operator) |
| Addon ConfigMap watch + restart | open-cluster-management-io/addon-framework | Upstream | Common addon functionality to watch ConfigMap and restart |

### Modified Components

| Component | Repository | Modification | Owner |
|---|---|---|---|
| backplane-operator | stolostron/backplane-operator | Inject sidecar for cluster-manager-operator, addon-managers | Downstream |
| import-controller | stolostron/import-controller | Inject sidecar for klusterlet-operator | Downstream |
| cluster-manager-operator | open-cluster-management-io/registration-operator | Watch ConfigMap, create ConfigMaps in managed ns, restart on change | Upstream |
| klusterlet-operator | open-cluster-management-io/registration-operator | Watch ConfigMap, run AddonTLSConfigController, restart on change | Upstream |
| All hub/spoke components | Multiple ocm repos | Use sdk-go TLS library, watch ConfigMap, restart on change | Upstream |
| addon-framework | open-cluster-management-io/addon-framework | Provide ConfigMap watch + restart for all addons | Upstream |

### Sidecar Injection

**backplane-operator** injects sidecar into:
- cluster-manager-operator pod
- Addon-manager pods in different namespaces (e.g., submariner-addon-manager)

**import-controller** injects sidecar into:
- klusterlet-operator pod

**Detection logic:**
```go
func shouldInjectSidecar() bool {
    _, err := client.Discovery().ServerResourcesForGroupVersion("config.openshift.io/v1")
    return err == nil
}
```

---

## Compliance Verification

**Tools:** `tls-scanner`, `semgrep`, E2E tests

**Test Scenarios:**
1. Change APIServer TLS profile → verify components restart with new config
2. Deploy on vanilla Kubernetes → verify TLS 1.2 fallback
3. Add new addon → verify ConfigMap copied to addon namespace
4. Sidecar crash → verify components continue with last known config

**Current Findings:**

[pkg/common/options/webhook.go:97](pkg/common/options/webhook.go#L97) - Hardcoded `tls.VersionTLS12` needs remediation

---

## FAQ

**Q: Do upstream OCM repos need OpenShift dependencies?**
A: No. Upstream uses standard Kubernetes client-go to read ConfigMaps.

**Q: What if sidecar crashes?**
A: Kubernetes restarts sidecar. Components continue with last known ConfigMap.

**Q: Why restart instead of hot-reload?**
A: Simpler implementation. TLS changes are infrequent. Kubernetes handles graceful restarts.

**Q: Can users customize TLS profile?**
A: Yes, via `APIServer.spec.tlsSecurityProfile`. Changes propagate automatically.

**Q: What about client TLS?**
A: Separate initiative. This design focuses on server TLS (webhooks, metrics servers).

**Q: Why does each managed cluster use its own TLS profile?**
A: Managed cluster admins control their own security policy independently.

**Q: What if ConfigMap is deleted?**
A: Yes, automatically recreated via reconciliation:
- **Sidecars**: Recreate within seconds from APIServer TLS profile
- **Operators**: Recreate from source ConfigMaps
- **Components**: Fall back to TLS 1.2 temporarily, then restart with recreated ConfigMap

**Q: How does this support PQC?**
A: When OpenShift adds PQC cipher suites to APIServer TLS profiles, all components automatically adopt them via dynamic ConfigMap updates.

---

## Approval and Sign-off

**Document Owner:** ACM Server Foundation Team
**JIRA:** [ACM-26882](https://issues.redhat.com/browse/ACM-26882)
**Status:** Awaiting Review

### Required Approvals

- [ ] Server Foundation Team
- [ ] Installer Team

---

## References

- **OpenShift Requirement:** [Hint for resolving TLS non-compliance tickets Code Examples](https://docs.google.com/document/d/1234567890)
- **JIRA:** [ACM-26882: [ACM] Central TLS Profile consistency](https://issues.redhat.com/browse/ACM-26882)
- **Existing Pattern:** `pkg/operator/operators/klusterlet/controllers/addonsecretcontroller/controller.go`
