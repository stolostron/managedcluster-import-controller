// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package testinghelpers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var importYaml = `
---
apiVersion: v1
kind: Namespace
metadata:
  annotations:
    workload.openshift.io/allowed: "management"
  name: "open-cluster-management-agent"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: klusterlet
  namespace: "open-cluster-management-agent"
imagePullSecrets:
- name: "open-cluster-management-image-pull-credentials"
---
apiVersion: v1
kind: Secret
metadata:
  name: "bootstrap-hub-kubeconfig"
  namespace: "open-cluster-management-agent"
type: Opaque
data:
  kubeconfig: "test"
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: klusterlet
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: open-cluster-management:klusterlet-admin-aggregate-clusterrole
  labels:
    rbac.authorization.k8s.io/aggregate-to-admin: "true"
rules:
- apiGroups: ["operator.open-cluster-management.io"]
  resources: ["klusterlets"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: klusterlet
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: klusterlet
subjects:
- kind: ServiceAccount
  name: klusterlet
  namespace: "open-cluster-management-agent"
---
kind: Deployment
apiVersion: apps/v1
metadata:
  name: klusterlet
  namespace: "open-cluster-management-agent"
  labels:
    app: klusterlet
spec:
  replicas: 1
  selector:
    matchLabels:
      app: klusterlet
  template:
    metadata:
      labels:
        app: klusterlet
    spec:
      serviceAccountName: klusterlet
      containers:
      - name: klusterlet
        image: registration-operator:latest
        imagePullPolicy: IfNotPresent
        args:
          - "/registration-operator"
          - "klusterlet"
---
apiVersion: operator.open-cluster-management.io/v1
kind: Klusterlet
metadata:
  name: klusterlet
spec:
  registrationImagePullSpec: "registration:latest"
  workImagePullSpec: "work:latest"
  clusterName: "test"
  namespace: "open-cluster-management-agent"
---
apiVersion: v1
kind: Secret
metadata:
  name: "open-cluster-management-image-pull-credentials"
  namespace: "open-cluster-management-agent"
type: kubernetes.io/dockerconfigjson
data:
    .dockerconfigjson: test
`
var crdsv1Yaml = `
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: klusterlets.operator.open-cluster-management.io
spec: {}
`

var crdsv1beta1Yaml = `
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  name: klusterlets.operator.open-cluster-management.io
spec: {}
`

func GetImportSecret(managedClusterName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-import", managedClusterName),
			Namespace: managedClusterName,
		},
		Data: map[string][]byte{
			"crdsv1.yaml":      []byte(crdsv1Yaml),
			"crdsv1beta1.yaml": []byte(crdsv1beta1Yaml),
			"crds.yaml":        []byte(crdsv1Yaml),
			"import.yaml":      []byte(importYaml),
		},
	}
}

func BuildKubeconfig(restConfig *rest.Config) []byte {
	kubeConfigData, err := clientcmd.Write(
		clientcmdapi.Config{
			Clusters: map[string]*clientcmdapi.Cluster{"test-cluster": {
				Server:                   restConfig.Host,
				CertificateAuthorityData: restConfig.CAData,
				//InsecureSkipTLSVerify: true,
			}},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{"test-auth": {
				//Token: restConfig.BearerToken,
				ClientCertificateData: restConfig.CertData,
				ClientKeyData:         restConfig.KeyData,
			}},
			Contexts: map[string]*clientcmdapi.Context{"test-context": {
				Cluster:   "test-cluster",
				AuthInfo:  "test-auth",
				Namespace: "configuration",
			}},
			CurrentContext: "test-context",
		})
	if err != nil {
		panic(err)
	}
	return kubeConfigData
}
