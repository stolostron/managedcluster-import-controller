<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [multicloud-operators-cluster-controller](#multicloud-operators-cluster-controller)
    - [What is the multicloud-operators-cluster-controller](#what-is-the-multicloud-operators-cluster-controller)
    - [How to's](#how-tos)
    - [Community, discussion, contribution, and support](#community-discussion-contribution-and-support)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# multicloud-operators-cluster-controller

## What is the multicloud-operators-cluster-controller

`multicloud-operators-cluster-controller` is the controller that handles functionalities that's related to the clusterregistry cluster resource.

current functionality of `multicloud-operators-cluster-controller`
- installing multicluster-endpoint on cluster created by hive via syncset
- triggering the remote deletion of multicluster-endpoint on managed cluster

## How to's

[Auto importing of an existing cluster](docs/cluster_auto_import.md)

[Manual importing of an existing cluster](docs/cluster_manual_import.md)

[Auto importing of a ClusterAPI provisioned cluster](docs/clusterapi_cluster_import.md)

[Detatching a managed cluster from Multicloud Manager](docs/detatch_managed_cluster.md)

[Importing an Hive provisioned OpenShift cluster](docs/hive_cluster_import.md)

[Updating Endpoint on a managed cluster](docs/remote_endpoint_update.md)

[Selective initilization of controllers](docs/selective_controller_init.md)

[Run functional test](docs/functional_test.md)

## Community, discussion, contribution, and support

Check the [DEVELOPMENT Doc](docs/development.md) for how to build and make changes.

Check the [CONTRIBUTING Doc](CONTRIBUTING.md) for how to contribute to the repo.

You can reach the maintainers of this by raising issues. Slack communication is coming soon
