// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// +build functional

package functional

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	libgoclient "github.com/open-cluster-management/library-go/pkg/client"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

var (
	testNamespace        string
	clientHub            kubernetes.Interface
	clientHubDynamic     dynamic.Interface
	gvrClusterdeployment schema.GroupVersionResource
	gvrSecret            schema.GroupVersionResource
	gvrServiceaccount    schema.GroupVersionResource
	gvrManagedcluster    schema.GroupVersionResource
	gvrManifestwork      schema.GroupVersionResource
	gvrCSR               schema.GroupVersionResource

	optionsFile string
	baseDomain  string
	kubeconfig  string

	defaultImageRegistry       string
	defaultImagePullSecretName string
)

func newManagedcluster(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
				"managedClusterClientConfigs": []map[string]interface{}{
					{
						"url":      "fake-cluster-url",
						"caBundle": []byte("fake-CA"),
					},
				},
			},
		},
	}
}

func newClusterdeployment(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"spec": map[string]interface{}{
				"baseDomain":  "fake-domain.red-chesterfield.com",
				"clusterName": name,
				"installed":   true,
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

func newAutoImportSecretWithKubeConfig(clusterName string) (*corev1.Secret, error) {
	dir, _ := os.Getwd()
	klog.V(5).Infof("Current Directory: %s", dir)
	kubeconfig, err := ioutil.ReadFile("../../kind_kubeconfig_internal_mc.yaml")
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: clusterName,
		},
		StringData: map[string]string{
			"autoImportRetry": "5",
			"kubeconfig":      string(kubeconfig),
		},
	}, nil
}

func newClusterDeploymentSecretWithKubeConfig(clusterName string) (*corev1.Secret, error) {
	dir, _ := os.Getwd()
	klog.V(5).Infof("Current Directory: %s", dir)
	kubeconfig, err := ioutil.ReadFile("../../kind_kubeconfig_internal_mc.yaml")
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clusterdeployment-secret",
			Namespace: clusterName,
		},
		StringData: map[string]string{
			"kubeconfig": string(kubeconfig),
		},
	}, nil
}

func newAutoImportSecretWithToken(clusterName string) (*corev1.Secret, error) {
	token := os.Getenv("MANAGED_CLUSTER_TOKEN")
	apiURL := os.Getenv("MANAGED_CLUSTER_API_SERVER_URL")
	if len(token) == 0 || len(apiURL) == 0 {
		return nil, fmt.Errorf(
			`MANAGED_CLUSTER_TOKEN and/or 
			MANAGED_CLUSTER_API_SERVER_URL are not set for cluster: %s`,
			clusterName)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: clusterName,
		},
		StringData: map[string]string{
			"autoImportRetry": "5",
			"token":           token,
			"server":          apiURL,
		},
	}, nil
}

// deleteIfExists deletes resources by using gvr & name & namespace, will wait for deletion to complete by using eventually
func deleteIfExists(clientHubDynamic dynamic.Interface, gvr schema.GroupVersionResource, name, namespace string) {
	ns := clientHubDynamic.Resource(gvr).Namespace(namespace)
	if _, err := ns.Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
		Expect(errors.IsNotFound(err)).To(Equal(true))
		return
	}
	Expect(func() error {
		// possibly already got deleted
		err := ns.Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}()).To(BeNil())

	By("Wait for deletion")
	Eventually(func() error {
		var err error
		_, err = ns.Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			return fmt.Errorf("found object %s in namespace %s after deletion", name, namespace)
		}
		return nil
	}, 10, 1).Should(BeNil())
}

// deleteIfExistsClusterScoped deletes resources by using gvr & name, will wait for deletion to complete by using eventually
func deleteIfExistsClusterScoped(clientHubDynamic dynamic.Interface, gvr schema.GroupVersionResource, name string) {
	ns := clientHubDynamic.Resource(gvr)
	if _, err := ns.Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
		Expect(errors.IsNotFound(err)).To(Equal(true))
		return
	}
	Expect(func() error {
		// possibly already got deleted
		err := ns.Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}()).To(BeNil())

	By("Wait for deletion")
	Eventually(func() error {
		var err error
		_, err = ns.Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			return fmt.Errorf("found non scoped object %s after deletion", name)
		}
		return nil
	}, 10, 1).Should(BeNil())
}

// createNewUnstructured creates resources by using gvr & obj, will get object after create.
func createNewUnstructured(
	clientHubDynamic dynamic.Interface,
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
	name, namespace string,
) {
	klog.V(5).Infof("Creation Unstructured of %s %s/%s", gvr, name, namespace)
	ns := clientHubDynamic.Resource(gvr).Namespace(namespace)
	klog.V(5).Infof("ns client created for %s %s/%s created", gvr, name, namespace)
	Expect(ns.Create(context.TODO(), obj, metav1.CreateOptions{})).NotTo(BeNil())
	klog.V(5).Infof("Check if Unstructured %s %s/%s created", gvr, name, namespace)
	Expect(ns.Get(context.TODO(), name, metav1.GetOptions{})).NotTo(BeNil())
	klog.V(5).Infof("Unstructured %s %s/%s created", gvr, name, namespace)
}

// createNewUnstructuredClusterScoped creates resources by using gvr & obj, will get object after create.
func createNewUnstructuredClusterScoped(
	clientHubDynamic dynamic.Interface,
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
	name string,
) {
	klog.V(5).Infof("Creation Unstructured of %s %s", gvr, name)
	s := clientHubDynamic.Resource(gvr)
	klog.V(5).Infof("ns created for %s %s created", gvr, name)
	Expect(s.Create(context.TODO(), obj, metav1.CreateOptions{})).NotTo(BeNil())
	klog.V(5).Infof("Check if Unstructured %s %s created", gvr, name)
	Expect(s.Get(context.TODO(), name, metav1.GetOptions{})).NotTo(BeNil())
	klog.V(5).Infof("Unstructured %s %s created", gvr, name)
}

func init() {
	klog.SetOutput(GinkgoWriter)
	klog.InitFlags(nil)

	flag.StringVar(&baseDomain, "base-domain", "", "Provide the base domain for the cluster under test (e.g. -base-domain=\"demo.red-chesterfield.com\").")

	flag.StringVar(&optionsFile, "options", "", "Location of an \"options.yaml\" file to provide input for various tests")

}

var _ = BeforeSuite(func() {
	By("Setup Hub client")
	gvrClusterdeployment = schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"}
	gvrSecret = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	gvrServiceaccount = schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}
	gvrManagedcluster = schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
	gvrManifestwork = schema.GroupVersionResource{Group: "work.open-cluster-management.io", Version: "v1", Resource: "manifestworks"}
	gvrCSR = schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1beta1", Resource: "certificatesigningrequests"}

	var err error
	clientHub, err = libgoclient.NewDefaultKubeClient("")
	Expect(err).To(BeNil())
	clientHubDynamic, err = libgoclient.NewDefaultKubeClientDynamic("")
	Expect(err).To(BeNil())

	defaultImageRegistry = "quay.io/open-cluster-management"
	defaultImagePullSecretName = "multiclusterhub-operator-pull-secret"
})

func TestRcmController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RcmController Suite")
}
