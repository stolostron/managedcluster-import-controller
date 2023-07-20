#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -o errexit
set -o nounset

# Input: KUBECTL(kubectl or oc), OCM_VERSION, E2E_KUBECONFIG, E2E_MANAGED_KUBECONFIG, cluster_ip, cluster_context

KUBECTL=${KUBECTL:-kubectl}

BUILD_DIR="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
REPO_DIR="$(dirname "$BUILD_DIR")"
WORK_DIR="${REPO_DIR}/_output"

mkdir -p "${WORK_DIR}"

E2E_KUBECONFIG="${WORK_DIR}/e2e-kubeconfig"
E2E_MANAGED_KUBECONFIG="${WORK_DIR}/e2e-managed-kubeconfig"

export OCM_VERSION=main
export OCM_BRANCH=$OCM_VERSION
export REGISTRATION_OPERATOR_IMAGE=quay.io/stolostron/registration-operator:$OCM_VERSION
export REGISTRATION_IMAGE=quay.io/stolostron/registration:$OCM_VERSION
export WORK_IMAGE=quay.io/stolostron/work:$OCM_VERSION

echo "###### deploy managedcluster-import-controller"

kubectl kustomize "$REPO_DIR/deploy/base" \
  | sed -e "s,quay.io/open-cluster-management/registration:latest,$REGISTRATION_IMAGE," \
  -e "s,quay.io/open-cluster-management/work:latest,$WORK_IMAGE," \
  -e "s,quay.io/open-cluster-management/registration-operator:latest,$REGISTRATION_OPERATOR_IMAGE," \
  | kubectl apply -f -

sleep 5
${KUBECTL} -n open-cluster-management rollout status deploy managedcluster-import-controller --timeout=120s

echo "###### prepare auto-import-secret"
cluster_ip=$(${KUBECTL} get svc kubernetes -n default -o jsonpath="{.spec.clusterIP}")
cluster_context=$(${KUBECTL} config current-context)
${KUBECTL} config set "clusters.${cluster_context}.server" "https://${cluster_ip}" --kubeconfig $E2E_KUBECONFIG
${KUBECTL} delete secret e2e-auto-import-secret -n open-cluster-management --ignore-not-found
${KUBECTL} create secret generic e2e-auto-import-secret --from-file=kubeconfig=$E2E_KUBECONFIG -n open-cluster-management

echo "###### prepare serviceaccouts"
cat << EOF | ${KUBECTL} apply -f -
kind: ServiceAccount
apiVersion: v1
metadata:
  name: managed-cluster-import-e2e-sa
  namespace: open-cluster-management
---
apiVersion: v1
kind: Secret
type: kubernetes.io/service-account-token
metadata:
  name: managed-cluster-import-e2e-sa-token
  namespace: open-cluster-management
  annotations:
    kubernetes.io/service-account.name: "managed-cluster-import-e2e-sa"
---
kind: ServiceAccount
apiVersion: v1
metadata:
  name: managed-cluster-import-e2e-limited-sa
  namespace: open-cluster-management
---
apiVersion: v1
kind: Secret
type: kubernetes.io/service-account-token
metadata:
  name: managed-cluster-import-e2e-limited-sa-token
  namespace: open-cluster-management
  annotations:
    kubernetes.io/service-account.name: "managed-cluster-import-e2e-limited-sa"
---
kind: ServiceAccount
apiVersion: v1
metadata:
  name: managed-cluster-import-e2e-restore-sa
  namespace: open-cluster-management
---
apiVersion: v1
kind: Secret
type: kubernetes.io/service-account-token
metadata:
  name: managed-cluster-import-e2e-restore-sa-token
  namespace: open-cluster-management
  annotations:
    kubernetes.io/service-account.name: "managed-cluster-import-e2e-restore-sa"
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
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: managed-cluster-import-e2e-restore
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: klusterlet-bootstrap-kubeconfig
subjects:
  - kind: ServiceAccount
    name: managed-cluster-import-e2e-restore-sa
    namespace: open-cluster-management
EOF

echo "###### prepare auto-import-secret for hosted cluster"
${KUBECTL} delete secret e2e-managed-auto-import-secret -n open-cluster-management --ignore-not-found
${KUBECTL} create secret generic e2e-managed-auto-import-secret --from-file=kubeconfig=$E2E_MANAGED_KUBECONFIG -n open-cluster-management
