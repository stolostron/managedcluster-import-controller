#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -o errexit
set -o nounset

# Input: KUBECTL(kubectl or oc), OCM_VERSION)

KUBECTL=${KUBECTL:-kubectl}
OCM_VERSION=${OCM_VERSION:-main}

function wait_deployment() {
  set +e
  for((i=0;i<30;i++));
  do
    ${KUBECTL} -n $1 get deploy $2
    if [ 0 -eq $? ]; then
      break
    fi
    echo "sleep 1 second to wait deployment $1/$2 to exist: $i"
    sleep 1
  done
  set -e
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

wait_deployment open-cluster-management cluster-manager
${KUBECTL} -n open-cluster-management rollout status deploy cluster-manager --timeout=120s

wait_deployment open-cluster-management-hub cluster-manager-registration-controller
${KUBECTL} -n open-cluster-management-hub rollout status deploy cluster-manager-registration-controller --timeout=120s
${KUBECTL} -n open-cluster-management-hub rollout status deploy cluster-manager-registration-webhook --timeout=120s
${KUBECTL} -n open-cluster-management-hub rollout status deploy cluster-manager-work-webhook --timeout=120s

# scale replicas to save resources, after the hub are installed, we don't need
# the cluster-manager and placement-controller for the e2e test
${KUBECTL} -n open-cluster-management scale --replicas=0 deployment/cluster-manager
${KUBECTL} -n open-cluster-management-hub scale --replicas=0 deployment/cluster-manager-placement-controller
