[comment]: # ( Copyright Contributors to the Open Cluster Management project )

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

# managedcluster import controller

ManagedCluster import controller supports import, detach of ManagedCluster for ACM.

## Community, discussion, contribution, and support

Check the [CONTRIBUTING Doc](CONTRIBUTING.md) for how to contribute to the repo.

Check the [DEVELOPMENT Doc](docs/development.md) for how to build and make changes.

Check the [Run functional test](docs/functional_test.md) for how to run functional tests.

------

## Getting Started

This is a guide on how to build and deploy klusterlet addon controller from code.

**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [managedcluster-import-controller](#managedcluster-import-controller)
    - [What is the managedcluster-import-controller](#what-is-the-managedcluster-import-controller)
    - [How to's](#how-tos)
    - [Community, discussion, contribution, and support](#community-discussion-contribution-and-support)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# managedcluster-import-controller

## What is the managedcluster-import-controller

`managedcluster-import-controller` is the controller that handles functionalities that's related to the managedcluster  resource.

current functionality of `managedcluster-import-controller`
- installing klusterlet on cluster created by hive via the applier
- triggering the remote deletion of klusterlet on managed cluster

## How to's

[Manual importing of an existing cluster](docs/managedcluster_manual_import.md)

[Detatching a managed cluster from Multicloud Manager](docs/detatch_managed_cluster.md)

[Importing an Hive provisioned OpenShift cluster](docs/hive_cluster_import.md)

[Updating Klusterlet on a managed cluster](docs/remote_klusterlet_update.md)

[Selective initilization of controllers](docs/selective_controller_init.md)


