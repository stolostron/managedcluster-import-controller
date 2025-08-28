[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Auto importing of a ClusterAPI provisioned cluster

## Prereq

These are one time configurations.

### Enable the ClusterImporter feature gates on the ClusterManager

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
    - feature: ManagedClusterAutoApproval
      mode: Enable
    autoApproveUsers:
    - system:serviceaccount:multicluster-engine:agent-registration-bootstrap
```
run `kubectl apply -f` to apply the above yaml content to ClusterManager.

### Bind the CAPI manager permission to the import controller

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cluster-manager-registration-capi
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: capi-operator-manager-role
subjects:
- kind: ServiceAccount
  name: registration-controller-sa
  namespace: open-cluster-management-hub
```
run `kubectl apply -f` to apply the above yaml content to create the clusterrolebinding.


## Create cluster-info configmap

- Get the CA bundle from the hub cluster
```shell
kubectl get cm -n kube-public kube-root-ca.crt -o yaml | yq '.data."ca.crt"' | base64
```
- build the configmap
```shell
apiVersion: v1
data:
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        server: {APIServer address}
        certificate-authority-data: {CA data obtained from the last step}
        name: ""
    contexts: null
    current-context: ""
    kind: Config
    preferences: {}
    users: null
kind: ConfigMap
metadata:
  name: cluster-info
  namespace: kube-public
```
**Note:** Do not include `certificate-authority-data` in the kubeconfig if the hub is running on a ROSA-HCP cluster.
- create the configmap on the hub cluster.
```shell
kubectl apply -f cm.yaml
```

## Create the CAPI cluster and the managedCluster

The name of the managedcluster MUST be the same as CAPI cluster's name and namespace. e.g.

```yaml
kind: Cluster
metadata:
  name: capi-rosa-cluster
  namespace: capi-rosa-cluster
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
---
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: capi-rosa-cluster
spec:
  hubAcceptsClient: true
```
The managedCluster will be auto-imported when the CAPI cluster is provisioned.
