# Detatching a managed cluster from Multicloud Manager

## User action

### Remove ClusterRegistry Cluster for the managed cluster

- `kubectl delete clusterregistry.k8s.io <cluster-name> -n <cluster-namespace>`

## ClusterController action

### ClusterRegistry Cluster Controller

- ClusterRegistry Cluster deletion triggers `Reconcile()` in `pkg/controllers/clusterregistry/cluster_controller.go`.
- If the cluster is Online the controller will create a Work that creates a Job on the managed cluster to uninstall multicluster-endpoint.
- Once the cluster is Offline the finalizer will be removed from the ClusterRegistry Cluster and the Cluster will be deleted.
