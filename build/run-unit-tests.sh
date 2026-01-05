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

export KUBEBUILDER_ASSETS="${repo_dir}/_output/kubebuilder/bin"

k8s_version="1.30.0"
kubebuilder="kubebuilder-tools-${k8s_version}-${GOHOSTOS}-${GOHOSTARCH}.tar.gz"
kubebuilder_path="${repo_dir}/_output/${kubebuilder}"

if [ ! -d "${KUBEBUILDER_ASSETS}" ]; then
    echo "Downloading kubebuilder ${k8s_version} into $KUBEBUILDER_ASSETS"
    mkdir -p "${KUBEBUILDER_ASSETS}"
	curl -s -f -L "https://storage.googleapis.com/kubebuilder-tools/${kubebuilder}" -o "${kubebuilder_path}"
	tar -C "${KUBEBUILDER_ASSETS}" --strip-components=2 -zvxf "${kubebuilder_path}"
fi

echo "Running unit test in $pkg_dir"
# Workaround for Go 1.25.0 build cache regression with CGO_ENABLED=1
# Clear cache before running tests to avoid corruption
# See: https://github.com/golang/go/issues/69566
go clean -cache
go test -cover -covermode=atomic -coverprofile=${coverage_dir}/cover.out ${pkg_dir}

COVERAGE=$(go tool cover -func=_output/unit/coverage/cover.out | grep "total:" | awk '{ print $3 }')
echo "-------------------------------------------------------------------------"
echo "TOTAL COVERAGE IS ${COVERAGE}"
echo "-------------------------------------------------------------------------"
