[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Auto import a managed cluster

The hub is automatically importing a managed cluster when a secret called `auto-import-secret` is placed in a namespace named as the cluster name. The namespace will also needs to contain the managedcluster and the kubeaddonconfig CR. The `auto-import-secret` will be automatically deleted when the import is completed (sucessfully on not).


## Creating a namespace in which the cluster will get imported
On the Hub cluster:
- Create a namespace
  ```shell
  kubectl create ns <cluster_name>
  ```
  Namespace name should be same as cluster name

## Creating the auto-import-secret.
On the hub cluster, create a secret containing the kubeconfig or the pair (server/token) of the managed cluster. 

- Create the auto-import-secret with kubeconfig:
``` yaml
apiVersion: v1
kind: Secret
metadata:
  name: auto-import-secret
  namespace: <cluster_name>
stringData:
  autoImportRetry: "<autoImportRetry>"
  kubeconfig: |- 
    <kubeconfig>
type: Opaque
```

- Create the auto-import-secret with token/server:
``` yaml
apiVersion: v1
kind: Secret
metadata:
  name: auto-import-secret
  namespace: <cluster_name>
stringData:
  autoImportRetry: "<autoImportRetry>"
  token: <token>
  server: <api_server_url>
type: Opaque
```

The autoImportRetry is the number of time the operator will retry to use that secret to import the managed cluster. 0 retry means try ones. If the import failed a condition "ManagedClusterImportSucceeded" in the managedcluster CR will be set to "False" along with a reason and message.

## Creating a Managed Cluster
On the Hub Cluster: 
- Create a ManagedCluster CR:

```yaml
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: <cluster_name>
  local-cluster: "true"
spec:
  hubAcceptsClient: true
```

Setting the `label-cluster` to `"true"` will tell the MangedCluster controller to start the import of the hub as a managed cluster.

## Creating a klusterlet addons on the managed cluster

On the Hub Cluster: 
- Create a KlusterletAddonConfig CR:

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

- Controller will generate a secret named `<cluster_name>-import`.
- The `<cluster_name>-import` secret contains the crds.yaml and import.yaml that the user will apply on managed cluster to install klusterlet.
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
kubectl get managedclusters $<cluster_name> -o yaml
```

- Status should indicate ManagedClusterJoined and ManagedClusterAvailable and Status: "True" for successful import. The "ManagedClusterImportSucceeded" status only applies to the deployment of the agent to the managed cluster but doesn't mean that the agent is able to communicate with the hub.

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
  - lastTransitionTime: "2020-06-23T17:14:10Z"
    message: Import succeeded
    reason: ManagedClusterImported
    status: "True"
    type: ManagedClusterImportSucceeded

```

## Klusterlet addon controller

On the Hub Cluster: 
- klusterletaddonconfig creation triggers `Reconcile()` in klusterlet addon controller

- Controller will create manifestworks for addons in the <cluster_name> namespace

On the managed cluster:
- Check the addons installed in the namespace `open-cluster-management-agent-addon`

```
kubectl get pods -n open-cluster-management-agent-addon
```
