[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Import a cluster with klusterlet in Detached mode

We can import a cluster with klusterlet in [Detached mode](https://github.com/open-cluster-management-io/registration-operator/tree/v0.6.0#deploy-spokeklusterlet-with-detached-mode), which means the Klusterlet(registration-agent, work-agent) will be deployed outside of the managed cluster but all addons' agents will remain in the managed cluster, and we define the cluster where the Klusterlet runs as management-cluster.

## Prerequisites

A management cluster that will be used to deploy the Klusterlet(registration-agent, work-agent), and has already been imported to the hub cluster, and will not be detached from the hub before ALL Detached mode managed clusters are detached from the hub.

## Import a managed cluster

1. create a ManagedCluster on the hub with 2 annotations
    ```
    ╰─$ oc apply -f - <<EOF
    apiVersion: cluster.open-cluster-management.io/v1
    kind: ManagedCluster
    metadata:
      name: cluster1
      annotations:
        import.open-cluster-management.io/klusterlet-deploy-mode: Detached
        import.open-cluster-management.io/management-cluster-name: local-cluster 
    spec:
      hubAcceptsClient: true
    EOF
    ```

2. create the auto-import-secret which contains a key `kubeconfig` and value managed cluster's kubeconfig in the <managed-cluster-name> namespace

    ```
    oc create secret generic auto-import-secret --from-file=kubeconfig=./managedClusterKubeconfig -n cluster1
    ```

3. check the managed cluster status: `oc get managedcluster cluster1`

4. install addons(optional)
    ```
    ╰─$ oc apply -f - <<EOF
    apiVersion: agent.open-cluster-management.io/v1
    kind: KlusterletAddonConfig
    metadata:
      name: cluster1
      namespace: cluster1
    spec:
      applicationManager:
        enabled: true
      certPolicyController:
        enabled: true
      clusterLabels:
        cloud: auto-detect
        name: cluster1
        vendor: auto-detect
      clusterName: cluster1
      clusterNamespace: cluster1
      iamPolicyController:
        enabled: true
      policyController:
        enabled: true
      proxyConfig: {}
      searchCollector:
        enabled: true
      version: 2.5.0
    EOF
    ```

## Detach the hosted cluster from the hub cluster.
    ```
    oc delete managedcluster cluster1
    ```
