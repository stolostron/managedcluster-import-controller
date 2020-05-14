# Auto importing of an existing cluster

## Prerequisite

### Creating a ClusterRegistry Cluster

- Refer to <https://github.com/kubernetes/cluster-registry/blob/master/pkg/apis/clusterregistry/v1alpha1/types.go> for resource definition.

### Creating a Cluster KUBECONFIG Secret

- Cluster config secret must be in the same namespace as ClusterRegistry Cluster.
- The secret `labels` must contain `purpose: import-cluster`.
- The secret `data` must contain the `kubeconfig`.

### Creating a Multicloud KlusterletConfig for the cluster you are importing

- Example of KlusterletConfig resource refer to test/resources/test_klusterlet_config.yaml
- Refer to apis/agent/v1beta1/klusterletconfig_types.go and apis/agent/v1beta1/klusterlet_types.go for resource definition.

## ClusterController action

### ClusterAPI Cluster Controller

- ClusterAPI Cluster creation triggers `Reconcile()` in `pkg/controllers/clusterapi/cluster_controller.go`.
- The controller will create the ClusterRegistry Cluster using the information in ClusterAPI cluster.

### KlusterletConfig Controller

- KlusterletConfig creation triggers `Reconcile()` in `pkg/controllers/klusterletconfig/klusterletconfig_controller.go`.
- Controller will use information in KlusterletConfig to generate a secret named `{cluster-name}-import`.
- The `{cluster-name}-import` secret contains the import.yaml that the will be apply to the managed cluster to install klusterlet.

### AutoImport Controller

- Secret with `purpose: import-cluster` label will trigger the `Reconcile()` in `pkg/controller/autoimport/import_controller.go`.
- The controller will use the import KUBECONFIG secret and the import manifest to run `kubectl apply` on the target cluster to install klusterlet.
