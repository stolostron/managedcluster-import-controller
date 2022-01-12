[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Detatching a managed cluster

## User action

### Remove ManagedCluster to detach a cluster

- `kubectl delete managedcluster <cluster-name>`

#### If cluster is offline

Deleting an offline (not Available) ManagedCluster is allowed, and it removes all resources on hub without removing anything on the managed cluster. To completely cleanup the managed cluster, user can run the [self-destruct.sh](https://github.com/stolostron/klusterlet-addon-controller/blob/main/hack/self-destruct.sh) script on managedcluster.

## ManagedCluster Import Controller action

###  ManagedCluster Import Controller

- ManagedCluster deletion triggers `Reconcile()` in [/pkg/controller/managedcluster/managedcluster_controller.go](https://github.com/stolostron/managedcluster-import-controller/blob/master/pkg/controller/managedcluster/managedcluster_controller.go).
- If the managed cluster is online the controller will wait for klusterlet-addon-controller to remove all addon manifestworks first, and then delete the manifestwork of klusterlet.
- Once the managed cluster is Offline the finalizer will be removed from the ManagedCluster. Then, the ManagedCluster and cluster namespace will be deleted.
