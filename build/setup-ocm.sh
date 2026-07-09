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
  for((i=0;i<60;i++));
  do
    ${KUBECTL} -n $1 get deploy $2
    if [ 0 -eq $? ]; then
      break
    fi
    echo "sleep 1 second to wait deployment $1/$2 to exist: $i"
    sleep 1
  done
  set -e

  if ! ${KUBECTL} -n $1 get deploy $2 &>/dev/null; then
    echo "####### DIAGNOSTIC: deployment $1/$2 not found after 60s #######"
    echo "=== Pods in namespace $1 ==="
    ${KUBECTL} -n $1 get pods -o wide || true
    echo "=== Pod details in namespace $1 ==="
    ${KUBECTL} -n $1 describe pods || true
    echo "=== Cluster Manager operator logs ==="
    ${KUBECTL} -n open-cluster-management logs -l app=cluster-manager --tail=200 || true
    echo "=== ClusterManager CR status ==="
    ${KUBECTL} get clustermanagers -o yaml || true
    echo "####### END DIAGNOSTIC #######"
    exit 1
  fi
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
rm -rf ${OCM_CHART_DIR}
git clone --depth 1 --branch $OCM_VERSION --filter=blob:none --sparse \
  https://github.com/stolostron/ocm.git ${OCM_CHART_DIR}
cd ${OCM_CHART_DIR}
git sparse-checkout set deploy/cluster-manager/chart
cd ${REPO_DIR}

${HELM} upgrade --install cluster-manager ${OCM_CHART_DIR}/deploy/cluster-manager/chart/cluster-manager \
--namespace=open-cluster-management --create-namespace --set replicaCount=1,images.registry="quay.io/stolostron",images.tag=$OCM_VERSION


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
