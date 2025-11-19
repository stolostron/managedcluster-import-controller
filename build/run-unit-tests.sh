#!/bin/bash -e
###############################################################################
# (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
# Note to U.S. Government Users Restricted Rights:
# U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
# Contract with IBM Corp.
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -o errexit
set -o nounset

build_dir="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
repo_dir="$(dirname "$build_dir")"
pkg_dir="${repo_dir}/pkg/..."
cache_dir="${repo_dir}/_output/unit/cache"
coverage_dir="${repo_dir}/_output/unit/coverage"

mkdir -p ${cache_dir}
mkdir -p ${coverage_dir}

# required by kubebuilder-envtest
export XDG_CACHE_HOME="${cache_dir}"

# Use setup-envtest to download and manage envtest binaries
# Install setup-envtest if not present
if ! command -v setup-envtest &> /dev/null; then
    echo "Installing setup-envtest..."
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
fi

# Get the path to envtest binaries for k8s 1.34.x
export KUBEBUILDER_ASSETS=$(setup-envtest use -p path 1.34.x)

echo "Running unit test in $pkg_dir"
go test -cover -covermode=atomic -coverprofile=${coverage_dir}/cover.out ${pkg_dir}

COVERAGE=$(go tool cover -func=_output/unit/coverage/cover.out | grep "total:" | awk '{ print $3 }')
echo "-------------------------------------------------------------------------"
echo "TOTAL COVERAGE IS ${COVERAGE}"
echo "-------------------------------------------------------------------------"
