[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Cluster Auto-Import

The **cluster auto-import** feature automatically imports a managed cluster into a ACM hub. This document provides an overview of the auto-import feature, including different methods and configurations.

## Overview

When importing a cluster, the **import-controller** running on the hub cluster applies the necessary manifests to the target Kubernetes cluster. These manifests install the **klusterlet** agent and initiate the cluster registration process.

Once triggered, the import process continues retrying until the managed cluster successfully joins the hub. If an attempt fails, the next attempt will start after a backoff period.

When the process completes, the target cluster joins the ACM hub as a managed cluster.

This **auto-import** process serves as an alternative to **manual import**, where users copy a command from the ACM console and run it on the target cluster to perform the same installation and bootstrap steps.

## Auto-Import Methods

There are two primary methods for initiating an auto-import:

1.  **Console-Based Import**: When importing a cluster through the ACM console, you provide either a **kubeconfig** or a **kube-apiserver endpoint and token** with cluster-admin permissions. The import-controller then uses these credentials to connect to the cluster and install the klusterlet.

2.  **CLI-Based Import**: For CLI-driven or automated environments, you can trigger an auto-import by creating a specific secret and other required resources on the hub cluster.

## CLI-Based Auto-Import Guide

To auto-import a managed cluster using the CLI, follow these steps on the hub cluster:

### 1. Create a Namespace

Create a namespace on the hub cluster with the same name as the managed cluster you intend to import.

```shell
kubectl create ns <cluster_name>
```

### 2. Create the Auto-Import Secret

In the newly created namespace, create a secret named `auto-import-secret`. This secret must contain the credentials for accessing the managed cluster. The import-controller uses this secret to connect to the managed cluster and will delete the secret once the import process is complete (whether it succeeds or fails).

You can provide the credentials in one of two formats:

*   **Kubeconfig**:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: auto-import-secret
      namespace: <cluster_name>
    stringData:
      autoImportRetry: "5" # Optional: Number of retries
      kubeconfig: |-
        <kubeconfig_content>
    type: Opaque
    ```

*   **API Server URL and Token**:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: auto-import-secret
      namespace: <cluster_name>
    stringData:
      autoImportRetry: "5" # Optional: Number of retries
      token: <token>
      server: <api_server_url>
    type: Opaque
    ```

The optional `autoImportRetry` field specifies the number of times the import-controller will attempt to import the cluster. If not specified, it defaults to a system-defined retry mechanism. If the import fails, the `ManagedClusterImportSucceeded` condition on the `ManagedCluster` resource will be set to `False` with a reason and message.

### 3. Create a ManagedCluster Resource

Create a `ManagedCluster` custom resource in the same namespace on the hub cluster:

```yaml
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: <cluster_name>
spec:
  hubAcceptsClient: true
```

### 4. Create a KlusterletAddonConfig Resource

To enable addons on the managed cluster, create a `KlusterletAddonConfig` resource in the same namespace on the hub cluster:

```yaml
apiVersion: agent.open-cluster-management.io/v1
kind: KlusterletAddonConfig
metadata:
  name: <cluster_name>
  namespace: <cluster_name>
spec:
  clusterName: <cluster_name>
  clusterNamespace: <cluster_name>
  applicationManager:
    enabled: true
  policyController:
    enabled: true
  searchCollector:
    enabled: true
  certPolicyController:
    enabled: true
  iamPolicyController:
    enabled: true
  version: 2.2.0 # Specify the desired addon version
```

## Validation

After creating these resources, the import-controller will begin the import process. Here’s how to validate the different stages:

### Klusterlet Installation

The import-controller generates a secret named `<cluster_name>-import`, which contains the manifests for installing the klusterlet. The controller then applies these manifests to the managed cluster.

*   **Check Pod Status on Managed Cluster**:

    ```shell
    kubectl get pod -n open-cluster-management-agent
    ```

### Certificate Signing Request (CSR)

Once the klusterlet agent is running on the managed cluster, it will create a Certificate Signing Request (CSR) on the hub cluster. This CSR is automatically approved.

*   **Check for CSR on Hub Cluster**:

    ```shell
    kubectl get csr
    ```

    You should see a CSR with a name prefixed by your cluster name in a `Pending` state, which will then transition to `Approved,Issued`.

### Managed Cluster Status

After the CSR is approved, the managed cluster will join the hub.

*   **Check Managed Cluster Status on Hub Cluster**:

    ```shell
    kubectl get managedclusters <cluster_name> -o yaml
    ```

    The status should show `ManagedClusterJoined` and `ManagedClusterConditionAvailable` as `True`. The `ManagedClusterImportSucceeded` condition indicates the status of the klusterlet installation.

### Addon Installation

The `KlusterletAddonConfig` resource triggers the klusterlet addon controller to create `ManifestWork` resources for the addons in the `<cluster_name>` namespace on the hub. These `ManifestWork` resources are then applied to the managed cluster.

*   **Check Addon Pods on Managed Cluster**:

    ```shell
    kubectl get pods -n open-cluster-management-agent-addon
    ```

---

# Auto-Import Strategy

The auto-import feature has two strategies that determine whether the import process is a one-time event or a continuous synchronization:

### `ImportOnly`

The import-controller applies the klusterlet manifests to the managed cluster only if the `ManagedClusterImportSucceeded` condition is missing or not `True`. Once the cluster joins the hub, the import-controller stops applying the manifests. This is the default behavior in ACM 2.14 and later.

### `ImportAndSync`

The import-controller applies the klusterlet manifests and continues to synchronize them with the hub configuration even after the managed cluster has joined. This was the default behavior in ACM 2.13 and earlier.

## Configuring the Auto-Import Strategy

Since ACM 2.14, you can override the default auto-import strategy by updating a `ConfigMap`:

*   **Name**: `import-controller-config`
*   **Namespace**: The namespace where the multicluster engine operator is installed.
*   **Key**: `autoImportStrategy` (set to `ImportOnly` or `ImportAndSync`)

If the `ConfigMap` or the key does not exist, the system uses the default strategy.

---

# Annotations Affecting Auto-Import

Several annotations on the `ManagedCluster` resource can be used to control the auto-import behavior.

## `import.open-cluster-management.io/disable-auto-import`

Introduced in ACM 2.10. The import-controller honors this annotation when the key is present. The value is ignored, so `''` and `"true"` have the same effect. The business continuity procedure sets an empty value.

While the annotation is present on a `ManagedCluster`, auto-import is disabled unconditionally. The auto-import strategy, the `ManagedClusterImportSucceeded` condition, and `immediate-import` do not override it.

*   The import-controller does not apply klusterlet manifests to that cluster. This covers Hive, `auto-import-secret`, and local-cluster import.
*   The manifestwork controller sets `updateStrategy.type: ReadOnly` on every manifest in the `<cluster>-klusterlet` and `<cluster>-klusterlet-crds` ManifestWorks. The work-agent does not create or update those objects.

The cluster stays connected if it was already imported. The annotation does not detach it.

Removing the annotation requeues the import reconcilers and clears those `ReadOnly` ManifestConfigs. In ACM 2.14 and later, those reconcilers apply klusterlet manifests only when `ManagedClusterImportSucceeded` is not `True` or the strategy is `ImportAndSync`. The auto-import reconciler also needs `auto-import-secret`. That secret is deleted after a successful import unless it has `managedcluster-import-controller.open-cluster-management.io/keeping-auto-import-secret`. The Hive reconciler uses the Hive admin kubeconfig and does not need `auto-import-secret`.

### True disaster recovery, with the primary hub down

`disable-auto-import` is unnecessary for a failover while the primary hub is down, when that hub uses `ImportOnly`.

Two paths on the primary hub can write `bootstrap-hub-kubeconfig` on the spoke. The import-controller writes it with the managed-cluster client. The work-agent on the spoke writes it by applying the klusterlet ManifestWork it reads from the hub it is registered to. While the primary hub is down, the import-controller is not running, and the spoke cannot read that hub's ManifestWorks.

The restore hub writes `bootstrap-hub-kubeconfig`. The registration agent connects to the restore hub, and the work-agent then applies ManifestWorks from the restore hub.

When the primary hub starts again, `ImportOnly` skips the import-controller apply because `ManagedClusterImportSucceeded` is already `True`. The spoke's work-agent is connected to the restore hub and applies that hub's ManifestWorks. The spoke stays registered to the restore hub.

This is the state after the spoke has switched. If the primary hub starts while the spoke's work-agent is still connected to it, that work-agent can still apply the primary hub's klusterlet ManifestWork and copy the old bootstrap secret back. `ImportOnly` does not change the ManifestWork update strategy.

Under `ImportAndSync`, the Hive and local-cluster reconcilers apply klusterlet manifests again after the primary hub starts. A Hive cluster still has its admin kubeconfig, so that apply can write the primary hub bootstrap secret back onto the spoke. A cluster imported with `auto-import-secret` does not get that re-apply: the secret is deleted after a successful import unless it has the keeping annotation, and the auto-import reconciler returns when the secret is missing.

Use `ImportOnly` on the primary hub for this recovery path. A fresh hub uses `ImportOnly` by default. A hub upgraded from before MCE 2.9 keeps `ImportAndSync` until `import-controller-config` is changed.

### Both hubs active during a disaster recovery test

The annotation matters when the primary hub stays up, as in a disaster recovery simulation. Both hubs then try to manage the same clusters.

The restore hub writes `bootstrap-hub-kubeconfig` on the managed cluster so the registration agent connects to the restore hub. Until that agent restarts, the work-agent on the managed cluster is still connected to the primary hub and resyncs the klusterlet ManifestWork from there. That ManifestWork still contains the primary hub bootstrap secret. With the default `Update` strategy, the work-agent copies that secret back, and the cluster returns to the primary hub.

`ImportOnly` leaves this path in place. It only skips the import-controller's direct apply after `ManagedClusterImportSucceeded` is `True`. The klusterlet ManifestWorks remain on the primary hub, and the work-agent still applies them.

Before the restore, set the annotation on the **primary hub** for every `ManagedCluster` that should move:

```yaml
metadata:
  annotations:
    import.open-cluster-management.io/disable-auto-import: ''
```

On the primary hub this has two results:

1.  The import-controller does not re-apply klusterlet manifests or refresh the bootstrap secret for that cluster.
2.  The klusterlet ManifestWorks become `ReadOnly`, so the work-agent that is still connected to the primary hub does not overwrite the bootstrap secret the restore hub just wrote.

The managed cluster then stays with the restore hub while the primary hub is still running. Leave the annotation on the primary hub until that hub should manage the cluster again. Do not set it on the restore hub for clusters that hub should import.

This is the "Disable the automatic import for managed clusters" step in [Run the restore operation while the primary hub cluster is active](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.17/html/business_continuity/business-cont-overview#keep-hub-active-restore).

### Comparison with `spec.hubAcceptsClient: false`

`disable-auto-import` and `spec.hubAcceptsClient: false` are for a cluster that has already joined and is Available. They do different jobs. An empty `disable-auto-import` value behaves the same as `"true"`.

While the setting is applied:

| | `disable-auto-import` | `hubAcceptsClient: false` |
| --- | --- | --- |
| Purpose | Stop this hub from pushing klusterlet manifests. | Stop this hub from accepting the klusterlet. |
| Hub applies manifests | Stopped unconditionally. Strategy, import status, and `immediate-import` do not override it. Klusterlet ManifestWorks are `ReadOnly`. | Still attempted. Registration cannot finish, because the hub no longer grants the agent access. |
| Spoke connection | Stays connected. | The hub removes the registration-agent and work-agent cluster role bindings, so those agents lose permission to call this hub. With the `MultipleHubs` feature, the registration agent restarts and selects another bootstrap kubeconfig. |
| `ManagedCluster` status | Stays Available. | `Available` becomes `Unknown` after 5 lease intervals. The default lease interval is 60 seconds, so the default is about 5 minutes. |

When the setting is undone:

| | Remove `disable-auto-import` | Set `hubAcceptsClient: true` |
| --- | --- | --- |
| Auto-import | Starts only when `ManagedClusterImportSucceeded` is not `True`, or the strategy is `ImportAndSync`. | Does not start. |
| ManifestWorks | `ReadOnly` is cleared. The work-agent applies klusterlet manifests again. | Unchanged by this field. |
| Spoke connection | Unchanged. The cluster was still connected. | Setting the field to `true` recreates the agent RBAC. The cluster can become Available again while the klusterlet is still installed and its hub client certificate is still valid. |

## `import.open-cluster-management.io/immediate-import`

Introduced in ACM 2.14. An empty value forces the hub to apply klusterlet manifests for that `ManagedCluster` again.

Only an empty value starts an import. The import-controller ignores every other value.

| Value | Effect |
| --- | --- |
| `''` | Starts an import on the next reconcile. |
| `Completed` | Ignored. The hub has finished the import this annotation requested. |
| Any other non-empty value | Ignored. |

The auto-import, Hive, and local-cluster reconcilers watch `ManagedCluster` updates. While the value is empty, an update of that `ManagedCluster` enqueues a reconcile. After the value is `Completed`, this annotation no longer enqueues a reconcile.

An empty value changes the `ImportOnly` check:

*   Under `ImportOnly`, the hub stops applying klusterlet manifests once `ManagedClusterImportSucceeded` is `True`. An empty `immediate-import` skips that check when an import reconciler observes it, so that reconciler applies the manifests again on a cluster that is already imported. The import-status controller sets the value to `Completed` as soon as the `<cluster>-klusterlet` ManifestWork is available. On a cluster that is already imported, that ManifestWork is already available, so import-status can set `Completed` on the same update that added the empty value. If that update lands before an import reconciler runs, `ImportOnly` skips the re-apply.
*   Under `ImportAndSync`, the hub already reapplies when the auto-import secret or a klusterlet ManifestWork changes. The empty annotation is how to start that apply from a `ManagedCluster` update alone.

The apply uses the same credentials as a normal auto-import: the `auto-import-secret`, the Hive admin kubeconfig, or the hub client for the local cluster. The import-controller waits until the klusterlet ManifestWorks exist, then applies the import manifests on the managed cluster.

`disable-auto-import` is checked first and disables auto-import unconditionally. While it is present, the reconcile returns before `immediate-import` is read. An empty `immediate-import` does not start an import, and it does not clear `ReadOnly` on the klusterlet ManifestWorks.

When the klusterlet ManifestWork is available, the import-status controller sets the annotation value to `Completed`. The key stays on the `ManagedCluster`. The controller does not delete it. If the annotation is missing or already `Completed`, that update does nothing.

While the value stays empty, a failed apply is retried. After the value is `Completed`, `ImportOnly` again skips a cluster whose import has succeeded. To force another import, set the value back to an empty string.
