#!/bin/bash
###############################################################################
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

set -o errexit
set -o nounset

# Input: KUBECTL(kubectl or oc), OCM_VERSION, E2E_KUBECONFIG, E2E_MANAGED_KUBECONFIG, cluster_ip, cluster_context

KUBECTL=${KUBECTL:-kubectl}
OCM_VERSION=${OCM_VERSION:-main}
IMPORT_CONTROLLER_IMAGE_NAME=${IMPORT_CONTROLLER_IMAGE_NAME:-managedcluster-import-controller:latest}

BUILD_DIR="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
REPO_DIR="$(dirname "$BUILD_DIR")"
WORK_DIR="${REPO_DIR}/_output"

mkdir -p "${WORK_DIR}"

E2E_KUBECONFIG="${WORK_DIR}/e2e-kubeconfig"
E2E_MANAGED_KUBECONFIG="${WORK_DIR}/e2e-managed-kubeconfig"
E2E_EXTERNAL_MANAGED_KUBECONFIG="${WORK_DIR}/e2e-external-managed-kubeconfig"

export OCM_BRANCH=$OCM_VERSION
export REGISTRATION_OPERATOR_IMAGE=quay.io/stolostron/registration-operator:$OCM_VERSION
export REGISTRATION_IMAGE=quay.io/stolostron/registration:$OCM_VERSION
export WORK_IMAGE=quay.io/stolostron/work:$OCM_VERSION

echo "###### deploy managedcluster-import-controller by image $IMPORT_CONTROLLER_IMAGE_NAME"

export DEPLOY_MANIFESTS="${REPO_DIR}/deploy/base"

AGENT_REGISTRATION_ARG=${1:-disable-agent-registration}
if [ "$AGENT_REGISTRATION_ARG"x = "enable-agent-registration"x ]; then
    DEPLOY_MANIFESTS="${REPO_DIR}/deploy/agentregistration"
fi

#  -e "s,ENV_TYPE_VALUE,e2e," \
kubectl kustomize $DEPLOY_MANIFESTS \
  | sed -e "s,quay.io/open-cluster-management/registration:latest,$REGISTRATION_IMAGE," \
  -e "s,quay.io/open-cluster-management/work:latest,$WORK_IMAGE," \
  -e "s,quay.io/open-cluster-management/registration-operator:latest,$REGISTRATION_OPERATOR_IMAGE," \
  -e "s,managedcluster-import-controller:latest,$IMPORT_CONTROLLER_IMAGE_NAME," \
  | kubectl apply -f -

sleep 5
${KUBECTL} -n open-cluster-management rollout status deploy managedcluster-import-controller --timeout=300s

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
${KUBECTL} delete secret e2e-managed-external-secret -n open-cluster-management --ignore-not-found
${KUBECTL} create secret generic e2e-managed-external-secret --from-file=kubeconfig=$E2E_EXTERNAL_MANAGED_KUBECONFIG -n open-cluster-management

AGENT_REGISTRATION_ARG=${1:-disable-agent-registration}
if [ "$AGENT_REGISTRATION_ARG"x = "enable-agent-registration"x ]; then
echo "###### prepare agent-regitration"

echo "###### get host of agent-registration server"
export agent_registration_host=$(${KUBECTL} get route -n open-cluster-management agent-registration -o=jsonpath="{.spec.host}")
echo "host: $agent_registration_host"

echo "###### get CA from the hub cluster"
${KUBECTL} get configmap -n kube-system kube-root-ca.crt -o=jsonpath="{.data['ca\.crt']}" > ca.crt

echo "###### create a serviceaccount binding with clientclusterrole, and create a token for the serviceaccount"

cat << EOF | ${KUBECTL} apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: managed-cluster-import-e2e-agent-registration-sa
  namespace: open-cluster-management
---
apiVersion: v1
kind: Secret
type: kubernetes.io/service-account-token
metadata:
  name: managed-cluster-import-e2e-agent-registration-sa-token
  namespace: open-cluster-management
  annotations:
    kubernetes.io/service-account.name: "managed-cluster-import-e2e-agent-registration-sa"
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: managed-cluster-import-e2e-agent-registration
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: managedcluster-import-controller-agent-registration-client
subjects:
  - kind: ServiceAccount
    name: managed-cluster-import-e2e-agent-registration-sa
    namespace: open-cluster-management
EOF

# get serviceaccount token
export token=$(${KUBECTL} get secret -n open-cluster-management managed-cluster-import-e2e-agent-registration-sa-token -o=jsonpath='{.data.token}' | base64 -d)

echo "###### check agent-registration server is healthy"
response=$(curl -s -o /dev/null -w "%{http_code}" --cacert ca.crt -H "Authorization: Bearer $token" https://$agent_registration_host/agent-registration)
if [ "$response" != "200" ]; then
  echo "Error: Agent registration server health check failed with status code $response"
  exit 1
fi

echo "###### apply crds from the endpoint"
curl --cacert ca.crt -H "Authorization: Bearer $token" https://$agent_registration_host/agent-registration/crds/v1 | kubectl apply -f -

echo "###### apply manifest from the endpoint"
curl --cacert ca.crt -H "Authorization: Bearer $token" https://$agent_registration_host/agent-registration/manifests/cluster-e2e-test-agent?klusterletconfig=default | kubectl apply -f -

fi
