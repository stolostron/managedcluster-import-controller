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

echo "###### deploy ocm"

BUILD_DIR="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
REPO_DIR="$(dirname "$BUILD_DIR")"
HELM="${REPO_DIR}/_output/helm"

#
# We are specifically overriding images with stolostron built images, so also use the stolostron repo
# to get the ocm helm charts for cluster-manager, ensuring the chart RBAC and operator image are
# always from the same version.
#
OCM_CHART_DIR="${REPO_DIR}/_output/ocm"
rm -rf "${OCM_CHART_DIR}"
git clone --depth 1 --branch "$OCM_VERSION" --filter=blob:none --sparse \
  https://github.com/stolostron/ocm.git "${OCM_CHART_DIR}"
git -C "${OCM_CHART_DIR}" sparse-checkout set deploy/cluster-manager/chart

${HELM} upgrade --install cluster-manager "${OCM_CHART_DIR}/deploy/cluster-manager/chart/cluster-manager" \
  --namespace=open-cluster-management --create-namespace \
  --set replicaCount=1,images.registry="quay.io/stolostron",images.tag="$OCM_VERSION"

${KUBECTL} wait -n open-cluster-management --for=create deployment/cluster-manager --timeout=60s || debug_and_exit
${KUBECTL} -n open-cluster-management rollout status deployment/cluster-manager --timeout=120s || debug_and_exit

${KUBECTL} wait -n open-cluster-management-hub --for=create deployment/cluster-manager-registration-controller --timeout=60s || debug_and_exit
${KUBECTL} -n open-cluster-management-hub rollout status deployment/cluster-manager-registration-controller --timeout=120s || debug_and_exit
${KUBECTL} -n open-cluster-management-hub rollout status deployment/cluster-manager-registration-webhook --timeout=120s || debug_and_exit
${KUBECTL} -n open-cluster-management-hub rollout status deployment/cluster-manager-work-webhook --timeout=120s || debug_and_exit

# scale replicas to save resources, after the hub are installed, we don't need
# the cluster-manager and placement-controller for the e2e test
${KUBECTL} -n open-cluster-management scale --replicas=0 deployment/cluster-manager
${KUBECTL} -n open-cluster-management-hub scale --replicas=0 deployment/cluster-manager-placement-controller
