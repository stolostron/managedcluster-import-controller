# TLS Profile Compliance Design for Server Foundation Repos

**JIRA:** [ACM-26882: [ACM] Central TLS Profile consistency](https://issues.redhat.com/browse/ACM-26882)
**Document Status:** Design Document
**Last Updated:** 2026-03-12
**Deadline:** OCP 4.22 GA

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

This design document provides a unified approach for all server foundation repositories to implement TLS profile compliance as required by OpenShift 4.22 GA. The requirement mandates that all components **dynamically fetch and apply TLS configuration from centralized sources** rather than hardcoding TLS settings. This is critical for **Post-Quantum Cryptography (PQC) readiness**.

### Challenge for OCM

OCM repos are **upstream Kubernetes projects** that must work on any Kubernetes distribution. They **cannot depend on OpenShift-specific APIs** like `APIServer.spec.tlsSecurityProfile`.

### Background: Deployment Relationships

Understanding who deploys what is critical to the design:

**Hub Cluster:**
- **backplane-operator** (Stolostron) deploys:
  - `cluster-manager-operator` (OCM operator, scenario 3)
  - `cluster-proxy-addon-manager` (OCM addon manager, scenario 5)
- **cluster-manager-operator** (OCM) deploys:
  - `registration-controller` (scenario 4)
  - `work-controller` (scenario 4)
  - `placement-controller` (scenario 4)

**Managed Cluster:**
- **import-controller** (Stolostron) deploys:
  - `klusterlet-operator` (OCM operator, scenario 6)
- **klusterlet-operator** (OCM) deploys:
  - `klusterlet-agent` (registration-agent, work-agent) (scenario 7)
- **Addon agents** (scenario 8) are deployed by respective addon managers
- **klusterlet-operator** already has capability to copy image pull secrets to addon namespaces

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
6. **Addon Flexibility**: For addon managers (Scenario 5) and addon agents (Scenario 8), this design is **for reference only**. Each addon squad can decide whether to adopt this pattern or implement their own solution. We do not enforce using this specific proposal for addons.

---

## Stolostron Scenarios

### Scenario 1: Stolostron Hub (OpenShift)

**Components:**
- All Stolostron-specific hub components

**Solution:**
Refer to [Hint for resolving TLS non-compliance tickets Code Examples](https://docs.google.com/document/d/1234567890) for implementation.

**Key Points:**
- Use OpenShift library-go TLS helpers
- Watch `APIServer.spec.tlsSecurityProfile` directly
- No sidecar needed (native OpenShift code)

---

### Scenario 2: Stolostron Spoke (OpenShift)

**Components:**
- All Stolostron-specific managed cluster components

**Solution:**
Refer to [Hint for resolving TLS non-compliance tickets Code Examples](https://docs.google.com/document/d/1234567890) for implementation.

**Key Points:**
- Use OpenShift library-go TLS helpers
- Watch managed cluster's `APIServer.spec.tlsSecurityProfile`
- No sidecar needed (native OpenShift code)

---

## OCM Hub Scenarios

### Scenario 3: OCM Hub - cluster-manager-operator

**Component:** `cluster-manager-operator` (registration-operator in cluster-manager mode)

**Deployed by:** backplane-operator (Stolostron)

**Platform:** OpenShift (when deployed by Stolostron) or Kubernetes (upstream)

**Architecture Flow:**

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

**How It Works:**

1. **Sidecar Injection** (Downstream - backplane-operator)
   - backplane-operator detects OpenShift platform
   - Injects `tls-profile-sync` sidecar into cluster-manager-operator pod

2. **ConfigMap Creation** (Sidecar)
   - Sidecar watches hub's `APIServer.spec.tlsSecurityProfile`
   - Creates/updates `ocm-tls-profile` ConfigMap in `multicluster-engine` namespace

3. **Component Consumption** (Upstream - cluster-manager-operator)
   - Watches `ocm-tls-profile` ConfigMap using standard Kubernetes client-go
   - When ConfigMap changes → applies new TLS config → **restarts itself**
   - If ConfigMap not found (vanilla K8s) → falls back to TLS 1.2

**Code Owner:**
- Sidecar injection: Downstream (backplane-operator)
- Sidecar container: Downstream (stolostron/import-controller->tls-profile-sync)
- ConfigMap watching + restart logic: Upstream (registration-operator)

---

### Scenario 4: OCM Hub - ocm-hub-components

**Components:**
- registration-controller
- work-controller (work-webhook)
- placement-controller

**Deployed by:** cluster-manager-operator (OCM)

**Deployed in:** `open-cluster-management-hub` namespace (all components in same namespace)

**Platform:** OpenShift (when operator deployed by Stolostron) or Kubernetes (upstream)

**Architecture Flow:**

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

**How It Works:**

1. **Operator Reads Source ConfigMap**
   - cluster-manager-operator (in `multicluster-engine` namespace) watches `multicluster-engine/ocm-tls-profile`

2. **Operator Creates ConfigMap in Hub Components Namespace**
   - Creates/updates `ocm-tls-profile` in `open-cluster-management-hub` namespace
   - Single ConfigMap shared by all hub components (they're all in the same namespace)

3. **Components Watch Their Namespace's ConfigMap**
   - All hub components watch `open-cluster-management-hub/ocm-tls-profile`
   - On ConfigMap change → applies TLS config → **restarts**
   - If ConfigMap not found → falls back to TLS 1.2

**Code Owner:**
- ConfigMap propagation logic: Upstream (cluster-manager-operator)
- ConfigMap watching + restart: Upstream (each component)

---

### Scenario 5: OCM Hub - addon-manager

> **Note:** This scenario is **for reference only**. Each addon squad can decide whether to adopt this pattern or implement their own solution.

**Components:**
- cluster-proxy-addon-manager (deployed in `multicluster-engine` namespace)
- Other addon managers like submariner (deployed in their own namespaces)

**Deployed by:** backplane-operator (Stolostron)

**Platform:** OpenShift (when deployed by Stolostron) or Kubernetes (upstream)

#### Sub-case 5a: cluster-proxy-addon-manager (Same Namespace as Operator)

**Deployed in:** `multicluster-engine` namespace (same as cluster-manager-operator)

**Architecture Flow:**

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

**How It Works:**

1. **No Sidecar Needed**
   - cluster-proxy-addon-manager deployed in same namespace as cluster-manager-operator
   - Reads the same ConfigMap created by cluster-manager-operator's sidecar

2. **Component Consumption**
   - Watches `multicluster-engine/ocm-tls-profile`
   - On ConfigMap change → applies TLS config → **restarts**
   - If ConfigMap not found → falls back to TLS 1.2

**Code Owner:**
- All upstream (no sidecar injection needed)

#### Sub-case 5b: Other Addon Managers (Different Namespace)

**Example:** submariner-addon-manager

**Deployed in:** Addon-specific namespace (e.g., `submariner-operator`)

**Architecture Flow:**

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

**How It Works:**

1. **Sidecar Injection** (Downstream - backplane-operator)
   - backplane-operator detects addon manager in different namespace
   - Injects `tls-profile-sync` sidecar into addon manager pod

2. **ConfigMap Creation** (Sidecar)
   - Sidecar watches hub's `APIServer.spec.tlsSecurityProfile`
   - Creates/updates `ocm-tls-profile` in addon's namespace (e.g., `submariner-operator`)

3. **Component Consumption** (Upstream - addon manager)
   - Watches ConfigMap in its own namespace
   - On change → applies TLS config → **restarts**
   - If ConfigMap not found → falls back to TLS 1.2

**Code Owner:**
- Sidecar injection: Downstream (backplane-operator)
- Sidecar container: Downstream (stolostron/tls-profile-sync)
- ConfigMap watching + restart: Upstream (addon-manager code)

---

## OCM Spoke Scenarios

### Scenario 6: OCM Spoke - klusterlet-operator

**Component:** `klusterlet-operator` (registration-operator in klusterlet mode)

**Deployed by:** import-controller (Stolostron)

**Platform:** OpenShift (when deployed by Stolostron) or Kubernetes (upstream)

**Architecture Flow:**

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

**How It Works:**

1. **Sidecar Injection** (Downstream - import-controller)
   - import-controller detects OpenShift platform on managed cluster
   - Injects `tls-profile-sync` sidecar into klusterlet-operator pod

2. **ConfigMap Creation** (Sidecar)
   - Sidecar watches **managed cluster's** `APIServer.spec.tlsSecurityProfile` (NOT hub!)
   - Creates/updates `ocm-tls-profile` in `open-cluster-management-agent` namespace

3. **Component Consumption** (Upstream - klusterlet-operator)
   - Watches `ocm-tls-profile` ConfigMap
   - On change → applies TLS config → **restarts**
   - If ConfigMap not found → falls back to TLS 1.2

**Important:** Each managed cluster uses its **OWN** TLS profile, not the hub's!

**Code Owner:**
- Sidecar injection: Downstream (import-controller)
- Sidecar container: Downstream (stolostron/tls-profile-sync)
- ConfigMap watching + restart: Upstream (registration-operator)

---

### Scenario 7: OCM Spoke - klusterlet-agent

**Components:**
- klusterlet-agent (singleton mode)
- registration-agent (default mode)
- work-agent (default mode)

**Deployed by:** klusterlet-operator (OCM)

**Platform:** OpenShift (when operator deployed by Stolostron) or Kubernetes (upstream)

#### Sub-case 7a: Default Mode (Same Namespace as Operator)

**Deployed in:** `open-cluster-management-agent` namespace (same as klusterlet-operator)

**Architecture Flow:**

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

**How It Works:**

1. **No ConfigMap Copy Needed**
   - klusterlet-operator and klusterlet agents in same namespace (`open-cluster-management-agent`)
   - All components read the same ConfigMap

2. **Component Consumption**
   - Agents watch `open-cluster-management-agent/ocm-tls-profile`
   - On change → apply TLS config → **restart**
   - If ConfigMap not found → fall back to TLS 1.2

**Code Owner:**
- All upstream (no Stolostron-specific code)

#### Sub-case 7b: Hosted Mode (Different Namespace)

**Deployed in:** Hosted namespace (e.g., `klusterlet-<managed-cluster-name>`)

**Architecture Flow:**

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

**How It Works:**

1. **Operator Reads Source ConfigMap**
   - klusterlet-operator (in `open-cluster-management-agent`) watches `open-cluster-management-agent/ocm-tls-profile`

2. **Operator Copies ConfigMap to Hosted Namespace**
   - klusterlet-operator runs a controller to detect hosted mode deployments
   - Copies ConfigMap to hosted namespace (e.g., `klusterlet-<managed-cluster-name>`)

3. **Component Consumption**
   - Agents in hosted namespace watch ConfigMap in their namespace
   - On change → apply TLS config → **restart**
   - If ConfigMap not found → fall back to TLS 1.2

**Note:** In hosted mode, the TLS profile comes from the **hosting cluster's** APIServer (where klusterlet-operator runs), not the managed cluster's APIServer.

**Code Owner:**
- ConfigMap copy logic: Upstream (klusterlet-operator)
- ConfigMap watching + restart: Upstream (agents)

---

### Scenario 8: OCM Spoke - addon-agent

> **Note:** This scenario is **for reference only**. Each addon squad can decide whether to adopt this pattern or implement their own solution.

**Components:**
- app-addon-agent
- policy-addon-agent
- observability-addon-agent
- cluster-proxy-addon-agent
- Any other addon agents

**Deployed by:** Respective addon managers (hub)

**Platform:** OpenShift (when klusterlet deployed by Stolostron) or Kubernetes (upstream)

**Architecture Flow:**

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

**How It Works:**

1. **ConfigMap Copy Controller** (Upstream - klusterlet-operator)
   - `AddonTLSConfigController` runs in klusterlet-operator
   - Pattern: Same as existing `AddonPullImageSecretController`
   - Watches namespaces labeled `addon.open-cluster-management.io/namespace: "true"`

2. **ConfigMap Propagation**
   - Source: `open-cluster-management-agent/ocm-tls-profile`
   - Destination: `addon-<name>/ocm-tls-profile`
   - On source update → update all copies

3. **Addon Agent Consumption**
   - Each addon agent watches ConfigMap in its own namespace
   - On change → apply TLS config → **restart**
   - If ConfigMap not found → fall back to TLS 1.2

**Why Copy ConfigMaps?**
- ✅ Namespace isolation (addons read from own namespace)
- ✅ Simpler RBAC (no cross-namespace access needed)
- ✅ Consistent with existing pattern (image pull secrets)

**Code Owner:**
- AddonTLSConfigController: Upstream (klusterlet-operator)
- ConfigMap watching + restart: Upstream (addon agent code)

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

Components watch the ConfigMap and restart when it changes:

**Pseudo-code:**
```go
// In component's main.go
func main() {
    // 1. Read TLS config from ConfigMap (or use fallback)
    tlsConfig := readTLSConfigFromConfigMap()

    // 2. Start ConfigMap watcher
    go watchConfigMapAndRestart()

    // 3. Start component with TLS config
    startComponent(tlsConfig)
}

func watchConfigMapAndRestart() {
    watcher := watchConfigMap("ocm-tls-profile")
    for event := range watcher.ResultChan() {
        if event.Type == watch.Modified {
            // Restart: exit with code 0, let Kubernetes restart the pod
            os.Exit(0)
        }
    }
}

func readTLSConfigFromConfigMap() *tls.Config {
    cm, err := client.CoreV1().ConfigMaps(namespace).Get("ocm-tls-profile")
    if err != nil {
        // ConfigMap not found → fallback to TLS 1.2
        return &tls.Config{MinVersion: tls.VersionTLS12}
    }

    // Parse ConfigMap and return TLS config
    return parseTLSConfig(cm)
}
```

### New Components

| Component | Repository | Owner | Purpose |
|---|---|---|---|
| tls-profile-sync sidecar | stolostron/tls-profile-sync | Downstream | Watches OpenShift APIServer, creates ConfigMap |
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

### Sidecar Injection Points

**Downstream Repos Responsible for Sidecar Injection:**

1. **backplane-operator** (Stolostron) injects sidecar into:
   - cluster-manager-operator pod
   - Addon-manager pods deployed in different namespaces (e.g., submariner-addon-manager)

2. **import-controller** (Stolostron) injects sidecar into:
   - klusterlet-operator pod

**Sidecar Detection Logic:**
```go
// In backplane-operator / import-controller
func shouldInjectSidecar() bool {
    // Check if platform is OpenShift
    _, err := client.Discovery().ServerResourcesForGroupVersion("config.openshift.io/v1")
    return err == nil
}
```

---

## Compliance Verification

**Tools:**
- `tls-scanner`: Validate no hardcoded TLS settings
- `semgrep`: Scan for `tls.VersionTLS` hardcoding
- E2E tests: Verify dynamic TLS profile changes

**Test Scenarios:**
1. Change OpenShift APIServer TLS profile → verify components restart with new config
2. Deploy on vanilla Kubernetes → verify components use TLS 1.2 fallback
3. Add new addon → verify ConfigMap copied to addon namespace
4. Sidecar crash → verify components continue with last known config

---

## Current Findings

### Identified Hardcoded TLS Settings

**File:** [pkg/common/options/webhook.go:97](pkg/common/options/webhook.go#L97)
```go
config.MinVersion = tls.VersionTLS12  // HARDCODED - needs remediation
```

**Remediation:**
Replace with shared TLS library that reads from ConfigMap or uses fallback.

**Expected Change:**
```go
// Before
TLSOpts: []func(config *tls.Config){
    func(config *tls.Config) {
        config.MinVersion = tls.VersionTLS12  // HARDCODED
    },
},

// After
TLSOpts: []func(config *tls.Config){
    GetTLSConfigFromConfigMap(ctx, client, namespace),  // Dynamic
},
```

---

## FAQ

**Q: Do upstream OCM repos need OpenShift dependencies?**
A: No. Upstream only uses standard Kubernetes client-go to read ConfigMaps.

**Q: What if sidecar crashes?**
A: Kubernetes restarts sidecar. Components continue with last known ConfigMap state.

**Q: Why restart components instead of hot-reload?**
A: Simpler implementation. TLS profile changes are infrequent (admin actions). Kubernetes handles graceful restart.

**Q: Can users customize TLS profile?**
A: Yes, via `APIServer.spec.tlsSecurityProfile` (OpenShift cluster-wide setting). Changes propagate automatically.

**Q: What about client TLS?**
A: Separate initiative. This design focuses on server TLS (webhook servers, metrics servers, etc.)

**Q: Why does each managed cluster use its own TLS profile?**
A: Managed cluster security admins control their own security policy. Managed cluster may require stricter or looser TLS than hub.

**Q: What if ConfigMap is deleted? Does the operator create it again?**

A: Yes, the ConfigMap is automatically recreated through Kubernetes reconciliation loops:

- **Sidecars**: The `tls-profile-sync` sidecar continuously watches the APIServer TLS profile and reconciles the ConfigMap. If deleted, it recreates the ConfigMap within seconds based on the current APIServer settings.
- **Operators**: Operators (cluster-manager-operator, klusterlet-operator) watch their source ConfigMaps and recreate managed ConfigMaps. For example, if `open-cluster-management-hub/ocm-tls-profile` is deleted, cluster-manager-operator recreates it from `multicluster-engine/ocm-tls-profile`.
- **During Recreation**: Components detect ConfigMap deletion and fall back to TLS 1.2 temporarily. Once the ConfigMap is recreated, components detect the change and restart to apply the correct TLS profile.
- **Result**: Brief service interruption during restart, but system self-heals automatically.

**Q: How does this support Post-Quantum Cryptography (PQC)?**
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
