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
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
)

func hasLabel(obj *unstructured.Unstructured, key string, value string) bool {
	if obj == nil {
		klog.Info("empty object")
		return false
	}
	labels := obj.GetLabels()
	klog.V(1).Info(fmt.Sprintf("labels: %#v", labels))
	for k, v := range labels {
		if k == key && v == value {
			return true
		}
	}
	return false
}

var _ = Describe("Clusterdeployment", func() {
	var (
		selectorsyncsetName string
		labelKey            string
		labelValue          string
	)
	BeforeEach(func() {
		selectorsyncsetName = "multicluster-endpoint"
		labelKey = "cluster-managed"
		labelValue = "true"
	})
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
	It("Should create selectorsyncset with selector", func() {
		By("Deleting selectorsyncset if needed")
		deleteIfExists(clientHubDynamic, gvrSelectorsyncset, selectorsyncsetName, "")
		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)
		By("Getting generated selectorsyncset")
		selectorsyncset := getWithTimeout(clientHubDynamic, gvrSelectorsyncset, selectorsyncsetName, "", true, 15)
		By("Checking selectors")
		matchLabels := getMap(selectorsyncset, "spec", "clusterDeploymentSelector", "matchLabels")
		Expect(matchLabels).NotTo(BeNil())
		Expect(func() bool {
			for k, v := range matchLabels {
				if k == labelKey {
					value, _ := v.(string)
					return value == labelValue
				}
			}
			return false
		}()).To(Equal(true))

	})
	It("Should contain no cluster-managed=true label if cluster not exist", func() {
		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Checking labels")
		Consistently(func() bool {
			clusterdeployment = getWithTimeout(clientHubDynamic, gvrClusterdeployment, testNamespace, testNamespace, true, 2)
			return hasLabel(clusterdeployment, labelKey, labelValue)
		}, 4, 1).Should(Equal(false))
	})
	It("Should add labels after cluster created", func() {
		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		By("Checking labels added")
		Eventually(func() bool {
			clusterdeployment = getWithTimeout(clientHubDynamic, gvrClusterdeployment, testNamespace, testNamespace, true, 3)
			return hasLabel(clusterdeployment, labelKey, labelValue)
		}, 10, 1).Should(Equal(true))

	})
	It("Should add labels if cluster exists", func() {
		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Checking labels added")
		Eventually(func() bool {
			clusterdeployment = getWithTimeout(clientHubDynamic, gvrClusterdeployment, testNamespace, testNamespace, true, 3)
			return hasLabel(clusterdeployment, labelKey, labelValue)
		}, 10, 1).Should(Equal(true))

	})
	It("Should remove labels if cluster is deleted", func() {
		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		By("Checking labels added")
		Eventually(func() bool {
			clusterdeployment = getWithTimeout(clientHubDynamic, gvrClusterdeployment, testNamespace, testNamespace, true, 3)
			return hasLabel(clusterdeployment, labelKey, labelValue)
		}, 10, 1).Should(Equal(true))

		By("Deleting clusterregistry")
		deleteIfExists(clientHubDynamic, gvrClusterregistry, testNamespace, testNamespace)
		By("Checking labels removed")
		Eventually(func() bool {
			clusterdeployment = getWithTimeout(clientHubDynamic, gvrClusterdeployment, testNamespace, testNamespace, true, 3)
			return hasLabel(clusterdeployment, labelKey, labelValue)
		}, 10, 1).Should(Equal(false))

	})
})
