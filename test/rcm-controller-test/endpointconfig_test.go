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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

var _ = Describe("Endpointconfig", func() {
	var (
		patchString      string // string of endpointconfig patch
		importYamlBefore string // regex for matching import.yaml in the import secret before updates
		importYamlAfter  string // regex for matching mport.yaml in the import secret  after updates
		syncsetBefore    string // regex for matching syncset (json format)
		syncsetAfter     string // regex for matching syncset (json format)
	)

	BeforeEach(func() {
		patchString = fmt.Sprintf(
			"[{\"op\":\"%s\",\"path\":\"%s\",\"value\":%t}]",
			"replace", "/spec/applicationManager/enabled", false,
		)
		importYamlBefore = "applicationManager:[\\n\\r\\s]+enabled: true"
		importYamlAfter = "applicationManager:[\\n\\r\\s]+enabled: false"
		syncsetBefore = "\"applicationManager\":{\"enabled\":true}"
		syncsetAfter = "\"applicationManager\":{\"enabled\":false}"
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

	It("Should update import secret", func() {
		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		By("Creating endpointconfig with empty registry and imagePullSecret")
		endpointconfig := newEndpointconfig(testNamespace, "", "")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		By("Checking import secret")
		importSecret := getSecretWithTimeout(clientHub, testNamespace+"-import", testNamespace, 15)
		Expect(importSecret.Data).ToNot(BeNil())
		importYaml := string(importSecret.Data["import.yaml"])
		Expect(importYaml).To(MatchRegexp(importYamlBefore))

		By("Update endpiontconfig")
		ns := clientHubDynamic.Resource(gvrEndpointconfig).Namespace(testNamespace)
		_, err := ns.Patch(testNamespace, types.JSONPatchType, []byte(patchString), metav1.PatchOptions{})
		Expect(err).To(BeNil())

		By("Verifying import secret updates")
		Eventually(func() string {
			importSecret = getSecretWithTimeout(clientHub, testNamespace+"-import", testNamespace, 15)
			if importSecret == nil {
				return ""
			}
			importYaml = string(importSecret.Data["import.yaml"])
			return importYaml
		}, 5, 1).Should(MatchRegexp(importYamlAfter))
	})

	It("Should update syncset", func() {
		By("Creating clusterregistry")
		cluster := newClusterregistry(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterregistry,
			cluster, testNamespace, testNamespace)

		By("Creating endpointconfig with empty registry and imagePullSecret")
		endpointconfig := newEndpointconfig(testNamespace, "", "")
		createNewUnstructured(clientHubDynamic, gvrEndpointconfig,
			endpointconfig, testNamespace, testNamespace)

		By("Creating clusterdeployment")
		clusterdeployment := newClusterdeployment(testNamespace)
		createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
			clusterdeployment, testNamespace, testNamespace)

		By("Checking syncset")
		syncset := getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, true, 15)
		resources, err := syncset.MarshalJSON()
		Expect(err).To(BeNil())
		Expect(string(resources)).To(MatchRegexp(syncsetBefore))

		By("Update endpiontconfig")
		ns := clientHubDynamic.Resource(gvrEndpointconfig).Namespace(testNamespace)
		_, err = ns.Patch(testNamespace, types.JSONPatchType, []byte(patchString), metav1.PatchOptions{})
		Expect(err).To(BeNil())

		By("Verifying import secret updates")
		Eventually(func() string {
			syncset := getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, true, 15)
			if syncset == nil {
				return ""
			}
			resources, err := syncset.MarshalJSON()
			if err != nil {
				klog.Error(err)
				return ""
			}
			return string(resources)
		}, 5, 1).Should(MatchRegexp(syncsetAfter))

	})
})
