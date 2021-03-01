[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Updating Klusterlet-addons on a managed cluster

## User action

### Update KlusterletAddonConfig on the multicluster-hub

- `kubectl edit klusterletaddonconfig -n <cluster-namespace> <cluster-name>`


### KlusterletAddonConfig Controller

- KlusterletAddonConfig update triggers `Reconcile()` in klusterlet-addon-controller.
- Corresponding addon will get enabled/disabled on based on what user has selected action