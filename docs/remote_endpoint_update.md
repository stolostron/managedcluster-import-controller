# Updating Endpoint on a managed cluster

## User action

### Update EndpointConfig on the multicluster-hub

- `kubectl edit endpointconfig -n <cluster-namespace> <cluster-name>`

## ClusterController actions

### EndpointConfig Controller

- EndpointConfig update triggers `Reconcile()` in `pkg/controllers/endpointconfig/endpointconfig_controller.go`.
- If the cluster is online controller will create a resourceview to fetch the Endpoint from managed cluster.
  If the endpoint is not same as endpointconfig, controller will create a work to update the Endpoint on managed cluster.
