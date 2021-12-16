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
COVERAGE_DIR="${REPO_DIR}/_output/coverage"

KIND_VERSION="v0.11.1"
KIND="${WORK_DIR}/bin/kind"

KUBE_VERSION="v1.20.2"
KUBECTL="${WORK_DIR}/bin/kubectl"

CLUSTER_NAME="e2e-test-cluster"

mkdir -p "${WORK_DIR}/bin"
mkdir -p "${WORK_DIR}/config"
mkdir -p "${COVERAGE_DIR}"

echo "###### installing kind"
curl -s -f -L "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-${GOHOSTOS}-${GOHOSTARCH}" -o "${KIND}"
chmod +x "${KIND}"

CLEAN_ARG=${1:-unclean}
if [ "$CLEAN_ARG"x = "clean"x ]; then
    ${KIND} delete cluster --name ${CLUSTER_NAME}
    exit 0
fi

echo "###### installing kubectl"
curl -s -f -L "https://storage.googleapis.com/kubernetes-release/release/${KUBE_VERSION}/bin/${GOHOSTOS}/${GOHOSTARCH}/kubectl" -o "${KUBECTL}"
chmod +x "${KUBECTL}"

echo "###### installing e2e test cluster"
export KUBECONFIG="${WORK_DIR}/kubeconfig"
${KIND} delete cluster --name ${CLUSTER_NAME}
# NOTE: If you are using Docker for Mac or Windows check that the hostPath is included in the Preferences -> Resources -> File Sharing.
cat << EOF | ${KIND} create cluster --image kindest/node:${KUBE_VERSION} --name ${CLUSTER_NAME} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: "${COVERAGE_DIR}"
    containerPath: /tmp/coverage
EOF
cluster_ip=$(${KUBECTL} get svc kubernetes -n default -o jsonpath="{.spec.clusterIP}")
cluster_context=$(${KUBECTL} config current-context)

echo "###### loading coverage image"
${KIND} load docker-image managedcluster-import-controller-coverage --name ${CLUSTER_NAME}

echo "###### deploy registration-operator"
rm -rf "$WORK_DIR/registration-operator"
git clone https://github.com/open-cluster-management/registration-operator.git "$WORK_DIR/registration-operator"
${KUBECTL} apply -k "$WORK_DIR/registration-operator/deploy/cluster-manager/config/manifests"
${KUBECTL} apply -k "$WORK_DIR/registration-operator/deploy/cluster-manager/config/samples"
rm -rf "$WORK_DIR/registration-operator"
sleep 10
${KUBECTL} -n open-cluster-management rollout status deploy cluster-manager --timeout=120s
${KUBECTL} -n open-cluster-management-hub rollout status deploy cluster-manager-registration-controller --timeout=120s
${KUBECTL} -n open-cluster-management-hub rollout status deploy cluster-manager-registration-webhook --timeout=120s
${KUBECTL} -n open-cluster-management-hub rollout status deploy cluster-manager-work-webhook --timeout=120s

echo "###### deploy managedcluster-import-controller with image coverage image"
kubectl kustomize "$REPO_DIR/deploy/test" | kubectl apply -f -
sleep 5
${KUBECTL} -n open-cluster-management rollout status deploy managedcluster-import-controller --timeout=120s

echo "###### prepare required crds"
${KUBECTL} apply -f "$REPO_DIR/test/e2e/resources/ocm"
${KUBECTL} apply -f "$REPO_DIR/test/e2e/resources/hive"
${KUBECTL} apply -f "$REPO_DIR/test/e2e/resources/ocp"
${KUBECTL} apply -f "$REPO_DIR/test/e2e/resources/hypershift"
cat << EOF | ${KUBECTL} apply -f -
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
spec:
  cloudConfig:
    name: ""
status:
  apiServerInternalURI: https://${cluster_ip}
  apiServerURL: https://${cluster_ip}
EOF

echo "###### prepare auto-import-secret"
cp "${KUBECONFIG}" "${WORK_DIR}"/e2e-kubeconfig
${KUBECTL} config set "clusters.${cluster_context}.server" "https://${cluster_ip}" --kubeconfig "${WORK_DIR}"/e2e-kubeconfig
${KUBECTL} delete secret e2e-auto-import-secret -n open-cluster-management --ignore-not-found
${KUBECTL} create secret generic e2e-auto-import-secret --from-file=kubeconfig="${WORK_DIR}"/e2e-kubeconfig -n open-cluster-management

echo "###### prepare serviceaccouts"
cat << EOF | ${KUBECTL} apply -f -
kind: ServiceAccount
apiVersion: v1
metadata:
  name: managed-cluster-import-e2e-sa
  namespace: open-cluster-management
---
kind: ServiceAccount
apiVersion: v1
metadata:
  name: managed-cluster-import-e2e-limited-sa
  namespace: open-cluster-management
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: managed-cluster-import-e2e
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: managed-cluster-import-e2e-sa
    namespace: open-cluster-management
EOF

echo "###### prepare imageregistry"
cat << EOF | ${KUBECTL} apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: e2e-registry
---
apiVersion: v1
kind: Secret
metadata:
  name: e2e-pull-secret
  namespace: e2e-registry
data:
  .dockerconfigjson: ewogICJhdXRocyI6IHsKICB9Cn0=
type: kubernetes.io/dockerconfigjson
---
apiVersion: imageregistry.open-cluster-management.io/v1alpha1
kind: ManagedClusterImageRegistry
metadata:
  name: e2e-image-registry
  namespace: e2e-registry
spec:
  registry: e2e.test
  pullSecret:
    name: e2e-pull-secret
  placementRef:
    group: cluster.open-cluster-management.io
    resource: placement
    name: test
EOF

# start the e2e test
${WORK_DIR}/e2e.test -test.v -ginkgo.v

echo "###### dump the test coverage"
rm -rf "${COVERAGE_DIR}"/*
# restart the controller to send the kill signal to get the e2e-test coverage
kubectl -n open-cluster-management delete pods --wait=true -l name=managedcluster-import-controller

if [ -f "${COVERAGE_DIR}/e2e-test-coverage.out" ]; then
  COVERAGE=$(go tool cover -func="${COVERAGE_DIR}/e2e-test-coverage.out" | grep "total:" | awk '{print $3}')
  echo "-------------------------------------------------------------------------"
  echo "TOTAL COVERAGE IS ${COVERAGE}"
  echo "-------------------------------------------------------------------------"
fi
