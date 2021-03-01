[comment]: # ( Copyright Contributors to the Open Cluster Management project )

<!--
    Auto-import not supported 
-->
# Auto importing of a ClusterAPI provisioned cluster

## Prereq

### Creating a ClusterAPI Cluster

- For information about how to create clusters swith ClusterAPI `https://www.ibm.com/support/knowledgecenter/SSFC4F_1.2.0/mcm/manage_cluster/create_gui.html`

### Creating a Multicloud KlusterletConfig for the cluster you are importing

- Example of KlusterletConfig resource refer to test/resources/test_klusterlet_config.yaml
- Refer to apis/agent/v1beta1/klusterletconfig_types.go and apis/multicloud/v1beta1/endpoint_types.go for API definition
- `ClusterName` and `ClusterNamespace` of KlusterletConfig must match the ClusterAPI Cluster `Name` and `Namespace`.

## ClusterController actions

### (external) ClusterAPI Controller

- ClusterAPI controller will start the provision process.
- Once the cluster provision process is complete the ClusterAPI controller will generate a secret that contains the KUBECONFIG for the newly provisioned cluster
- The KUBECONFIG secret will contain the label: `purpose: import-cluster`

### KlusterletConfig Controller

- KlusterletConfig creation triggers `Reconcile()` in `pkg/controllers/klusterletconfig/klusterletconfig_controller.go`.
- Controller will use information in KlusterletConfig to generate a secret named `{cluster-name}-import`.
- The `{cluster-name}-import` secret contains the import.yaml that the will be apply to the managed cluster to install klusterlet.

### AutoImport Controller

- Secret with `purpose: import-cluster` label will trigger the `Reconcile()` in `pkg/controller/autoimport/import_controller.go`
- The controller will use the import KUBECONFIG secret and the import manifest to run `kubectl apply` on the target cluster to install klusterlet.
