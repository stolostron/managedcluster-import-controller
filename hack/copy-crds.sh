#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

KLUSTERLET_CRD_V1_FILE="./vendor/open-cluster-management.io/api/operator/v1/0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml"
KLUSTERLET_CRD_V1BETA1_FILE="./vendor/open-cluster-management.io/api/operator/v1/0001_00_operator.open-cluster-management.io_klusterlets.crd.yaml"

cp $KLUSTERLET_CRD_V1_FILE ./pkg/controller/importconfig/manifests/klusterlet/crds/klusterlets.crd.v1.yaml

# The upstream v1beta1 klusterlet CRD is invalid, and we do not use the registraionConfiguration/workConfigration
# feature in MCE, so stop update this CRD until the upstream has a fix.
# See: https://github.com/open-cluster-management-io/api/issues/192
#
# cp $KLUSTERLET_CRD_V1BETA1_FILE ./pkg/controller/importconfig/manifests/klusterlet/crds/klusterlets.crd.v1beta1.yaml
