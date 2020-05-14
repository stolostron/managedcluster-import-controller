# Updating Klusterlet on a managed cluster

## User action

### Update KlusterletConfig on the multicluster-hub

- `kubectl edit klusterletconfig -n <cluster-namespace> <cluster-name>`

## ClusterController actions

### KlusterletConfig Controller

- KlusterletConfig update triggers `Reconcile()` in `pkg/controllers/klusterletconfig/klusterletconfig_controller.go`.
- If the cluster is online controller will create a resourceview to fetch the Klusterlet from managed cluster.
  If the klusterlet is not same as klusterletconfig, controller will create a work to update the Klusterlet on managed cluster.
