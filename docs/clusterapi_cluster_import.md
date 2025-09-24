[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Auto Importing ClusterAPI Provisioned Clusters

This document describes how to automatically import clusters provisioned through ClusterAPI into ACM/MCE.

## Prerequisites

The following configurations are required and should be performed once on your hub cluster.

### 1. Enable the ClusterImporter Feature Gate

Enable the ClusterImporter feature gate on the ClusterManager to allow automatic import functionality:

```yaml
apiVersion: operator.open-cluster-management.io/v1
kind: ClusterManager
metadata:
  name: cluster-manager
spec:
  registrationConfiguration:
    featureGates:
    - feature: ClusterImporter
      mode: Enable
```

Apply this configuration:
```bash
kubectl apply -f <filename>.yaml
```

### 2. Enable Cluster Import Config Secret Creation

Configure the import controller to create cluster import configuration secrets:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: import-controller-config
  namespace: multicluster-engine
data:
  clusterImportConfig: "true"
```

**If the ConfigMap doesn't exist:**
```bash
kubectl apply -f <filename>.yaml
```

**If the ConfigMap already exists:**
```bash
kubectl patch configmap import-controller-config -n multicluster-engine --type merge -p '{"data":{"clusterImportConfig":"true"}}'
```

### 3. Grant CAPI Manager Permissions

Bind the ClusterAPI manager permissions to the import controller service account:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cluster-manager-registration-capi
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: capi-manager-role
subjects:
- kind: ServiceAccount
  name: registration-controller-sa
  namespace: open-cluster-management-hub
```

Apply this configuration:
```bash
kubectl apply -f <filename>.yaml
```


## Create the ClusterAPI Cluster

Create your ClusterAPI cluster following the standard ClusterAPI documentation:

- **ClusterAPI Installer Documentation**: [cluster-api-installer repository](https://github.com/stolostron/cluster-api-installer/tree/main/doc)
- **Official ClusterAPI Documentation**: [cluster-api.sigs.k8s.io](https://cluster-api.sigs.k8s.io/)

### ROSA Cluster example
```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: capi-rosa-cluster
  namespace: capi-rosa-cluster #  Must be the same with the cluster name
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1beta2
    kind: ROSAControlPlane
    name: capi-rosa-cluster-control-plane
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
    kind: ROSACluster
    name: capi-rosa-cluster
```

> **Note**: The ClusterAPI cluster's name and the namespace should be the same. 

## Create the ManagedCluster

To enable automatic import, create a ManagedCluster resource with the **same name and namespace** as your ClusterAPI cluster.


```yaml
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: capi-rosa-cluster  # Must be the same with the  ClusterAPI cluster name
spec:
  hubAcceptsClient: true
```

Apply the configuration:
```bash
kubectl apply -f <filename>.yaml
```

