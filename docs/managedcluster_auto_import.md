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

## `import.open-cluster-management.io/disable-auto-import: "true"`

Introduced in ACM 2.10, this annotation stops the import-controller from attempting to import a `ManagedCluster`.

The behavior when the annotation is removed depends on the ACM version and the auto-import strategy. In ACM 2.14 and later, an import is initiated only if the `ManagedClusterImportSucceeded` condition is not `True` or if the strategy is `ImportAndSync`.

### Comparison: `spec.hubAcceptsClient: false` vs. `disable-auto-import: "true"`

The table below outlines the differences that occur after setting `spec.hubAcceptsClient` to `false` or adding the annotation `disable-auto-import: "true"` to a `ManagedCluster` that has already joined the hub and is currently in an available state.

| Setting                     | Auto-Import Disabled                                                                              | Disconnection from Hub                                                                                                   | Cluster State           | Behavior on Removal                                                                                             |
| --------------------------- | -------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | ----------------------- | --------------------------------------------------------------------------------------------------------------- |
| `disable-auto-import: "true"` | Yes, the import-controller will not attempt to import the cluster.                                 | No, the cluster remains connected if it was already imported.                                                            | No change               | Whether an auto-import process is triggered depends on the auto-import strategy and the current state of the `ManagedClusterImportSucceeded` condition.           |
| `hubAcceptsClient: false`   | No, the auto-import process may still be initiated, but the registration cannot be completed because of insufficient permissions. | Partial; certificate rotation and lease renewal stop, but the work agent continues to communicate until its client certificate expires. | Becomes "Unknown" after ~5 minutes. | No auto-import process will be triggered. However, the cluster may eventually appear as available if the klusterlet remains installed and the bootstrap hub kubeconfig is still valid. |

## `import.open-cluster-management.io/immediate-import`

Introduced in ACM 2.14, adding this annotation (with an empty value) to a `ManagedCluster` resource triggers an immediate import process, regardless of the configured auto-import strategy.

*   If the import succeeds, the annotation's value is updated to `Completed`.
*   If the import fails, the controller will retry with an exponential backoff.

**Note**: This annotation has no effect if the `disable-auto-import` annotation is present.
