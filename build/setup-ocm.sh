#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -o errexit
set -o nounset

# Input: KUBECTL(kubectl or oc), OCM_VERSION)

KUBECTL=${KUBECTL:-kubectl}
OCM_VERSION=backplane-2.6

function debug_and_exit() {
  echo "::group::####### DIAGNOSTIC: OCM setup failure #######"
  echo "=== Pods in open-cluster-management ==="
  ${KUBECTL} -n open-cluster-management get pods -o wide --ignore-not-found
  echo "=== Pod details in open-cluster-management ==="
  ${KUBECTL} -n open-cluster-management describe pods || true
  echo "=== Pods in open-cluster-management-hub ==="
  ${KUBECTL} -n open-cluster-management-hub get pods -o wide --ignore-not-found
  echo "=== Cluster Manager operator logs ==="
  ${KUBECTL} -n open-cluster-management logs -l app=cluster-manager --tail=200 || true
  echo "=== ClusterManager CR status ==="
  ${KUBECTL} get clustermanagers -o yaml --ignore-not-found
  echo "::endgroup::"
  exit 1
}

BUILD_DIR="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
REPO_DIR="$(dirname "$BUILD_DIR")"
WORK_DIR="${REPO_DIR}/_output"

mkdir -p "${WORK_DIR}"

echo "###### deploy ocm"
rm -rf "$WORK_DIR/_repo_ocm"

export OCM_BRANCH=$OCM_VERSION
export IMAGE_NAME=quay.io/stolostron/registration-operator:$OCM_VERSION
export OPERATOR_IMAGE_NAME=quay.io/stolostron/registration-operator:$OCM_VERSION
export REGISTRATION_OPERATOR_IMAGE=quay.io/stolostron/registration-operator:$OCM_VERSION
export REGISTRATION_IMAGE=quay.io/stolostron/registration:$OCM_VERSION
export WORK_IMAGE=quay.io/stolostron/work:$OCM_VERSION
export PLACEMENT_IMAGE=quay.io/stolostron/placement:$OCM_VERSION
export ADDON_MANAGER_IMAGE=quay.io/stolostron/addon-manager:$OCM_VERSION

git clone --depth 1 --branch $OCM_BRANCH https://github.com/stolostron/ocm.git "$WORK_DIR/_repo_ocm"
make deploy-hub-operator apply-hub-cr -C "$WORK_DIR/_repo_ocm"

rm -rf "$WORK_DIR/_repo_ocm"

${KUBECTL} wait -n open-cluster-management-hub --for=create deployment/cluster-manager-registration-controller --timeout=60s || debug_and_exit
${KUBECTL} -n open-cluster-management-hub rollout status deployment/cluster-manager-registration-controller --timeout=120s || debug_and_exit
${KUBECTL} -n open-cluster-management-hub rollout status deployment/cluster-manager-registration-webhook --timeout=120s || debug_and_exit
${KUBECTL} -n open-cluster-management-hub rollout status deployment/cluster-manager-work-webhook --timeout=120s || debug_and_exit

# scale replicas to save resources, after the hub are installed, we don't need
# the cluster-manager and placement-controller for the e2e test
${KUBECTL} -n open-cluster-management scale --replicas=0 deployment/cluster-manager
${KUBECTL} -n open-cluster-management-hub scale --replicas=0 deployment/cluster-manager-placement-controller
