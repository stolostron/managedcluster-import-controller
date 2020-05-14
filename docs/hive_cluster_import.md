# Importing an Hive provisioned OpenShift cluster

## Prereq

### Creating a Hive ClusterDeployment

- For more information about Hive ClusterDeployment refer to `https://github.com/openshift/hive`

### Creating a MultiCloud KlusterletConfig for the cluster you are importing

- Example of KlusterletConfig resource refer to test/resources/test_klusterlet_config.yaml
- Refer to apis/agent/v1beta1/klusterletconfig_types.go and apis/multicloud/v1beta1/endpoint_types.go for API definition

## ClusterController action

### ClusterDeployment Controller

- When the first ClusterDeployment is created the controller will create an SelectorSyncSet to deploy the common parts of the klusterlet install manifest. This inclues the namespace, service account and cluster role binding that is needed by the klusterlet operator. The SelectorSyncSet will apply these configuration to all the Hive provisioned clusters.

- When ClusterDeployment is created the controller will create the ClusterRegistry Cluster that corresponds to the Hive ClusterDeployment.

- When the KlusterletConfig is created the controller will generate the SyncSet for the import manifest which contain all of the resources needed to install klusterlet.

### (external) Hive SyncSet Controller

- Once the cluster provision is completed Hive will apply the resources defined in the SelectorSyncSet and SyncSet to install the klusterlet.
