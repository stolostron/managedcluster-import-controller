# Updating Endpoint on a managed cluster

## User action

### Update EndpointConfig on the multicluster-hub

- `kubectl edit endpointconfig -n <cluster-namespace> <cluster-name>`

## ClusterController actions

### EndpointConfig Controller

- EndpointConfig update triggers `Reconcile()` in `pkg/controllers/endpointconfig/endpointconfig_controller.go`.
- If the cluster is Online the controller will create a Work to update the Endpoint on managed cluster.

