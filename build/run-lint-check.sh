#!/bin/bash
# Copyright Contributors to the Open Cluster Management project
#set -x
set -eo pipefail

GOLANGCI_LINT_VERSION="1.37.1"
GOLANGCI_LINT_CACHE=/tmp/golangci-cache
GOOS=$(go env GOOS)
GOPATH=$(go env GOPATH)
export GOFLAGS=""

if ! which golangci-lint > /dev/null; then
    mkdir -p "${GOPATH}/bin"
    echo "${PATH}" | grep -q "${GOPATH}/bin"
    IN_PATH=$?
    if [ $IN_PATH != 0 ]; then
        echo "${GOPATH}/bin not in $$PATH"
        exit 1
    fi
    DOWNLOAD_URL="https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-${GOOS}-amd64.tar.gz"
    curl -sfL "${DOWNLOAD_URL}" | tar -C "${GOPATH}/bin" -zx --strip-components=1 "golangci-lint-${GOLANGCI_LINT_VERSION}-${GOOS}-amd64/golangci-lint"
fi

echo 'Running linting tool ...'
GOLANGCI_LINT_CACHE=${GOLANGCI_LINT_CACHE} golangci-lint run -c build/golangci.yml
#$(GOLANGCI_LINT_CACHE=${GOLANGCI_LINT_CACHE} golangci-lint run -c build/golangci.yml)
echo '##### lint-check #### Success' 