#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

# Input: KUBECTL(kubectl or oc)
# Arguments:
#   $1: with-managed|skip-managed (default: skip-managed) - whether to setup managed cluster for hosted mode testing
# Output: E2E_KUBECONFIG(WORK_DIR/_output/e2e-kubeconfig), E2E_MANAGED_KUBECONFIG(WORK_DIR/_output/e2e-managed-kubeconfig), cluster_ip, cluster_context
# Dependency: managedcluster-import-controller image has been built(load to kind cluster)

set -o errexit
set -o nounset

KUBECTL=${KUBECTL:-kubectl}

CLUSTER_NAME="e2e-test-cluster"
CLUSTER_NAME_MANAGED="e2e-test-cluster-managed"

BUILD_DIR="$(
  cd "$(dirname "$0")" >/dev/null 2>&1
  pwd -P
)"
REPO_DIR="$(dirname "$BUILD_DIR")"
WORK_DIR="${REPO_DIR}/_output"

E2E_KUBECONFIG="${WORK_DIR}/e2e-kubeconfig"
E2E_MANAGED_KUBECONFIG="${WORK_DIR}/e2e-managed-kubeconfig"
E2E_EXTERNAL_MANAGED_KUBECONFIG="${WORK_DIR}/e2e-external-managed-kubeconfig"

mkdir -p "${WORK_DIR}/bin"

KIND_VERSION="v0.17.0"
KIND="${WORK_DIR}/bin/kind"
KUBE_VERSION="v1.29.0"

SETUP_MANAGED_CLUSTER=${1:-skip-managed}

echo "###### installing kind"
curl -s -f -L "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-${GOHOSTOS}-${GOHOSTARCH}" -o "${KIND}"
chmod +x "${KIND}"

echo "###### installing e2e test cluster"
${KIND} delete cluster --name ${CLUSTER_NAME}
${KIND} create cluster --image kindest/node:${KUBE_VERSION} --name ${CLUSTER_NAME}

cluster_ip=$(${KUBECTL} get svc kubernetes -n default -o jsonpath="{.spec.clusterIP}")
cluster_context=$(${KUBECTL} config current-context)

echo "###### scaling down system components to save resources"
# scale replicas to 1 to save resources
${KUBECTL} --context="${cluster_context}" -n kube-system scale --replicas=1 deployment/coredns
# scale down other system components if they exist
${KUBECTL} --context="${cluster_context}" -n kube-system scale --replicas=1 deployment/kindnet || true
${KUBECTL} --context="${cluster_context}" -n kube-system scale --replicas=1 daemonset/kindnet || true

echo "###### loading image"
${KIND} load docker-image managedcluster-import-controller --name ${CLUSTER_NAME}

echo "###### prepare required crds"
${KUBECTL} apply -f "$REPO_DIR/test/e2e/resources/hive"
${KUBECTL} apply -f "$REPO_DIR/test/e2e/resources/ocp"
${KUBECTL} apply -f "$REPO_DIR/test/e2e/resources/assisted-service"

cat <<EOF | ${KUBECTL} apply -f -
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
spec:
  cloudConfig:
    name: ""
status:
  apiServerInternalURI: https://${cluster_ip}
  apiServerURL: https://${cluster_ip}
EOF

# store main cluster kubeconfig
${KIND} get kubeconfig --name=${CLUSTER_NAME} --internal >$E2E_KUBECONFIG

# prepare another managed cluster for hosted mode testing (optional)
if [ "$SETUP_MANAGED_CLUSTER"x = "with-managed"x ]; then
  echo "###### installing e2e test managed cluster"
  ${KIND} delete cluster --name ${CLUSTER_NAME_MANAGED}
  ${KIND} create cluster --image kindest/node:${KUBE_VERSION} --name ${CLUSTER_NAME_MANAGED}
  cluster_context_managed=$(${KUBECTL} config current-context)
  echo "managed cluster context is: ${cluster_context_managed}"

  echo "###### scaling down managed cluster system components to save resources"
  # scale replicas to 1 to save resources
  ${KUBECTL} --context="${cluster_context_managed}" -n kube-system scale --replicas=1 deployment/coredns
  # scale down other system components if they exist
  ${KUBECTL} --context="${cluster_context_managed}" -n kube-system scale --replicas=1 deployment/kindnet || true
  ${KUBECTL} --context="${cluster_context_managed}" -n kube-system scale --replicas=1 daemonset/kindnet || true

  # store managed cluster kubeconfigs
  ${KIND} get kubeconfig --name=${CLUSTER_NAME_MANAGED} --internal >$E2E_MANAGED_KUBECONFIG
  ${KIND} get kubeconfig --name=${CLUSTER_NAME_MANAGED} >$E2E_EXTERNAL_MANAGED_KUBECONFIG
else
  echo "###### skipping managed cluster setup"
fi

# check back to the test cluster
${KUBECTL} config use-context "${cluster_context}"
