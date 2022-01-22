#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

KLUSTERLET_CRD_V1_FILE="./vendor/open-cluster-management.io/api/operator/v1/0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml"
KLUSTERLET_CRD_V1BETA1_FILE="./vendor/open-cluster-management.io/api/operator/v1/0001_00_operator.open-cluster-management.io_klusterlets.crd.yaml"

cp $KLUSTERLET_CRD_V1_FILE ./pkg/controller/importconfig/manifests/klusterlet/crds/klusterlets.crd.v1.yaml
cp $KLUSTERLET_CRD_V1BETA1_FILE ./pkg/controller/importconfig/manifests/klusterlet/crds/klusterlets.crd.v1beta1.yaml
