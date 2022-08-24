#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

# set -e
# set -x

CURR_FOLDER_PATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
KIND_KUBECONFIG="${CURR_FOLDER_PATH}/../kind_kubeconfig.yaml"
KIND_KUBECONFIG_INTERNAL="${CURR_FOLDER_PATH}/../kind_kubeconfig_internal.yaml"
KIND_MANAGED_KUBECONFIG="${CURR_FOLDER_PATH}/../kind_kubeconfig_mc.yaml"
KIND_MANAGED_KUBECONFIG_INTERNAL="${CURR_FOLDER_PATH}/../kind_kubeconfig_internal_mc.yaml"
CLUSTER_NAME=$PROJECT_NAME-functional-test

export KUBECONFIG=${KIND_KUBECONFIG}
export DOCKER_IMAGE_AND_TAG=${1}

export FUNCT_TEST_TMPDIR="${CURR_FOLDER_PATH}/../test/functional/tmp"
export FUNCT_TEST_COVERAGE="${CURR_FOLDER_PATH}/../test/functional/coverage"

KIND="$(pwd)"/kind-linux-amd64
KUBECTL="$(pwd)"/kubectl

echo "installing kubectl"
curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl && chmod +x kubectl

echo "installing kind"
curl -LO https://github.com/kubernetes-sigs/kind/releases/download/v0.11.1/kind-linux-amd64 && chmod +x kind-linux-amd64

echo "installing ginkgo"
go install github.com/onsi/ginkgo/ginkgo

echo "setting up test tmp folder"
[ -d "$FUNCT_TEST_TMPDIR" ] && rm -r "$FUNCT_TEST_TMPDIR"
mkdir -p "$FUNCT_TEST_TMPDIR"
# mkdir -p "$FUNCT_TEST_TMPDIR/output"
# mkdir -p "$FUNCT_TEST_TMPDIR/kind-config"
mkdir -p "$FUNCT_TEST_TMPDIR/CR"

echo "setting up test coverage folder"
[ -d "$FUNCT_TEST_COVERAGE" ] && rm -r "$FUNCT_TEST_COVERAGE"
mkdir -p "${FUNCT_TEST_COVERAGE}"


#not used as we need to find a way to use token with kind.
#export MANAGED_CLUSTER_API_SERVER_URL=$(cat ${KIND_MANAGED_KUBECONFIG_INTERNAL}| grep "server:" |cut -d ":" -f2 -f3 -f4 | sed 's/^ //')
#export MANAGED_CLUSTER_TOKEN="itdove.thisisafaketoken"

cat << EOF > "${FUNCT_TEST_TMPDIR}/CR/fake_infrastructure_cr.yaml"
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
spec:
  cloudConfig:
    name: ""
status:
  apiServerInternalURI: API_SERVER_URL
  apiServerURL: API_SERVER_URL
EOF

echo "creating managed cluster"
${KIND} create cluster --name ${CLUSTER_NAME}-managed
# setup kubeconfig
${KIND} get kubeconfig --name ${CLUSTER_NAME}-managed > ${KIND_MANAGED_KUBECONFIG}
${KIND} get kubeconfig --name ${CLUSTER_NAME}-managed --internal > ${KIND_MANAGED_KUBECONFIG_INTERNAL}
echo "creating hub cluster"
${KIND} create cluster --name ${CLUSTER_NAME}

# setup kubeconfig
${KIND} get kubeconfig --name ${CLUSTER_NAME} > ${KIND_KUBECONFIG}
${KIND} get kubeconfig --name ${CLUSTER_NAME} --internal > ${KIND_KUBECONFIG_INTERNAL}
API_SERVER_URL=$(cat ${KIND_KUBECONFIG_INTERNAL}| grep "server:" | awk '{print $2}')

# load image if possible
${KIND} load docker-image ${DOCKER_IMAGE_AND_TAG} --name=${CLUSTER_NAME} -v 99 || echo "failed to load image locally, will use imagePullSecret"

echo "prepare required crds"
# setup cluster
make kind-cluster-setup

for dir in overlays/test/* ; do
  echo ">>>>>>>>>>>>>>>Executing test: $dir"

  # install rcm-controller
  echo "install managedcluster-import-controller"
  ${KUBECTL} apply -k "$dir" --dry-run=true -o yaml | sed "s|REPLACE_IMAGE|${DOCKER_IMAGE_AND_TAG}|g" | ${KUBECTL} apply -f -

  echo "Create the cluster infrastructure"
  sed "s|API_SERVER_URL|${API_SERVER_URL}|g" ${FUNCT_TEST_TMPDIR}/CR/fake_infrastructure_cr.yaml | ${KUBECTL} apply -f -

  # patch image
  echo "Wait rollout"
  ${KUBECTL} rollout status -n open-cluster-management deployment managedcluster-import-controller --timeout=600s

  echo "run functional test..."
  make functional-test
  # exit 1
  echo "remove deployment"
  ${KUBECTL} delete --wait=true -k "$dir"
done;

echo "Wait 10 sec for copy to AWS"
sleep 10

echo "delete clusters"
kind delete cluster --name ${CLUSTER_NAME}
kind delete cluster --name ${CLUSTER_NAME}-managed
