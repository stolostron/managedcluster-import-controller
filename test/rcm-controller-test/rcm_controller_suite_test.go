// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

// +build functional

package rcm_controller_test

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var (
	testNamespace        string
	clientHub            kubernetes.Interface
	clientHubDynamic     dynamic.Interface
	gvrClusterregistry   schema.GroupVersionResource
	gvrEndpointconfig    schema.GroupVersionResource
	gvrClusterdeployment schema.GroupVersionResource
	gvrSyncset           schema.GroupVersionResource
	gvrSelectorsyncset   schema.GroupVersionResource
	gvrSecret            schema.GroupVersionResource
	gvrServiceaccount    schema.GroupVersionResource
	optionsFile          string
	baseDomain           string
	kubeadminUser        string
	kubeadminCredential  string
	kubeconfig           string

	defaultImageRegistry       string
	defaultImagePullSecretName string
)

// getMap iterates obj items by using the path, and returns a map if able to follow the path. returns nil if not able to find the map
func getMap(obj *unstructured.Unstructured, path ...string) map[string]interface{} {
	if obj == nil {
		return nil
	}
	curr := obj.Object
	for _, k := range path {
		if next, ok := curr[k]; !ok {
			return nil
		} else if curr, ok = next.(map[string]interface{}); !ok {
			return nil
		}
	}
	return curr
}

func newClusterregistry(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "clusterregistry.k8s.io/v1alpha1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
				"labels": map[string]string{
					"cloud":  "auto-detect",
					"name":   name,
					"vendor": "auto-detect"},
			},
			"spec": map[string]interface{}{},
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
			},
		},
	}
}

func newEndpointconfig(name, registry, imagePullSecret string) *unstructured.Unstructured {
	res := unstructured.Unstructured{}
	r, err := os.Open("../../deploy/crds/multicloud_v1alpha1_endpointconfig_cr.yaml")
	Expect(err).To(BeNil())
	err = yaml.NewYAMLOrJSONDecoder(r, 1024).Decode(&res)
	Expect(err).To(BeNil())
	// update fields
	meta := getMap(&res, "metadata")
	Expect(meta).NotTo(BeNil(), "metadata should not be nil")
	meta["name"] = name
	meta["namespace"] = name

	spec := getMap(&res, "spec")
	Expect(spec).NotTo(BeNil(), "spec should not be nil")
	spec["clusterName"] = name
	spec["clusterNamespace"] = name
	spec["imageRegistry"] = registry
	spec["imagePullSecret"] = imagePullSecret

	return &res
}

// deleteIfExists deletes resources by using gvr & name & namespace, will wait for deletion to complete by using eventually
func deleteIfExists(clientHubDynamic dynamic.Interface, gvr schema.GroupVersionResource, name, namespace string) {
	ns := clientHubDynamic.Resource(gvr).Namespace(namespace)
	if _, err := ns.Get(name, metav1.GetOptions{}); err != nil {
		Expect(errors.IsNotFound(err)).To(Equal(true))
		return
	}
	Expect(func() error {
		// possibly already got deleted
		err := ns.Delete(name, nil)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}()).To(BeNil())

	By("Wait for deletion")
	Eventually(func() error {
		var err error
		_, err = ns.Get(name, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			return fmt.Errorf("found object %s in namespace %s after deletion", name, namespace)
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
	ns := clientHubDynamic.Resource(gvr).Namespace(namespace)
	Expect(ns.Create(obj, metav1.CreateOptions{})).NotTo(BeNil())
	Expect(ns.Get(name, metav1.GetOptions{})).NotTo(BeNil())
}

// getWithTimeout keeps polling to get the object for timeout seconds until wantFound is met (true for found, false for not found)
func getWithTimeout(
	clientHubDynamic dynamic.Interface,
	gvr schema.GroupVersionResource,
	name, namespace string,
	wantFound bool,
	timeout int,
) *unstructured.Unstructured {
	if timeout < 1 {
		timeout = 1
	}
	var obj *unstructured.Unstructured

	Eventually(func() error {
		var err error
		namespace := clientHubDynamic.Resource(gvr).Namespace(namespace)
		obj, err = namespace.Get(name, metav1.GetOptions{})
		if wantFound && err != nil {
			return err
		}
		if !wantFound && err == nil {
			return fmt.Errorf("expected to return IsNotFound error")
		}
		if !wantFound && err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}, timeout, 1).Should(BeNil())
	if wantFound {
		return obj
	}
	return nil

}

// getSecretWithTimeout keeps polling to get secret for timeout seconds
func getSecretWithTimeout(
	clientHub kubernetes.Interface,
	name, namespace string,
	timeout int,
) *corev1.Secret {
	if timeout < 1 {
		timeout = 1
	}

	var res *corev1.Secret
	Eventually(func() error {
		var err error
		res, err = clientHub.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
		return err
	}, timeout, 1).Should(BeNil())

	return res
}

func init() {
	klog.SetOutput(GinkgoWriter)
	klog.InitFlags(nil)

	flag.StringVar(&kubeadminUser, "kubeadmin-user", "kubeadmin", "Provide the kubeadmin credential for the cluster under test (e.g. -kubeadmin-user=\"xxxxx\").")
	flag.StringVar(&kubeadminCredential, "kubeadmin-credential", "",
		"Provide the kubeadmin credential for the cluster under test (e.g. -kubeadmin-credential=\"xxxxx-xxxxx-xxxxx-xxxxx\").")
	flag.StringVar(&baseDomain, "base-domain", "", "Provide the base domain for the cluster under test (e.g. -base-domain=\"demo.red-chesterfield.com\").")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Location of the kubeconfig to use; defaults to KUBECONFIG if not set")

	flag.StringVar(&optionsFile, "options", "", "Location of an \"options.yaml\" file to provide input for various tests")

}

var _ = BeforeSuite(func() {
	By("Setup Hub client")
	gvrClusterregistry = schema.GroupVersionResource{Group: "clusterregistry.k8s.io", Version: "v1alpha1", Resource: "clusters"}
	gvrEndpointconfig = schema.GroupVersionResource{Group: "multicloud.ibm.com", Version: "v1alpha1", Resource: "endpointconfigs"}
	gvrClusterdeployment = schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"}
	gvrSyncset = schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "syncsets"}
	gvrSelectorsyncset = schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "selectorsyncsets"}
	gvrSecret = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	gvrServiceaccount = schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}
	clientHub = NewKubeClient("", "", "")
	clientHubDynamic = NewKubeClientDynamic("", "", "")
	defaultImageRegistry = "quay.io/open-cluster-management"
	defaultImagePullSecretName = "multiclusterhub-operator-pull-secret"
	testNamespace = "rcm-fuctional-tests"
	By("Create Namesapce if needed")
	namespaces := clientHub.CoreV1().Namespaces()
	if _, err := namespaces.Get(testNamespace, metav1.GetOptions{}); err != nil && errors.IsNotFound(err) {
		Expect(namespaces.Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		})).NotTo(BeNil())
	}
	Expect(namespaces.Get(testNamespace, metav1.GetOptions{})).NotTo(BeNil())
})

func TestRcmController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RcmController Suite")
}

func NewKubeClient(url, kubeconfig, context string) kubernetes.Interface {
	klog.V(5).Infof("Create kubeclient for url %s using kubeconfig path %s\n", url, kubeconfig)
	config, err := LoadConfig(url, kubeconfig, context)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func NewKubeClientDynamic(url, kubeconfig, context string) dynamic.Interface {
	klog.V(5).Infof("Create kubeclient dynamic for url %s using kubeconfig path %s\n", url, kubeconfig)
	config, err := LoadConfig(url, kubeconfig, context)
	if err != nil {
		panic(err)
	}

	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func LoadConfig(url, kubeconfig, context string) (*rest.Config, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	klog.V(5).Infof("Kubeconfig path %s\n", kubeconfig)
	// If we have an explicit indication of where the kubernetes config lives, read that.
	if kubeconfig != "" {
		if context == "" {
			return clientcmd.BuildConfigFromFlags(url, kubeconfig)
		}
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
			&clientcmd.ConfigOverrides{
				CurrentContext: context,
			}).ClientConfig()
	}
	// If not, try the in-cluster config.
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}
	// If no in-cluster config, try the default location in the user's home directory.
	if usr, err := user.Current(); err == nil {
		klog.V(5).Infof("clientcmd.BuildConfigFromFlags for url %s using %s\n", url, filepath.Join(usr.HomeDir, ".kube", "config"))
		if c, err := clientcmd.BuildConfigFromFlags("", filepath.Join(usr.HomeDir, ".kube", "config")); err == nil {
			return c, nil
		}
	}

	return nil, fmt.Errorf("could not create a valid kubeconfig")

}
