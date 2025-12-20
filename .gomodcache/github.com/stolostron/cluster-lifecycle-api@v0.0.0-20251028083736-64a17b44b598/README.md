# Cluster Lifecycle API

The `cluster-lifecycle-api` repository defines relevant concepts and types for Cluster Lifecycle APIs used in MCE and ACM.

Some APIs are moved from the foundation repository https://github.com/stolostron/multicloud-operators-foundation.

## APIs

* `ManagedClusterAction` is defined as a certain action job executed on a certain managed cluster to Create/Update/Delete a resource.
* `ManagedClusterView` is defined to get a specified resource on a certain managed cluster.
* `ManagedClusterImageRegistry` is defined as a configuration to override the images of pods deployed on the managed clusters.
* `ManagedClusterInfo` is the namespace-scoped definition of managedCluster, including some special infos for MCE and ACM.
* `KluterletConfig` is defined to hold the configuration of klusterlet.

## How to update the APIs

1. Folk the repository the dir `$GOPATH/src/github.com/stolostron/cluster-lifecycle-api` .
2. Run `make update` .

----

## Community, discussion, contribution, and support

Check the [CONTRIBUTING Doc](CONTRIBUTING.md) for how to contribute to the repo.

## Security Response

If you've found a security issue that you'd like to disclose confidentially please contact
Red Hat's Product Security team. Details at [here](https://access.redhat.com/security/team/contact).
