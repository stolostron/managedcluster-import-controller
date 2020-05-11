#!/bin/bash
###############################################################################
# (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
# Note to U.S. Government Users Restricted Rights:
# U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
# Contract with IBM Corp.
# Licensed Materials - Property of IBM
# Copyright (c) 2020 Red Hat, Inc.
###############################################################################

set -e


CURR_FOLDER_PATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
KIND_KUBECONFIG="${CURR_FOLDER_PATH}/../kind_kubeconfig.yaml"
export KUBECONFIG=${KIND_KUBECONFIG}
export DOCKER_IMAGE_AND_TAG=${1}

export FUNCT_TEST_TMPDIR="${CURR_FOLDER_PATH}/../test/functional_test_tmp"

if ! which kubectl > /dev/null; then
    echo "installing kubectl"
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/
fi
if ! which kind > /dev/null; then
    echo "installing kind"
    curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.7.0/kind-$(uname)-amd64
    chmod +x ./kind
    sudo mv ./kind /usr/local/bin/kind
fi
if ! which ginkgo > /dev/null; then
    export GO111MODULE=off
    echo "Installing ginkgo ..."
    go get github.com/onsi/ginkgo/ginkgo
    go get github.com/onsi/gomega/...
fi
if ! which gocovmerge > /dev/null; then
  echo "Installing gocovmerge..."
  go get -u github.com/wadey/gocovmerge
fi

echo "setting up test folder"
[ -d "$FUNCT_TEST_TMPDIR" ] && rm -r "$FUNCT_TEST_TMPDIR"
mkdir -p "$FUNCT_TEST_TMPDIR"
mkdir -p "$FUNCT_TEST_TMPDIR/output"
mkdir -p "$FUNCT_TEST_TMPDIR/kind-config"

echo "generating kind configfile"
cat << EOF > "${FUNCT_TEST_TMPDIR}/kind-config/kind-config.yaml"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: "${FUNCT_TEST_TMPDIR}/output"
    containerPath: /tmp/coverage
EOF

echo "creating cluster"
kind create cluster --name functional-test --config "${FUNCT_TEST_TMPDIR}/kind-config/kind-config.yaml"

# setup kubeconfig
kind get kubeconfig --name functional-test > ${KIND_KUBECONFIG}

# load image if possible
kind load docker-image ${DOCKER_IMAGE_AND_TAG} --name=functional-test -v 99 || echo "failed to load image locally, will use imagePullSecret"

echo "install cluster"
# setup cluster
make kind-cluster-setup

for dir in overlays/test/* ; do
  echo ">>>>>>>>>>>>>>>Executing test: $dir"
  
  # install rcm-controller
  echo "install rcm-controller"
  kubectl apply -k "$dir"
  echo "install imagePullSecret"
  kubectl create secret -n open-cluster-management docker-registry multiclusterhub-operator-pull-secret --docker-server=quay.io --docker-username=${DOCKER_USER} --docker-password=${DOCKER_PASS}
  
  # patch image
  echo "patch image"
  kubectl patch deployment rcm-controller -n open-cluster-management -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"rcm-controller\",\"image\":\"${DOCKER_IMAGE_AND_TAG}\"}]}}}}"
  kubectl rollout status -n open-cluster-management deployment rcm-controller --timeout=90s
  sleep 10
  
  echo "run functional test..."
  make functional-test
  
  echo "remove deployment"
  kubectl delete -k "$dir"

done;

echo "delete cluster"
kind delete cluster --name functional-test

if [ `find $FUNCT_TEST_TMPDIR/output -prune -empty 2>/dev/null` ]; then
  echo "no coverage files found. skipping"
else
  echo "merging coverage files"
  # report coverage if has any coverage files
  rm -rf "${CURR_FOLDER_PATH}/../test/coverage-functional"
  mkdir -p "${CURR_FOLDER_PATH}/../test/coverage-functional"
  
  cp "$FUNCT_TEST_TMPDIR/output/"* "${CURR_FOLDER_PATH}/../test/coverage-functional/"
  ls -l "${CURR_FOLDER_PATH}/../test/coverage-functional/"
  
  gocovmerge "${CURR_FOLDER_PATH}/../test/coverage-functional/"* >> "${CURR_FOLDER_PATH}/../test/coverage-functional/cover-functional.out"
  COVERAGE=$(go tool cover -func="${CURR_FOLDER_PATH}/../test/coverage-functional/cover-functional.out" | grep "total:" | awk '{ print $3 }' | sed 's/[][()><%]/ /g')
  echo "-------------------------------------------------------------------------"
  echo "TOTAL COVERAGE IS ${COVERAGE}%"
  echo "-------------------------------------------------------------------------"
  
  go tool cover -html "${CURR_FOLDER_PATH}/../test/coverage-functional/cover-functional.out" -o ${PROJECT_DIR}/test/coverage-functional/cover-functional.html
fi