# Importing an Hive provisioned OpenShift cluster

You can use the scripts available at [applier-samples-for-acm](https://github.com/open-cluster-management/applier-samples-for-acm) to ease the import process.
## Prereq

### Creating a Hive ClusterDeployment

- For more information about Hive ClusterDeployment refer to `https://github.com/openshift/hive`


## ManagedClusterController action

### ManagedCluster Import Controller

- When the first ClusterDeployment is created along with managedcluster and klusterletaddonconfig, the controller will create an SyncSet to deploy the common parts of the klusterlet install manifest. This includes the namespace, service account and cluster role binding that is needed by the klusterlet operator. The SyncSet will apply these configuration to all the Hive provisioned clusters.

- When managedcluster is created, the controller will create klusterlet on the managedcluster. 

### Kusterlet addon Controller

- When klusterletaddonconfig is created, klusterlet-addon-controller will create klusterlet addon on the corresponding Hive ClusterDeployment.


### (external) Hive SyncSet Controller

- Once the cluster provision is completed Hive will apply the resources defined in the SyncSet to install the klusterlet.
