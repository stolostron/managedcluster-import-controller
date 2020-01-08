# Importing an Hive provisioned OpenShift cluster

## Prereq

### Creating a Hive ClusterDeployment

- For more information about Hive ClusterDeployment refer to `https://github.com/openshift/hive`

### Creating a MultiCloud EndpointConfig for the cluster you are importing

- Example of EndpointConfig resource refer to test/resources/test_endpoint_config.yaml
- Refer to apis/multicloud/v1alpha1/endpointconfig_types.go and apis/multicloud/v1beta1/endpoint_types.go for API definition

## ClusterController action

### ClusterDeployment Controller

- When the first ClusterDeployment is created the controller will create an SelectorSyncSet to deploy the common parts of the multicluster-endpoint install manifest. This inclues the namespace, service account and cluster role binding that is needed by the multicluster-endpoint operator. The SelectorSyncSet will apply these configuration to all the Hive provisioned clusters.

- When ClusterDeployment is created the controller will create the ClusterRegistry Cluster that corresponds to the Hive ClusterDeployment.

- When the EndpointConfig is created the controller will generate the SyncSet for the import manifest which contain all of the resources needed to install multicluster-endpoint.

### (external) Hive SyncSet Controller

- Once the cluster provision is completed Hive will apply the resources defined in the SelectorSyncSet and SyncSet to install the multicluster-endpoint.
