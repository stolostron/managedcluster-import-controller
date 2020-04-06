// +build functional

package rcm_controller_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
)

// isOwner checks if obj is owned by owner, obj can either be unstructured or ObjectMeta
func isOwner(owner *unstructured.Unstructured, obj interface{}) bool {
	if obj == nil {
		return false
	}
	var owners []metav1.OwnerReference
	objMeta, ok := obj.(*metav1.ObjectMeta)
	if ok {
		owners = objMeta.GetOwnerReferences()
	} else {
		if objUnstructured, ok := obj.(*unstructured.Unstructured); ok {
			owners = objUnstructured.GetOwnerReferences()
		} else {
			klog.Error("Failed to get owners")
			return false
		}
	}

	for _, ownerRef := range owners {
		if _, ok := owner.Object["metadata"]; !ok {
			klog.Error("no meta")
			continue
		}
		meta, ok := owner.Object["metadata"].(map[string]interface{})
		if !ok || meta == nil {
			klog.Error("no meta map")
			continue
		}
		name, ok := meta["name"].(string)
		if !ok || name == "" {
			klog.Error("failed to get name")
			continue
		}
		if ownerRef.Kind == owner.Object["kind"] && ownerRef.Name == name {
			return true
		}
	}
	return false
}

var _ = Describe("Clusterregistry", func() {
	AfterEach(func() {
		By("Delete endpointconfig if exist")
		deleteIfExists(clientHubDynamic, gvrEndpointconfig, testNamespace, testNamespace)

		By("Delete cluster if exist")
		deleteIfExists(clientHubDynamic, gvrClusterregistry, testNamespace, testNamespace)

		By("Delete clusterdeployment")
		deleteIfExists(clientHubDynamic, gvrClusterdeployment, testNamespace, testNamespace)

		By("Delete other resources")
		deleteIfExists(clientHubDynamic, gvrServiceaccount, testNamespace+"-bootstrap-sa", testNamespace)
		deleteIfExists(clientHubDynamic, gvrSecret, testNamespace+"-import", testNamespace)
		deleteIfExists(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace)

	})
	It("Should create bootstrap serviceAccount", func() {
		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		var sa *corev1.ServiceAccount
		When("clusterregistry created, wait for bootstrap serviceAccount", func() {
			Eventually(func() error {
				var err error
				klog.V(1).Info("Wait bootstrap serviceAccount...")
				sa, err = clientHub.CoreV1().ServiceAccounts(testNamespace).Get(testNamespace+"-bootstrap-sa", metav1.GetOptions{})
				return err
			}, 10, 1).Should(BeNil())
			klog.V(1).Info("bootstrap serviceAccount created")
		})
		By("Checking ownerRef", func() {
			Expect(isOwner(cluster, &sa.ObjectMeta)).To(Equal(true))
		})
	})

	It("Should add ownerRef to created endpointconfig", func() {
		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		endpointconfig := newEndpointconfig(testNamespace, "", "")
		By("Creating endpointconfig")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		When("endpointconfig created, wait for ownerRef", func() {
			Eventually(func() bool {
				klog.V(1).Info("Wait ownerRef ...")
				namespace := clientHubDynamic.Resource(gvrEndpointconfig).Namespace(testNamespace)
				endpointconfig, err := namespace.Get(testNamespace, metav1.GetOptions{})
				if err != nil {
					return false
				}
				return isOwner(cluster, endpointconfig)
			}, 30, 1).Should(BeTrue())
			klog.V(1).Info("bootstrap serviceAccount created")
		})
	})

	It("Should add ownerRef to existing endpointconfig", func() {
		endpointconfig := newEndpointconfig(testNamespace, "", "")
		By("Creating endpointconfig")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		When("endpointconfig created, wait for ownerRef", func() {
			Eventually(func() bool {
				klog.V(1).Info("Wait ownerRef ...")
				namespace := clientHubDynamic.Resource(gvrEndpointconfig).Namespace(testNamespace)
				endpointconfig, err := namespace.Get(testNamespace, metav1.GetOptions{})
				if err != nil {
					return false
				}
				return isOwner(cluster, endpointconfig)
			}, 30, 1).Should(BeTrue())
			klog.V(1).Info("bootstrap serviceAccount created")
		})
	})

	It("Should create import secret if endpointconfig exists", func() {
		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		endpointconfig := newEndpointconfig(testNamespace, "", "")
		By("Creating endpointconfig")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		var importSecret *corev1.Secret
		When("clusterregistry created, wait for import secret", func() {
			Eventually(func() error {
				var err error
				klog.V(1).Info("Wait import secret...")
				importSecret, err = clientHub.CoreV1().Secrets(testNamespace).Get(testNamespace+"-import", metav1.GetOptions{})
				return err
			}, 10, 1).Should(BeNil())
			klog.V(1).Info("import secret created")
		})
		By("Checking ownerRef", func() {
			Expect(isOwner(cluster, &importSecret.ObjectMeta)).To(Equal(true))
		})
	})

	It("Should create syncset if clusterdeployment and endpointconfig exist", func() {
		endpointconfig := newEndpointconfig(testNamespace, "", "")
		By("Creating endpointconfig")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		By("Waiting syncset")
		syncset := getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, true, 30)

		By("Checking ownerRef")
		Expect(isOwner(cluster, syncset)).To(Equal(true))

	})
	It("Should create syncset if clusterdeployment and endpointconfig exist (different order)", func() {
		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		endpointconfig := newEndpointconfig(testNamespace, "quay.io/open-cluster-management", "fake-secret")
		By("Creating endpointconfig")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Waiting syncset")
		syncset := getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, true, 30)

		By("Checking ownerRef")
		Expect(isOwner(cluster, syncset)).To(Equal(true))

	})

	It("Should remove endpointconfig, import secret, syncset, serviceAccount when cluster is deleted", func() {

		By("Checking no endpointconfig, import secret, syncset, and serviceAccount")
		getWithTimeout(clientHubDynamic, gvrEndpointconfig, testNamespace, testNamespace, false, 5)
		getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, false, 5)
		getWithTimeout(clientHubDynamic, gvrSecret, testNamespace+"-import", testNamespace, false, 5)
		getWithTimeout(clientHubDynamic, gvrServiceaccount, testNamespace+"-bootstrap-sa", testNamespace, false, 5)

		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		endpointconfig := newEndpointconfig(testNamespace, "", "")
		By("Creating endpointconfig")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Waiting import secret, syncset, and service account")
		getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, true, 15)
		getWithTimeout(clientHubDynamic, gvrSecret, testNamespace+"-import", testNamespace, true, 15)
		getWithTimeout(clientHubDynamic, gvrServiceaccount, testNamespace+"-bootstrap-sa", testNamespace, true, 5)

		By("By deleting clusterregistry")
		deleteIfExists(clientHubDynamic, gvrClusterregistry, testNamespace, testNamespace)

		By("Waiting for deletion of import secret, syncset, and service account")
		getWithTimeout(clientHubDynamic, gvrEndpointconfig, testNamespace, testNamespace, false, 15)
		getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, false, 15)
		getWithTimeout(clientHubDynamic, gvrSecret, testNamespace+"-import", testNamespace, false, 15)
		getWithTimeout(clientHubDynamic, gvrServiceaccount, testNamespace+"-bootstrap-sa", testNamespace, false, 5)

	})

})
