#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -o errexit
set -o nounset

BUILD_DIR="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
REPO_DIR="$(dirname "$BUILD_DIR")"
WORK_DIR="${REPO_DIR}/_output"

mkdir -p "${WORK_DIR}"

E2E_KUBECONFIG="${WORK_DIR}/e2e-kubeconfig"
E2E_MANAGED_KUBECONFIG="${WORK_DIR}/e2e-managed-kubeconfig"

cp $KUBECONFIG $E2E_KUBECONFIG
cp $KUBECONFIG $E2E_MANAGED_KUBECONFIG
