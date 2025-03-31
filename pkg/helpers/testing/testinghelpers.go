// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package testinghelpers

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
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
var crdsYaml = `
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: klusterlets.operator.open-cluster-management.io
spec: {}
`

var hostedImportYaml = `
---
apiVersion: v1
kind: Secret
metadata:
  name: "bootstrap-hub-kubeconfig"
  namespace: "klusterlet-test-hosted"
type: Opaque
data:
  kubeconfig: "test"
---
apiVersion: operator.open-cluster-management.io/v1
kind: Klusterlet
metadata:
  name: klusterlet
spec:
  deployOption:
    mode: Hosted
  registrationImagePullSpec: "registration:latest"
  workImagePullSpec: "work:latest"
  clusterName: "test"
  namespace: "open-cluster-management-agent"
`

func GetImportSecret(managedClusterName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-import", managedClusterName),
			Namespace: managedClusterName,
		},
		Data: map[string][]byte{
			"crds.yaml":   []byte(crdsYaml),
			"import.yaml": []byte(importYaml),
		},
	}
}

func GetHostedImportSecret(managedClusterName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-import", managedClusterName),
			Namespace: managedClusterName,
		},
		Data: map[string][]byte{
			"import.yaml": []byte(hostedImportYaml),
		},
	}
}

func BuildKubeconfig(restConfig *rest.Config) []byte {
	kubeConfigData, err := clientcmd.Write(
		clientcmdapi.Config{
			Clusters: map[string]*clientcmdapi.Cluster{"test-cluster": {
				Server:                   restConfig.Host,
				CertificateAuthorityData: restConfig.CAData,
				// InsecureSkipTLSVerify: true,
			}},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{"test-auth": {
				// Token: restConfig.BearerToken,
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

func NewRootCA(commoneName string) ([]byte, []byte, error) {
	// set up our CA certificate
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(2019),
		Subject:               pkix.Name{CommonName: commoneName},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	// pem encode
	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return nil, nil, err
	}

	caPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})
	if err != nil {
		return nil, nil, err
	}

	return caPEM.Bytes(), caPrivKeyPEM.Bytes(), nil
}

func NewServerCertificate(commonName string, caCertData, caKeyData []byte) ([]byte, []byte, error) {
	// set up our server certificate
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject:      pkix.Name{CommonName: commonName},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 6, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	block, _ := pem.Decode(caCertData)
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	block, _ = pem.Decode(caKeyData)
	caKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &certPrivKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := new(bytes.Buffer)
	err = pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	if err != nil {
		return nil, nil, err
	}

	certPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	if err != nil {
		return nil, nil, err
	}

	return certPEM.Bytes(), certPrivKeyPEM.Bytes(), nil
}

type managedClusterBuilder struct {
	name                  string
	setImportingCondition bool
	annotations           map[string]string
}

func NewManagedClusterBuilder(name string) *managedClusterBuilder {
	b := &managedClusterBuilder{
		name:                  name,
		setImportingCondition: true,
		annotations:           map[string]string{},
	}
	return b
}
func (b *managedClusterBuilder) WithImportingCondition(set bool) *managedClusterBuilder {
	b.setImportingCondition = set
	return b
}
func (b *managedClusterBuilder) WithAnnotations(k, v string) *managedClusterBuilder {
	b.annotations[k] = v
	return b
}

func (b *managedClusterBuilder) Build() *clusterv1.ManagedCluster {
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.name,
			Annotations: b.annotations,
		},
	}
	if b.setImportingCondition {
		mc.Status.Conditions = []metav1.Condition{
			{
				Type:    constants.ConditionManagedClusterImportSucceeded,
				Status:  metav1.ConditionFalse,
				Reason:  constants.ConditionReasonManagedClusterImporting,
				Message: "Start to import managed cluster test",
			},
		}
	}
	return mc
}
