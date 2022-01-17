[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Importing an Hive provisioned OpenShift cluster


## Prereq

### Creating a Hive ClusterDeployment

- For more information about Hive ClusterDeployment refer to `https://github.com/openshift/hive`


## ManagedClusterController action

### ManagedCluster Import Controller

- When the first ClusterDeployment is created along with managedcluster and klusterletaddonconfig, the controller will call the applier to deploy the common parts of the klusterlet install manifest. This includes the namespace, service account and cluster role binding that is needed by the klusterlet operator.

- When managedcluster is created, the controller will create klusterlet on the managedcluster. 

### Kusterlet addon Controller

- When klusterletaddonconfig is created, klusterlet-addon-controller will create klusterlet addon on the corresponding Hive ClusterDeployment.

