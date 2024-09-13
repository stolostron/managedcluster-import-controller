// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/onsi/ginkgo/v2"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
)

type Label struct {
	key   string
	value string
}

const ocmNamespace = "open-cluster-management"

var (
	infrastructureGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "infrastructures",
	}
	clusterdeploymentGVR = schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterdeployments",
	}
)

func Logf(format string, args ...interface{}) {
	fmt.Fprintf(ginkgo.GinkgoWriter, "DEBUG: "+format+"\n", args...)
}

func CreateHostedManagedCluster(clusterClient clusterclient.Interface, name, management string) (*clusterv1.ManagedCluster, error) {
	clusterAnnotations := map[string]string{}
	clusterAnnotations[constants.KlusterletDeployModeAnnotation] = string(operatorv1.InstallModeHosted)
	clusterAnnotations[constants.HostingClusterNameAnnotation] = management
	cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return clusterClient.ClusterV1().ManagedClusters().Create(
			context.TODO(),
			&clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Annotations: clusterAnnotations,
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
			},
			metav1.CreateOptions{},
		)
	}

	return cluster, err
}

func CreateHostedManagedClusterWithShortLeaseDuration(clusterClient clusterclient.Interface, name, management string) (*clusterv1.ManagedCluster, error) {
	clusterAnnotations := map[string]string{}
	clusterAnnotations[constants.KlusterletDeployModeAnnotation] = string(operatorv1.InstallModeHosted)
	clusterAnnotations[constants.HostingClusterNameAnnotation] = management
	cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return clusterClient.ClusterV1().ManagedClusters().Create(
			context.TODO(),
			&clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Annotations: clusterAnnotations,
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 5,
				},
			},
			metav1.CreateOptions{},
		)
	}

	return cluster, err
}

func CreateManagedCluster(clusterClient clusterclient.Interface, name string, labels ...Label) (*clusterv1.ManagedCluster, error) {
	clusterLabels := map[string]string{}
	for _, label := range labels {
		clusterLabels[label.key] = label.value
	}

	cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), name, metav1.GetOptions{})
	Logf("create managed cluster get cluster error: %v, cluster: %s", err, cluster)
	if errors.IsNotFound(err) {
		return clusterClient.ClusterV1().ManagedClusters().Create(
			context.TODO(),
			&clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:   name,
					Labels: clusterLabels,
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
			},
			metav1.CreateOptions{},
		)
	}

	return cluster, err
}

func CreateManagedClusterWithAnnotations(
	clusterClient clusterclient.Interface,
	name string,
	annotations map[string]string,
	labels ...Label) (*clusterv1.ManagedCluster, error) {
	clusterLabels := map[string]string{}
	for _, label := range labels {
		clusterLabels[label.key] = label.value
	}

	cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), name, metav1.GetOptions{})
	Logf("create managed cluster get cluster error: %v, cluster: %s", err, cluster)
	if errors.IsNotFound(err) {
		return clusterClient.ClusterV1().ManagedClusters().Create(
			context.TODO(),
			&clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Labels:      clusterLabels,
					Annotations: annotations,
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
			},
			metav1.CreateOptions{},
		)
	}

	return cluster, err
}

func RemoveManagedClusterAnnotations(clusterClient clusterclient.Interface, name string) error {
	cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	cluster.Annotations = map[string]string{}

	_, err = clusterClient.ClusterV1().ManagedClusters().Update(context.TODO(), cluster, metav1.UpdateOptions{})
	return err
}

func CreateManagedClusterWithShortLeaseDuration(clusterClient clusterclient.Interface, name string,
	annotations map[string]string, labels ...Label) (*clusterv1.ManagedCluster, error) {
	clusterLabels := map[string]string{}
	for _, label := range labels {
		clusterLabels[label.key] = label.value
	}

	cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), name, metav1.GetOptions{})
	Logf("create short lease duration managed cluster get cluster error: %v, cluster: %s", err, cluster)
	if errors.IsNotFound(err) {
		return clusterClient.ClusterV1().ManagedClusters().Create(
			context.TODO(),
			&clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Labels:      clusterLabels,
					Annotations: annotations,
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 5,
				},
			},
			metav1.CreateOptions{},
		)
	}

	return cluster, err
}

func CreateClusterDeployment(dynamicClient dynamic.Interface, clusterName string) error {
	clusterdeployments := dynamicClient.Resource(clusterdeploymentGVR).Namespace(clusterName)
	clusterDeployment := newClusterdeployment(clusterName)

	_, err := clusterdeployments.Get(context.TODO(), clusterName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err := clusterdeployments.Create(context.TODO(), clusterDeployment, metav1.CreateOptions{})
		return err
	}
	return err
}

func GetClusterDeployment(dynamicClient dynamic.Interface, clusterName string) (*unstructured.Unstructured, error) {
	clusterdeployments := dynamicClient.Resource(clusterdeploymentGVR).Namespace(clusterName)
	return clusterdeployments.Get(context.TODO(), clusterName, metav1.GetOptions{})
}

func InstallClusterDeployment(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, clusterName string) error {
	clusterdeployments := dynamicClient.Resource(clusterdeploymentGVR).Namespace(clusterName)
	clusterDeployment, err := clusterdeployments.Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	clusterDeployment = clusterDeployment.DeepCopy()
	if err := unstructured.SetNestedField(clusterDeployment.Object, true, "spec", "installed"); err != nil {
		return err
	}

	secret, err := newClusterDeploymentImportSecret(kubeClient, clusterName)
	if err != nil {
		return err
	}

	_, err = kubeClient.CoreV1().Secrets(clusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	_, err = clusterdeployments.Update(context.TODO(), clusterDeployment, metav1.UpdateOptions{})
	return err
}

func DeleteClusterDeployment(dynamicClient dynamic.Interface, clusterName string) error {
	clusterdeployments := dynamicClient.Resource(clusterdeploymentGVR).Namespace(clusterName)
	return clusterdeployments.Delete(context.TODO(), clusterName, metav1.DeleteOptions{})
}

func NewLable(key, value string) Label {
	return Label{
		key:   key,
		value: value,
	}
}

func NewAutoImportSecret(kubeClient kubernetes.Interface, clusterName string, mode ...operatorv1.InstallMode) (*corev1.Secret, error) {
	name := "e2e-auto-import-secret"
	if len(mode) != 0 && mode[0] == operatorv1.InstallModeHosted {
		name = "e2e-managed-auto-import-secret"
	}
	secret, err := kubeClient.CoreV1().Secrets(ocmNamespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: clusterName,
		},
		Data: map[string][]byte{
			"autoImportRetry": []byte("1"),
			"kubeconfig":      secret.Data["kubeconfig"],
		},
	}, nil
}

func NewAutoImportSecretWithToken(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, clusterName string) (*corev1.Secret, error) {
	server, token, err := getServerAndToken(kubeClient, dynamicClient, "managed-cluster-import-e2e-sa")
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: clusterName,
		},
		Data: map[string][]byte{
			"autoImportRetry": []byte("1"),
			"token":           token,
			"server":          server,
		},
	}, nil
}

func NewRestoreAutoImportSecret(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, clusterName string) (*corev1.Secret, error) {
	server, token, err := getServerAndToken(kubeClient, dynamicClient, "managed-cluster-import-e2e-sa")
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: clusterName,
			Labels: map[string]string{
				"cluster.open-cluster-management.io/restore-auto-import-secret": "true",
			},
		},
		Data: map[string][]byte{
			"autoImportRetry": []byte("0"),
			"token":           token,
			"server":          server,
		},
	}, nil
}

func NewInvalidAutoImportSecret(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, clusterName string) (*corev1.Secret, error) {
	server, token, err := getServerAndToken(kubeClient, dynamicClient, "managed-cluster-import-e2e-limited-sa")
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: clusterName,
		},
		Data: map[string][]byte{
			"autoImportRetry": []byte("3"),
			"token":           token,
			"server":          server,
		},
	}, nil
}

func NewExternalManagedKubeconfigSecret(kubeClient kubernetes.Interface, clusterName string) (*corev1.Secret, error) {

	name := "e2e-managed-auto-import-secret"
	secret, err := kubeClient.CoreV1().Secrets(ocmNamespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-managed-kubeconfig",
			Namespace: fmt.Sprintf("klusterlet-%s", clusterName),
		},
		Data: map[string][]byte{
			"kubeconfig": secret.Data["kubeconfig"],
		},
	}, nil
}

func NewManagedClient(kubeClient kubernetes.Interface) (kubernetes.Interface, error) {
	name := "e2e-managed-external-secret"
	secret, err := kubeClient.CoreV1().Secrets(ocmNamespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	kubeconfigRaw, ok := secret.Data["kubeconfig"]
	if !ok {
		return nil, fmt.Errorf("no kubeconfig data in the secret")
	}

	kubeconfig, err := clientcmd.Load(kubeconfigRaw)
	if err != nil {
		return nil, err
	}

	clientConfig, err := clientcmd.NewDefaultClientConfig(*kubeconfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func NewCSR(labels ...Label) *certificatesv1.CertificateSigningRequest {
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	csrb, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "redhat",
			Organization: []string{"RedHat"},
		},
		DNSNames:       []string{},
		EmailAddresses: []string{},
		IPAddresses:    []net.IP{},
	}, pk)
	if err != nil {
		panic(err)
	}

	csrLabels := map[string]string{}
	for _, label := range labels {
		csrLabels[label.key] = label.value
	}

	return &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "functional-test-csr",
			Labels: csrLabels,
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
			SignerName: certificatesv1.KubeAPIServerClientSignerName,
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrb}),
		},
	}
}

func ToImportResoruces(importYaml []byte) []*unstructured.Unstructured {
	yamls := [][]byte{}
	for _, yaml := range strings.Split(strings.Replace(string(importYaml), "\n---\n", "", 1), "\n---\n") {
		yamls = append(yamls, []byte(yaml))
	}

	unstructuredObjs := []*unstructured.Unstructured{}
	for _, raw := range yamls {
		jsonData, err := yaml.YAMLToJSON(raw)
		if err != nil {
			panic(err)
		}

		unstructuredObj := &unstructured.Unstructured{}
		_, _, err = unstructured.UnstructuredJSONScheme.Decode(jsonData, nil, unstructuredObj)
		if err != nil {
			panic(err)
		}

		unstructuredObjs = append(unstructuredObjs, unstructuredObj)
	}
	return unstructuredObjs
}

func ToKlusterlet(obj *unstructured.Unstructured) *operatorv1.Klusterlet {
	klusterlet := &operatorv1.Klusterlet{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, klusterlet); err != nil {
		panic(err)
	}

	return klusterlet
}

func newClusterDeploymentImportSecret(kubeClient kubernetes.Interface, clusterName string) (*corev1.Secret, error) {
	secret, err := kubeClient.CoreV1().Secrets(ocmNamespace).Get(context.TODO(), "e2e-auto-import-secret", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clusterdeployment-secret",
			Namespace: clusterName,
		},
		Data: map[string][]byte{
			"kubeconfig": secret.Data["kubeconfig"],
		},
	}, nil
}

func getServerAndToken(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, saName string) (server, token []byte, err error) {
	secrets, err := kubeClient.CoreV1().Secrets(ocmNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}

	var tokenSecret *corev1.Secret
	for _, ref := range secrets.Items {
		if strings.HasPrefix(ref.Name, saName) {
			tokenSecret, err = kubeClient.CoreV1().Secrets(ocmNamespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			if tokenSecret.Type == corev1.SecretTypeServiceAccountToken {
				break
			}
		}
	}
	if tokenSecret == nil {
		return nil, nil, fmt.Errorf("failed get the token of sa %s", saName)
	}

	infraConfig, err := dynamicClient.Resource(infrastructureGVR).Get(context.TODO(), "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	apiServer, found, err := unstructured.NestedString(infraConfig.Object, "status", "apiServerURL")
	if err != nil || !found {
		return nil, nil, fmt.Errorf("failed to get apiServerURL in infrastructures cluster: %v, %v", found, err)
	}

	return []byte(apiServer), tokenSecret.Data["token"], nil
}

func newClusterdeployment(clusterName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      clusterName,
				"namespace": clusterName,
			},
			"spec": map[string]interface{}{
				"baseDomain":  "fake-domain.red-chesterfield.com",
				"clusterName": clusterName,
				"installed":   false,
				"platform": map[string]interface{}{
					"aws": map[string]interface{}{
						"credentialsSecretRef": map[string]interface{}{
							"name": "fake-mycluster-aws-creds",
						},
						"region": "us-east-1",
					},
				},
				"provisioning": map[string]interface{}{
					"imageSetRef": map[string]interface{}{
						"name": "fake-hive-clusterimageset",
					},
					"installConfigSecretRef": map[string]interface{}{
						"name": "fake-hive-install-config",
					},
					"sshPrivateKeySecretRef": map[string]interface{}{
						"name": "fake-hive-ssh-private-key",
					},
				},
				"clusterMetadata": map[string]interface{}{
					"adminKubeconfigSecretRef": map[string]interface{}{
						"name": "clusterdeployment-secret",
					},
					"adminPasswordSecretRef": map[string]interface{}{
						"name": "clusterdeployment-secret",
					},
					"clusterID": "my-cluster-id",
					"infraID":   "my-infra-id",
				},
			},
		},
	}
}

func CreatePullSecret(kubeClient kubernetes.Interface, namespace, name string) error {
	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	Logf("create pull secret: get cluster error: %v, cluster: %s", err, secret)
	if errors.IsNotFound(err) {
		_, err = kubeClient.CoreV1().Secrets(namespace).Create(
			context.TODO(),
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
				Type: corev1.SecretTypeDockerConfigJson,
			},
			metav1.CreateOptions{},
		)
		return err
	}
	return nil
}

func CreateClusterWithImageRegistries(clusterClient clusterclient.Interface, name string,
	imageRegistries imageregistry.ImageRegistries) (*clusterv1.ManagedCluster, error) {
	imageRegistriesData, _ := json.Marshal(imageRegistries)
	return clusterClient.ClusterV1().ManagedClusters().Create(
		context.TODO(),
		&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Annotations: map[string]string{"open-cluster-management.io/image-registries": string(imageRegistriesData)},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: true,
			},
		},
		metav1.CreateOptions{},
	)
}
