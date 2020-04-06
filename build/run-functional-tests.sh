#!/bin/bash
set -e

CURR_FOLDER_PATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
KIND_KUBECONFIG="${CURR_FOLDER_PATH}/../kind_kubeconfig.yaml"
export KUBECONFIG=${KIND_KUBECONFIG}
export DOCKER_IMAGE_AND_TAG=${1}

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


echo "creating cluster"
make kind-create-cluster 

# setup kubeconfig
kind get kubeconfig --name functional-test > ${KIND_KUBECONFIG}

echo "install cluster"
# setup cluster
make kind-cluster-setup

# install rcm-controller
echo "install rcm-controller"
make kind-install-rcm-controller

# patch image
echo "patch image"
kubectl patch deployment rcm-controller -n open-cluster-management -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"rcm-controller\",\"image\":\"${DOCKER_IMAGE_AND_TAG}\"}]}}}}"
kubectl rollout status -n open-cluster-management deployment rcm-controller --timeout=90s
sleep 10

echo "running functional test..."
make functional-test

echo "delete cluster"
make kind-delete-cluster 

