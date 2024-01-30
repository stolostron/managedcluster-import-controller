#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

KLUSTERLET_CRD_V1_FILE="./vendor/open-cluster-management.io/api/operator/v1/0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml"
KLUSTERLET_CRD_V1BETA1_FILE="./vendor/open-cluster-management.io/api/crdsv1beta1/0001_00_operator.open-cluster-management.io_klusterlets.crd.yaml"

KLUSTERLET_CRD_V1_DEST_FILE="./pkg/bootstrap/manifests/klusterlet/crds/klusterlets.crd.v1.yaml"
KLUSTERLET_CRD_V1BETA1_DEST_FILE="./pkg/bootstrap/manifests/klusterlet/crds/klusterlets.crd.v1beta1.yaml"

cp $KLUSTERLET_CRD_V1_FILE $KLUSTERLET_CRD_V1_DEST_FILE

cp $KLUSTERLET_CRD_V1BETA1_FILE $KLUSTERLET_CRD_V1BETA1_DEST_FILE
# Remove lines containing "default: "
# The upstream v1beta1 klusterlet CRD is invalid, remove the v1beta1 crd lines containing "default: " in MCE
# until the upstream has a fix.
# See: https://github.com/open-cluster-management-io/api/issues/192
sed -i '/default: /d' $KLUSTERLET_CRD_V1BETA1_DEST_FILE
