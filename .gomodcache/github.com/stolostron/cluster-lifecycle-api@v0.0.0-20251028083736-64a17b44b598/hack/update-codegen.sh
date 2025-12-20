#!/bin/bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../../../k8s.io/code-generator)}

verify="${VERIFY:-}"

source "${CODEGEN_PKG}/kube_codegen.sh"

for group in action view clusterinfo imageregistry klusterletconfig; do
  kube::codegen::gen_client \
    --output-pkg "github.com/stolostron/cluster-lifecycle-api/client/${group}" \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.txt" \
    --output-dir "${SCRIPT_ROOT}/client/${group}" \
    --one-input-api ${group} \
    --with-watch \
    .
done
