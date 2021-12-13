#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -e
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


echo "installing kubectl"
curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/

if ! which kind > /dev/null; then
    echo "installing kind"
    curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.7.0/kind-$(uname)-amd64
    chmod +x ./kind
    sudo mv ./kind /usr/local/bin/kind
fi
if ! which ginkgo > /dev/null; then
    echo "Installing ginkgo ..."
    pushd $(mktemp -d) 
    GOSUMDB=off go get github.com/onsi/ginkgo/ginkgo
    GOSUMDB=off go get github.com/onsi/gomega/...
    popd
fi
if ! which gocovmerge > /dev/null; then
  echo "Installing gocovmerge..."
  pushd $(mktemp -d) 
  GOSUMDB=off go get -u github.com/wadey/gocovmerge
  popd
fi

echo "setting up test tmp folder"
[ -d "$FUNCT_TEST_TMPDIR" ] && rm -r "$FUNCT_TEST_TMPDIR"
mkdir -p "$FUNCT_TEST_TMPDIR"
# mkdir -p "$FUNCT_TEST_TMPDIR/output"
mkdir -p "$FUNCT_TEST_TMPDIR/kind-config"
mkdir -p "$FUNCT_TEST_TMPDIR/CR"

echo "setting up test coverage folder"
[ -d "$FUNCT_TEST_COVERAGE" ] && rm -r "$FUNCT_TEST_COVERAGE"
mkdir -p "${FUNCT_TEST_COVERAGE}"

echo "generating kind configfile"
cat << EOF > "${FUNCT_TEST_TMPDIR}/kind-config/kind-config.yaml"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    listenAddress: "0.0.0.0"
  - containerPort: 443
    hostPort: 443
    listenAddress: "0.0.0.0"
  - containerPort: 6443
    hostPort: 32800
    listenAddress: "0.0.0.0"
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration #for worker use JoinConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        system-reserved: memory=2Gi
  extraMounts:
  - hostPath: "${FUNCT_TEST_COVERAGE}"
    containerPath: /tmp/coverage
networking:
  apiServerPort: 6443
EOF

#not used as we need to find a way to use token with kind.
export MANAGED_CLUSTER_API_SERVER_URL=$(cat ${KIND_MANAGED_KUBECONFIG_INTERNAL}| grep "server:" |cut -d ":" -f2 -f3 -f4 | sed 's/^ //')
export MANAGED_CLUSTER_TOKEN="itdove.thisisafaketoken"

cat << EOF > "${FUNCT_TEST_TMPDIR}/kind-config/kind-managed-config.yaml"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 81
    hostPort: 81
    listenAddress: "0.0.0.0"
  - containerPort: 444
    hostPort: 444
    listenAddress: "0.0.0.0"
  - containerPort: 6444
    hostPort: 32801
    listenAddress: "0.0.0.0"
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration #for worker use JoinConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        system-reserved: memory=2Gi
networking:
  apiServerPort: 6444
EOF

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
kind create cluster --name ${CLUSTER_NAME}-managed --config "${FUNCT_TEST_TMPDIR}/kind-config/kind-managed-config.yaml"
# setup kubeconfig
kind get kubeconfig --name ${CLUSTER_NAME}-managed > ${KIND_MANAGED_KUBECONFIG}
kind get kubeconfig --name ${CLUSTER_NAME}-managed --internal > ${KIND_MANAGED_KUBECONFIG_INTERNAL}
echo "creating hub cluster"
kind create cluster --name ${CLUSTER_NAME} --config "${FUNCT_TEST_TMPDIR}/kind-config/kind-config.yaml"

# setup kubeconfig
kind get kubeconfig --name ${CLUSTER_NAME} > ${KIND_KUBECONFIG}
kind get kubeconfig --name ${CLUSTER_NAME} --internal > ${KIND_KUBECONFIG_INTERNAL}
API_SERVER_URL=$(cat ${KIND_KUBECONFIG_INTERNAL}| grep "server:" |cut -d ":" -f2 -f3 -f4 | sed 's/^ //')

# load image if possible
kind load docker-image ${DOCKER_IMAGE_AND_TAG} --name=${CLUSTER_NAME} -v 99 || echo "failed to load image locally, will use imagePullSecret"

echo "install cluster"
# setup cluster
make kind-cluster-setup
for dir in overlays/test/* ; do
  echo ">>>>>>>>>>>>>>>Executing test: $dir"

  # install rcm-controller
  echo "install managedcluster-import-controller"
  kubectl apply -k "$dir" --dry-run=true -o yaml | sed "s|REPLACE_IMAGE|${DOCKER_IMAGE_AND_TAG}|g" | kubectl apply -f -

  echo "Create the cluster infrastructure"
  sed "s|API_SERVER_URL|${API_SERVER_URL}|g" ${FUNCT_TEST_TMPDIR}/CR/fake_infrastructure_cr.yaml | kubectl apply -f -

  # patch image
  echo "Wait rollout"
  kubectl rollout status -n open-cluster-management deployment managedcluster-import-controller --timeout=180s
  
  echo "run functional test..."
  make functional-test
  # exit 1
  echo "remove deployment"
  kubectl delete --wait=true -k "$dir"
done;

echo "Wait 10 sec for copy to AWS"
sleep 10

echo "delete clusters"
kind delete cluster --name ${CLUSTER_NAME}
kind delete cluster --name ${CLUSTER_NAME}-managed

if [ `find $FUNCT_TEST_COVERAGE -prune -empty 2>/dev/null` ]; then
  echo "no coverage files found. skipping"
else
  echo "merging coverage files"

  gocovmerge "${FUNCT_TEST_COVERAGE}/"* >> "${FUNCT_TEST_COVERAGE}/cover-functional.out"
  COVERAGE=$(go tool cover -func="${FUNCT_TEST_COVERAGE}/cover-functional.out" | grep "total:" | awk '{ print $3 }' | sed 's/[][()><%]/ /g')
  echo "-------------------------------------------------------------------------"
  echo "TOTAL COVERAGE IS ${COVERAGE}%"
  echo "-------------------------------------------------------------------------"

  go tool cover -html "${FUNCT_TEST_COVERAGE}/cover-functional.out" -o ${PROJECT_DIR}/test/functional/coverage/cover-functional.html
fi

