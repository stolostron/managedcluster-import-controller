// Copyright (c) 2020 Red Hat, Inc.

// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	libgooptions "github.com/open-cluster-management/library-e2e-go/pkg/options"
	libgoapplier "github.com/open-cluster-management/library-go/pkg/applier"
	libgoclient "github.com/open-cluster-management/library-go/pkg/client"
	libgounstructured "github.com/open-cluster-management/library-go/pkg/unstructured"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const MANAGEDCLUSTERS_KUBECONFIGS_DIR = "test/e2e/resources/clusters"
const HUBCLUSTER_KUBECONFIG_DIR = "test/e2e/resources/hubs"

const (
	MANUAL_IMPORT_IMAGE_PULL_SECRET = "image-pull-secret"
	MANUAL_IMPORT_CLUSTER_SCENARIO  = "manual-import"
)

var _ = Describe("Manual import cluster", func() {

	var err error
	var managedClustersForManualImport map[string]string
	var hubClientClient client.Client
	var clusterClientClient client.Client
	var clientHub kubernetes.Interface
	var clientHubDynamic dynamic.Interface
	var clientHubClientset clientset.Interface
	var templateProcessor *libgoapplier.TemplateProcessor
	var hubApplier *libgoapplier.Applier
	var clusterApplier *libgoapplier.Applier

	BeforeEach(func() {
		managedClustersForManualImport, err = libgooptions.GetManagedClusterKubeConfigs(testOptions.ManagedClusters.ConfigDir, MANUAL_IMPORT_CLUSTER_SCENARIO)
		Expect(err).To(BeNil())
		if len(managedClustersForManualImport) == 0 {
			Skip("Manual import not executed because no managed cluster defined for import")
		}
		SetDefaultEventuallyTimeout(10 * time.Minute)
		SetDefaultEventuallyPollingInterval(10 * time.Second)
		kubeconfig := libgooptions.GetHubKubeConfig(testOptions.Hub.ConfigDir)
		clientHub, err = libgoclient.NewDefaultKubeClient(kubeconfig)
		Expect(err).To(BeNil())
		clientHubDynamic, err = libgoclient.NewDefaultKubeClientDynamic(kubeconfig)
		Expect(err).To(BeNil())
		clientHubClientset, err = libgoclient.NewDefaultKubeClientAPIExtension(kubeconfig)
		Expect(err).To(BeNil())
		yamlReader := libgoapplier.NewYamlFileReader("resources")
		templateProcessor, err = libgoapplier.NewTemplateProcessor(yamlReader, &libgoapplier.Options{})
		Expect(err).To(BeNil())
		hubClientClient, err = libgoclient.NewDefaultClient(kubeconfig, client.Options{})
		Expect(err).To(BeNil())
		hubApplier, err = libgoapplier.NewApplier(templateProcessor, hubClientClient, nil, nil, nil)
		Expect(err).To(BeNil())
	})

	It("Given a list of clusters to import (cluster/g0/manual-import-service-resources)", func() {
		for clusterName, clusterKubeconfig := range managedClustersForManualImport {
			klog.V(1).Infof("========================= Test cluster import cluster %s ===============================", clusterName)
			clusterClientClient, err = libgoclient.NewDefaultClient(clusterKubeconfig, client.Options{})
			Expect(err).To(BeNil())
			clusterApplier, err = libgoapplier.NewApplier(templateProcessor, clusterClientClient, nil, nil, libgoapplier.DefaultKubernetesMerger)
			Expect(err).To(BeNil())
			Eventually(func() error {
				klog.V(1).Info("Check CRDs")
				return libgoclient.HaveCRDs(clientHubClientset,
					[]string{
						"managedclusters.cluster.open-cluster-management.io",
						"manifestworks.work.open-cluster-management.io",
						"clusterdeployments.hive.openshift.io",
						"syncsets.hive.openshift.io",
					})
			}).Should(BeNil())
			// Eventually(func() error {
			// 	return libgoclient.HaveDeploymentsInNamespace(testOptions.HubCluster, testOptions.KubeConfig,
			// 		"open-cluster-management",
			// 		[]string{"cert-manager-webhook",
			// 			"console-header",
			// 			"etcd-operator",
			// 			"mcm-apiserver",
			// 			"mcm-controller",
			// 			"mcm-webhook",
			// 			"multicluster-operators-application",
			// 			"multicluster-operators-hub-subscription",
			// 			"multicluster-operators-standalone-subscription",
			// 			"multiclusterhub-repo",
			// 			"multiclusterhub-operator",
			// 			"rcm-controller",
			// 			"search-operator",
			// 			"mcm-controller",
			// 		})
			// }).Should(BeNil())

			By("creating the namespace in which the cluster will be imported", func() {
				//Create the cluster NS on master
				klog.V(1).Info("Creating the namespace in which the cluster will be imported")
				namespaces := clientHub.CoreV1().Namespaces()
				_, err := namespaces.Get(context.TODO(), clusterName, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						Expect(namespaces.Create(context.TODO(), &corev1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								Name: clusterName,
							},
						}, metav1.CreateOptions{})).NotTo(BeNil())
						Expect(namespaces.Get(context.TODO(), clusterName, metav1.GetOptions{})).NotTo(BeNil())
					} else {
						Fail(err.Error())
					}
				}
			})

			By("creating the managedCluster", func() {
				klog.V(1).Info("Creating the managedCluster")
				values := struct {
					ManagedClusterName string
				}{
					ManagedClusterName: clusterName,
				}
				names, err := templateProcessor.AssetNamesInPath("./managedcluster_cr.yaml", nil, false)
				Expect(err).To(BeNil())
				klog.V(1).Infof("names: %s", names)
				Expect(hubApplier.CreateOrUpdateAsset("managedcluster_cr.yaml", values)).To(BeNil())
			})

			var importSecret *corev1.Secret
			When("the managedcluster is created, wait for import secret", func() {
				var err error
				Eventually(func() error {
					klog.V(1).Infof("Wait import secret %s...", clusterName)
					importSecret, err = clientHub.CoreV1().Secrets(clusterName).Get(context.TODO(), clusterName+"-import", metav1.GetOptions{})
					if err != nil {
						klog.V(1).Info(err)
					}
					return err
				}).Should(BeNil())
				klog.V(1).Infof("bootstrap import secret %s created", clusterName+"-import")
			})

			By("Launching the manual import", func() {
				klog.V(1).Info("Apply the crds.yaml")
				klog.V(5).Infof("importSecret.Data[crds.yaml]: %s\n", importSecret.Data["crds.yaml"])
				Expect(clusterApplier.CreateOrUpdateAssets(importSecret.Data["crds.yaml"], nil, "---")).NotTo(HaveOccurred())
				klog.V(1).Info("Apply the import.yaml")
				klog.V(5).Infof("importSecret.Data[import.yaml]: %s\n", importSecret.Data["import.yaml"])
				Expect(clusterApplier.CreateOrUpdateAssets(importSecret.Data["import.yaml"], nil, "---")).NotTo(HaveOccurred())
			})

			When("Import launched, wait for cluster ready", func() {
				gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
				Eventually(func() error {
					klog.V(1).Infof("Wait %s comes online...", clusterName)
					managedCluster, err := clientHubDynamic.Resource(gvr).Get(context.TODO(), clusterName, metav1.GetOptions{})
					if err == nil {
						var condition map[string]interface{}
						condition, err = libgounstructured.GetCondition(managedCluster, "ManagedClusterConditionAvailable")
						if err != nil {
							return err
						}
						if v, ok := condition["status"]; ok && v == metav1.ConditionTrue {
							return nil
						}
					} else {
						klog.V(1).Info(err)
					}
					return err
				}).Should(BeNil())
				klog.V(1).Info("Cluster imported")
			})
			By(fmt.Sprintf("Detaching the %s CR on the hub", clusterName), func() {
				klog.V(1).Infof("Detaching the %s CR on the hub", clusterName)
				gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
				Expect(clientHubDynamic.Resource(gvr).Delete(context.TODO(), clusterName, metav1.DeleteOptions{})).Should(BeNil())

			})

			When("the deletion of the cluster is requested, wait for the effective deletion", func() {
				By(fmt.Sprintf("Checking the deletion of the %s CR on the hub", clusterName), func() {
					klog.V(1).Infof("Checking the deletion of the %s CR on the hub", clusterName)
					gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s CR deletion...", clusterName)
						_, err := clientHubDynamic.Resource(gvr).Get(context.TODO(), clusterName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
					klog.V(1).Infof("%s CR deleted", clusterName)
				})
			})

			When("the deletion of the cluster is done, wait for the namespace deletion", func() {
				By(fmt.Sprintf("Checking the deletion of the %s namespace on the hub", clusterName), func() {
					klog.V(1).Infof("Checking the deletion of the %s namespace on the hub", clusterName)
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s CR deletion...", clusterName)
						_, err := clientHub.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
					klog.V(1).Infof("%s namespace deleted", clusterName)
				})
			})
		}

	})

})
