#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

# Script to clean up kind clusters created by setup-kind-clusters.sh

set -o errexit
set -o nounset

CLUSTER_NAME="e2e-test-cluster"
CLUSTER_NAME_MANAGED="e2e-test-cluster-managed"

BUILD_DIR="$(
  cd "$(dirname "$0")" >/dev/null 2>&1
  pwd -P
)"
REPO_DIR="$(dirname "$BUILD_DIR")"
WORK_DIR="${REPO_DIR}/_output"

KIND_VERSION="v0.17.0"
KIND="${WORK_DIR}/bin/kind"

# Check if kind binary exists
if [ ! -f "${KIND}" ]; then
  echo "Kind binary not found at ${KIND}. Please run setup-kind-clusters.sh first to install kind."
  exit 1
fi

echo "###### cleaning up kind clusters"
${KIND} delete cluster --name ${CLUSTER_NAME} || echo "Cluster ${CLUSTER_NAME} not found or already deleted"
${KIND} delete cluster --name ${CLUSTER_NAME_MANAGED} || echo "Cluster ${CLUSTER_NAME_MANAGED} not found or already deleted"

echo "###### cleanup completed"
