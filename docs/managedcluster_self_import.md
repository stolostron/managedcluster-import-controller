# Self importing the hub

The hub is automatically self imported at startup as the rcm-chart creates the namespace, managedcluster and klusterletaddonconfig resources. Follow the steps below to re-import it if needed.

## Creating a namespace in which the cluster will get imported
On the Hub cluster:
- Create a namespace
  ```shell
  kubectl create ns {cluster_name}
  ```
  Namespace name should be same as cluster name

## Creating a Managed Cluster
On the Hub Cluster: 
- Create a ManagedCluster CR:

```
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: {cluster_name}
  local-cluster: "true"
spec:
  hubAcceptsClient: true
```

Setting the `label-cluster` to `"true"` will tell the MangedCluster controller to start the import of the hub as a managed cluster.

## Creating a klusterlet addons on the managed cluster

On the Hub Cluster: 
- Create a KlusterletAddonConfig CR:

```
apiVersion: agent.open-cluster-management.io/v1
kind: KlusterletAddonConfig
metadata:
  name: {cluster_name}
  namespace: {cluster_name}
spec:
  clusterName: {cluster_name}
  clusterNamespace: {cluster_name}
  applicationManager:
    enabled: true
  clusterLabels:
    cloud: auto-detect
    vendor: auto-detect
  policyController:
    enabled: true
  searchCollector:
    enabled: true
  certPolicyController:
    enabled: true
  iamPolicyController:
    enabled: true
  version: 2.2.0
```

## ManagedCluster controller

- ManagedCluster creation triggers `Reconcile()` in [/pkg/controller/managedcluster/managedcluster_controller.go](https://github.com/open-cluster-management/rcm-controller/blob/master/pkg/controller/managedcluster/managedcluster_controller.go).
- Controller will generate a secret named `{cluster_name}-import`.
- The `{cluster_name}-import` secret contains the crds.yaml and import.yaml that the user will apply on managed cluster to install klusterlet.
- The controller will apply the crds.yaml and import.yaml.

Validation:
- check the pod status on the managed cluster: `kubectl get pod -n open-cluster-management-agent`


## CSR will get automatically approved on Hub cluster

Once all the pod running on the managed cluster in namespace `open-cluster-management-agent`

It will create a csr on the hub with the managed cluster name as prefix.

- To check the if csr is created on the hub 

```
kubectl get csr
```
Example:  csr will be in pending state

```
kubectl get csr
NAME          AGE   REQUESTOR                                        CONDITION
test1-lpxcj   8s   system:serviceaccount:test1:test1-bootstrap-sa   Pending
```

- csr will get automatically approved

```
oc get csr --all-namespaces
NAME          AGE   REQUESTOR                                        CONDITION
test1-lpxcj   12s   system:serviceaccount:test1:test1-bootstrap-sa   Approved,Issued
```

- Once the csr is approved, check the managed cluster status

```
kubectl get managedclusters ${cluster_name} -o yaml
```

- Status should indicate ManagedClusterJoined and ManagedClusterAvailable and Status: "True" for successful import 

```
  - lastTransitionTime: "2020-06-23T17:14:15Z"
    message: Accepted by hub cluster admin
    reason: HubClusterAdminAccepted
    status: "True"
    type: HubAcceptedManagedCluster
  - lastTransitionTime: "2020-06-23T17:19:32Z"
    message: Managed cluster is available
    reason: ManagedClusterAvailable
    status: "True"
    type: ManagedClusterConditionAvailable
  - lastTransitionTime: "2020-06-23T17:19:32Z"
    message: Managed cluster joined
    reason: ManagedClusterJoined
    status: "True"
    type: ManagedClusterJoined
```

## Klusterlet addon controller

On the Hub Cluster: 
- klusterletaddonconfig creation triggers `Reconcile()` in klusterlet addon controller

- Controller will create manifestworks for addons in the {cluster_name} namespace

On the managed cluster:
- Check the addons installed in the namespace `open-cluster-management-agent-addon`

```
kubectl get pods -n open-cluster-management-agent-addon
```