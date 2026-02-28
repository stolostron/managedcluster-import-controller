#!/bin/bash -e
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -o errexit
set -o nounset

build_dir="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
repo_dir="$(dirname "$build_dir")"
pkg_dir="${repo_dir}/pkg/..."
coverage_dir="${repo_dir}/_output/unit/coverage"

mkdir -p ${coverage_dir}

# Set up envtest binaries using sdk-go ensure-envtest.sh
ENSURE_ENVTEST_SCRIPT_REF="${ENSURE_ENVTEST_SCRIPT_REF:-main}"
ENSURE_ENVTEST_SCRIPT="https://raw.githubusercontent.com/open-cluster-management-io/sdk-go/${ENSURE_ENVTEST_SCRIPT_REF}/ci/envtest/ensure-envtest.sh"
export KUBEBUILDER_ASSETS=$(curl -fsSL "${ENSURE_ENVTEST_SCRIPT}" | bash)
echo "KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"

echo "Running unit test in $pkg_dir"
# Workaround for Go 1.25.x build cache regression with CGO_ENABLED=1
# See: https://github.com/golang/go/issues/76946
go clean -cache
go test -cover -covermode=atomic -coverprofile=${coverage_dir}/cover.out ${pkg_dir}

COVERAGE=$(go tool cover -func=_output/unit/coverage/cover.out | grep "total:" | awk '{ print $3 }')
echo "-------------------------------------------------------------------------"
echo "TOTAL COVERAGE IS ${COVERAGE}"
echo "-------------------------------------------------------------------------"
