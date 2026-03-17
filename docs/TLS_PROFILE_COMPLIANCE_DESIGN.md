# TLS Profile Compliance Design for Server Foundation Repos

**JIRA:** [ACM-26882: [ACM] Central TLS Profile consistency](https://issues.redhat.com/browse/ACM-26882)
**Document Status:** Design Document
**Last Updated:** 2026-03-12

---

## Table of Contents

1. [Overview](#overview)
2. [Solution Overview](#solution-overview)
3. [Scenario Summary](#scenario-summary)
4. [Stolostron Scenarios](#stolostron-scenarios)
5. [OCM Hub Scenarios](#ocm-hub-scenarios)
6. [OCM Spoke Scenarios](#ocm-spoke-scenarios)
7. [Implementation Details](#implementation-details)
8. [Compliance Verification](#compliance-verification)
9. [FAQ](#faq)
10. [Approval and Sign-off](#approval-and-sign-off)
11. [References](#references)

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
│ Operators (Scenarios 3 & 6):                                     │
│   → Watch ConfigMap "ocm-tls-profile"                            │
│   → Restart themselves on ConfigMap change                       │
│   → Read ConfigMap and inject flags into component deployments   │
│                                                                  │
│ Components (Scenarios 4 & 7):                                    │
│   → Receive TLS config via command-line flags                    │
│   → Restarted by operator when config changes (via annotation)   │
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
2. **Sidecar Injection**: Downstream (Stolostron) repos inject sidecar into operators when deploying on OpenShift
3. **Two Patterns**:
   - **Operators (Scenarios 3 & 6)**: Watch ConfigMap and self-restart
   - **Components (Scenarios 4 & 7)**: Receive TLS config via command-line flags from their operators
4. **Operator-Controlled Rollout**: Operators trigger component restarts via deployment annotation changes (follows OpenShift pattern)
5. **Safe Fallback**: Components use TLS 1.2 when config not provided (vanilla Kubernetes)
6. **Addon Flexibility**: Scenarios 5 & 8 are **for reference only**. Addon squads may implement their own solution.

---

## Scenario Summary

| Scenario | Component | Platform | Sidecar | ConfigMap Pattern | Solution |
|---|---|---|---|---|---|
| **1** | Stolostron Hub | OpenShift | ✅ | Direct consumption | [Refer to OpenShift hint doc](#scenario-1--2-stolostron-hub--spoke-openshift) |
| **2** | Stolostron Spoke | OpenShift | ✅ | Direct consumption | [Refer to OpenShift hint doc](#scenario-1--2-stolostron-hub--spoke-openshift) |
| **3** | OCM Hub - cluster-manager-operator | OpenShift/K8s | ✅/❌ | Watches + restarts | [Sidecar + ConfigMap](#scenario-3-ocm-hub---cluster-manager-operator) |
| **4** | OCM Hub - ocm-hub-components | OpenShift/K8s | ❌ | Operator passes flags | [Operator flag injection](#scenario-4-ocm-hub---ocm-hub-components) |
| **5** | OCM Hub - addon-manager | OpenShift/K8s | ✅/❌ | Watches + restarts | [Sidecar + ConfigMap](#scenario-5-ocm-hub---addon-manager) |
| **6** | OCM Spoke - klusterlet-operator | OpenShift/K8s | ✅/❌ | Watches + restarts | [Sidecar + ConfigMap](#scenario-6-ocm-spoke---klusterlet-operator) |
| **7** | OCM Spoke - klusterlet-agent | OpenShift/K8s | ❌ | Operator passes flags | [Operator flag injection](#scenario-7-ocm-spoke---klusterlet-agent) |
| **8** | OCM Spoke - addon-agent | OpenShift/K8s | ❌ | Operator copies ConfigMap | [ConfigMap copy pattern](#scenario-8-ocm-spoke---addon-agent) |
| **9** | cluster-proxy components (self-deployed by cluster proxy manager/agent) | OpenShift/K8s | TBD | TBD | TBD |

---

## Stolostron Scenarios

### Scenario 1 & 2: Stolostron Hub & Spoke (OpenShift)

**Solution:** Refer to [Hint for resolving TLS non-compliance tickets Code Examples](https://docs.google.com/document/d/1cMc9E8psHfnoK06ntR8kHSWB8d3rMtmldhnmM4nImjs)

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
│ │ • Renders deployments with TLS flags                        │ │
│ │ • Watches ConfigMap, triggers rollout on change             │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Renders deployment with flags            │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Deployment: registration-controller                         │ │
│ │ Namespace: open-cluster-management-hub                      │ │
│ │                                                             │ │
│ │ spec:                                                       │ │
│ │   template:                                                 │ │
│ │     metadata:                                               │ │
│ │       annotations:                                          │ │
│ │         tls-config-hash: abc123...  # Triggers rollout      │ │
│ │     spec:                                                   │ │
│ │       containers:                                           │ │
│ │       - name: registration-controller                       │ │
│ │         command:                                            │ │
│ │         - /registration-controller                          │ │
│ │         - --tls-min-version=VersionTLS13                    │ │
│ │         - --tls-cipher-suites=TLS_AES_128_GCM_SHA256,...    │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ (work-controller and placement-controller use same pattern)     │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation (Flag Approach - follows OpenShift pattern):**

- **Operator reads ConfigMap:** cluster-manager-operator watches `multicluster-engine/ocm-tls-profile`
- **Operator renders flags:** Injects TLS values directly as command-line flags in component deployments
- **Operator triggers rollout:** Updates deployment annotation `tls-config-hash` when ConfigMap changes
- **Components read flags:** Parse `--tls-min-version` and `--tls-cipher-suites` on startup
- **Kubernetes handles restart:** Deployment rollout triggered by annotation change

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
**Namespace:** `open-cluster-management-agent` (default mode) or `klusterlet-<cluster-name>` (hosted mode)

```
┌─────────────────────────────────────────────────────────────────┐
│ Managed/Hosting Cluster                                         │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ klusterlet-operator (Scenario 6)                            │ │
│ │ Namespace: open-cluster-management-agent                    │ │
│ │                                                             │ │
│ │ • Reads: open-cluster-management-agent/ocm-tls-profile      │ │
│ │ • Renders deployments with TLS flags                        │ │
│ │ • Watches ConfigMap, triggers rollout on change             │ │
│ └────────────────────┬────────────────────────────────────────┘ │
│                      │ Renders deployment with flags            │
│                      │                                          │
│                      ▼                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Deployment: registration-agent                              │ │
│ │ Namespace: <target-namespace>                               │ │
│ │   • Default mode: open-cluster-management-agent             │ │
│ │   • Hosted mode: klusterlet-<managed-cluster-name>          │ │
│ │                                                             │ │
│ │ spec:                                                       │ │
│ │   template:                                                 │ │
│ │     metadata:                                               │ │
│ │       annotations:                                          │ │
│ │         tls-config-hash: def456...  # Triggers rollout      │ │
│ │     spec:                                                   │ │
│ │       containers:                                           │ │
│ │       - name: registration-agent                            │ │
│ │         command:                                            │ │
│ │         - /registration-agent                               │ │
│ │         - --tls-min-version=VersionTLS12                    │ │
│ │         - --tls-cipher-suites=...                           │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ (work-agent and klusterlet-agent use same pattern)              │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation (Flag Approach - follows OpenShift pattern):**

- **Operator reads ConfigMap:** klusterlet-operator watches `open-cluster-management-agent/ocm-tls-profile` (always from operator's namespace)
- **Operator renders flags:** Injects TLS values directly as command-line flags in agent deployments
- **Target namespace is just a parameter:** Operator deploys to `open-cluster-management-agent` (default) or `klusterlet-<cluster-name>` (hosted)
- **No ConfigMap copy needed:** With flags, operator reads once and injects into deployments regardless of target namespace
- **Operator triggers rollout:** Updates deployment annotation `tls-config-hash` when ConfigMap changes
- **Agents read flags:** Parse `--tls-min-version` and `--tls-cipher-suites` on startup

**Key insight:** The flag approach makes default vs. hosted mode irrelevant for TLS configuration - it's just a deployment namespace parameter!

**Note:** TLS profile comes from **hosting cluster's** APIServer (where klusterlet-operator runs), not managed cluster's APIServer.

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

#### Pattern 1: Operators (Scenarios 3 & 6) - ConfigMap Watch + Self-Restart

```go
// Operators watch ConfigMap and restart themselves
func main() {
    tlsConfig := readTLSConfigFromConfigMap()
    go watchConfigMapAndRestart()
    startOperator(tlsConfig)
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

#### Pattern 2: Components (Scenarios 4 & 7) - Flag-Based + Operator-Triggered Rollout

```go
// Components parse flags on startup (no ConfigMap watch)
func main() {
    tlsConfig := readTLSConfigFromFlags()
    startComponent(tlsConfig)
}

func readTLSConfigFromFlags() *tls.Config {
    minVersion := flag.String("tls-min-version", "", "Minimum TLS version")
    cipherSuites := flag.String("tls-cipher-suites", "", "TLS cipher suites")
    flag.Parse()

    if *minVersion == "" {
        return &tls.Config{MinVersion: tls.VersionTLS12}  // Fallback
    }
    return parseTLSConfig(*minVersion, *cipherSuites)
}
```

#### Operator Logic (watches ConfigMap and triggers component rollouts)

```go
// In cluster-manager-operator or klusterlet-operator
func reconcileComponents() {
    cm, err := client.CoreV1().ConfigMaps(operatorNamespace).Get("ocm-tls-profile")
    if err != nil {
        // Use TLS 1.2 defaults
        cm = getDefaultTLSConfig()
    }

    // Render deployment with TLS flags
    deployment := renderDeployment(
        "--tls-min-version=" + cm.Data["minTLSVersion"],
        "--tls-cipher-suites=" + cm.Data["cipherSuites"],
    )

    // Add hash annotation to trigger rollout on config change
    hash := hashConfigMap(cm.Data)
    deployment.Spec.Template.Annotations["tls-config-hash"] = hash

    // Apply deployment (Kubernetes triggers rollout if hash changed)
    client.AppsV1().Deployments(targetNamespace).Apply(deployment)
}
```

### New Components

| Component | Repository | Owner | Purpose |
|---|---|---|---|
| tls-profile-sync sidecar | stolostron/import-controller->tls-profile-sync | Downstream | Watches OpenShift APIServer, creates ConfigMap |
| Shared TLS library/helpers | open-cluster-management-io/sdk-go | Upstream | Flag parsing, fallback logic, TLS config helpers |
| AddonTLSConfigController | open-cluster-management-io/registration-operator | Upstream | Copies ConfigMap to addon namespaces (in klusterlet-operator) |
| Addon ConfigMap watch + restart | open-cluster-management-io/addon-framework | Upstream | Common addon functionality to watch ConfigMap and restart |

### Modified Components

| Component | Repository | Modification | Owner |
|---|---|---|---|
| backplane-operator | stolostron/backplane-operator | Inject sidecar for cluster-manager-operator, addon-managers | Downstream |
| import-controller | stolostron/import-controller | Inject sidecar for klusterlet-operator | Downstream |
| cluster-manager-operator | open-cluster-management-io/registration-operator | Watch ConfigMap, self-restart on change, inject flags into hub components | Upstream |
| klusterlet-operator | open-cluster-management-io/registration-operator | Watch ConfigMap, self-restart on change, inject flags into agents, run AddonTLSConfigController | Upstream |
| Hub components (reg/work/placement) | open-cluster-management-io/ocm | Parse TLS flags on startup using sdk-go | Upstream |
| Spoke agents (klusterlet/reg/work) | open-cluster-management-io/ocm | Parse TLS flags on startup using sdk-go | Upstream |
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

**Q: Why use flags for components (Scenarios 4 & 7) but ConfigMap watch for operators (Scenarios 3 & 6)?**
A: Operators manage their components' lifecycles, so they can inject flags and trigger rollouts. This follows the OpenShift pattern and reduces component complexity. Operators themselves use ConfigMap watch since they're not managed by another controller.

**Q: Does the flag approach work differently for hosted vs. default mode?**
A: No! This is a key advantage of the flag approach. The operator always reads ConfigMap from its own namespace (`open-cluster-management-agent`) and renders flags into deployments. The target namespace (`open-cluster-management-agent` for default or `klusterlet-<cluster-name>` for hosted) is just a parameter - the TLS logic is identical. No ConfigMap copying needed!

**Q: Can users customize TLS profile?**
A: Yes, via `APIServer.spec.tlsSecurityProfile`. Changes propagate automatically.

**Q: What about client TLS?**
A: **Client TLS is a separate initiative and NOT in scope for this design.** This design focuses exclusively on **server TLS** (HTTPS servers that accept connections, such as webhooks and metrics servers).

**Why client TLS is separate:**

- **Server-side TLS is the current focus** per the [OpenShift TLS compliance hint document](https://docs.google.com/document/d/1cMc9E8psHfnoK06ntR8kHSWB8d3rMtmldhnmM4nImjs)
- **Aligning Kubernetes client configuration to the cluster's TLS profile is a separate, later initiative**
- **Clients should use a modern TLS stack and not artificially limit negotiation** (e.g., able to negotiate TLS 1.3 when the server supports it)
- Setting client `MinVersion` from the hub's TLS profile (e.g., Modern = TLS 1.3 only) could **break connections** to servers that only support TLS 1.2 (e.g., ROKS clusters, external APIs)

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

- **OpenShift Requirement:** [Hint for resolving TLS non-compliance tickets Code Examples](https://docs.google.com/document/d/1cMc9E8psHfnoK06ntR8kHSWB8d3rMtmldhnmM4nImjs)
- **OpenShift Pattern:** [Centralized TLS Configuration Enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/security/centralized-tls-config.md)
- **JIRA:** [ACM-26882: [ACM] Central TLS Profile consistency](https://issues.redhat.com/browse/ACM-26882)
- **Existing Pattern:** `pkg/operator/operators/klusterlet/controllers/addonsecretcontroller/controller.go`
