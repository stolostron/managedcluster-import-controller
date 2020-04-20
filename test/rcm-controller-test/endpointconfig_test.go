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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

// getRcmController will list deployment with label app=rcm-controller, and return the first deployment
func getRcmController(clientHub kubernetes.Interface) (*appsv1.Deployment, error) {
	cl := clientHub.AppsV1().Deployments(metav1.NamespaceAll)
	listOptions := metav1.ListOptions{
		LabelSelector: "app=rcm-controller",
		Limit:         1,
	}
	rcmControllers, err := cl.List(listOptions)
	if err != nil {
		return nil, err
	}
	if len(rcmControllers.Items) == 0 {
		return nil, fmt.Errorf("Deployment with label 'app=rcm-controller' not found")
	}
	return &rcmControllers.Items[0], nil
}

// getEnv will return env variable value, and return empty if not found. If containerName is empty, will try all containers.
func getEnv(d *appsv1.Deployment, contanerName, name string) string {
	for _, c := range d.Spec.Template.Spec.Containers {
		if contanerName != "" && c.Name != contanerName {
			continue
		}
		for _, envVar := range c.Env {
			if envVar.Name == name {
				return envVar.Value
			}
		}
	}
	return ""
}

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

	It("Should use SHA if ENDPOINT_OPERATOR_SHA is set, and use tag when ENDPOINT_OPERATOR_SHA is empty", func() {
		By("Checking current rcm-controller deployment")
		dep, err := getRcmController(clientHub)
		Expect(err).To(BeNil())
		endpointOperatorSHA := getEnv(dep, "", "ENDPOINT_OPERATOR_SHA")
		imageTagPostfix := getEnv(dep, "", "IMAGE_TAG_POSTFIX")
		klog.V(1).Info("ENDPOINT_OPERATOR_SHA: " + endpointOperatorSHA)
		klog.V(1).Info("IMAGE_TAG_POSTFIX: " + imageTagPostfix)

		// checks of using SHA
		useSHAImageSecretCheck := "image: .*@" + endpointOperatorSHA
		useSHAEnvSecretCheck := "USE_SHA_MANIFEST[\\n\\r\\s]+value: \"true\""
		useSHAImageSyncsetCheck := "\"image\":\".*@" + endpointOperatorSHA + "\""
		useSHAEnvSyncsetCheck := "{\"name\":\"USE_SHA_MANIFEST\",\"value\":\"true\"}"
		if endpointOperatorSHA == "" {
			klog.V(1).Info("ENDPOINT_OPERATOR_SHA is empty")
			// checks of not using SHA
			useSHAImageSecretCheck = "image: [^:]*:.*" + imageTagPostfix
			useSHAEnvSecretCheck = "USE_SHA_MANIFEST[\\n\\r\\s]+value: \"false\""
			useSHAImageSyncsetCheck = "\"image\":\"[^:]*:.*" + imageTagPostfix + "\""
			useSHAEnvSyncsetCheck = "{\"name\":\"USE_SHA_MANIFEST\",\"value\":\"false\"}"
		} else {
			klog.V(1).Info("ENDPOINT_OPERATOR_SHA is not empty")
		}

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

		By("Validating import secret")
		importSecret := getSecretWithTimeout(clientHub, testNamespace+"-import", testNamespace, 15)
		Expect(importSecret.Data).ToNot(BeNil())
		importYaml := string(importSecret.Data["import.yaml"])
		Expect(importYaml).To(MatchRegexp(useSHAImageSecretCheck))
		Expect(importYaml).To(MatchRegexp(useSHAEnvSecretCheck))

		By("Validating syncset")
		syncset := getWithTimeout(clientHubDynamic, gvrSyncset, testNamespace+"-multicluster-endpoint", testNamespace, true, 15)
		resources, err := syncset.MarshalJSON()
		Expect(err).To(BeNil())
		Expect(string(resources)).To(MatchRegexp(useSHAImageSyncsetCheck))
		Expect(string(resources)).To(MatchRegexp(useSHAEnvSyncsetCheck))

	})

})
