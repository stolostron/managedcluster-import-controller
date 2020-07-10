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
	libgocrdv1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1/crd"
	libgounstructuredv1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1/unstructured"
	libgoapplier "github.com/open-cluster-management/library-go/pkg/applier"
	libgoclient "github.com/open-cluster-management/library-go/pkg/client"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	manualImportClusterScenario              = "manual-import"
	openClusterManagementAgentNamespace      = "open-cluster-management-agent"
	openClusterManagementAgentAddonNamespace = "open-cluster-management-agent-addon"
	klusterletCRDName                        = "klusterlet"
	manifestWorkNamePostfix                  = "-klusterlet"
	manifestWorkCRDSPostfix                  = "-crds"
)

var _ = Describe("Manual import cluster", func() {

	var err error
	var managedClustersForManualImport map[string]string
	var hubClientClient client.Client
	var clusterClientClient client.Client
	var clientCluster kubernetes.Interface
	var clientClusterDynamic dynamic.Interface
	var clientHub kubernetes.Interface
	var clientHubDynamic dynamic.Interface
	var clientHubClientset clientset.Interface
	var templateProcessor *libgoapplier.TemplateProcessor
	var hubApplier *libgoapplier.Applier
	var clusterApplier *libgoapplier.Applier

	BeforeEach(func() {
		managedClustersForManualImport, err = libgooptions.GetManagedClusterKubeConfigs(testOptions.ManagedClusters.ConfigDir, manualImportClusterScenario)
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
			clientCluster, err = libgoclient.NewDefaultKubeClient(clusterKubeconfig)
			Expect(err).To(BeNil())
			clientClusterDynamic, err = libgoclient.NewDefaultKubeClientDynamic(clusterKubeconfig)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				klog.V(1).Info("Check CRDs")
				has, _, _ := libgocrdv1.HasCRDs(clientHubClientset,
					[]string{
						"managedclusters.cluster.open-cluster-management.io",
						"manifestworks.work.open-cluster-management.io",
						"clusterdeployments.hive.openshift.io",
						"syncsets.hive.openshift.io",
					})
				return has
			}).Should(BeTrue())
			// Eventually(func() error {
			// 	return libgodeploymentv1.HasDeploymentsInNamespace(testOptions.HubCluster, testOptions.KubeConfig,
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
				//Wait 2 sec to make sure the CRDs are effective. The UI does the same.
				time.Sleep(2 * time.Second)
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
						condition, err = libgounstructuredv1.GetConditionByType(managedCluster, "ManagedClusterConditionAvailable")
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

			When("Cluster ready, wait manifestWorks to be applied", func() {
				manifestWorkCRDsName := clusterName + manifestWorkNamePostfix + manifestWorkCRDSPostfix
				By(fmt.Sprintf("Checking manfestwork %s to be applied", manifestWorkCRDsName), func() {
					klog.V(1).Infof("Checking manfestwork %s to be applied", manifestWorkCRDsName)
					Eventually(func() error {
						klog.V(1).Infof("Wait manifestwork %s to be applied...", manifestWorkCRDsName)
						gvr := schema.GroupVersionResource{Group: "work.open-cluster-management.io", Version: "v1", Resource: "manifestworks"}
						mwcrd, err := clientHubDynamic.Resource(gvr).Namespace(clusterName).Get(context.TODO(), manifestWorkCRDsName, metav1.GetOptions{})
						if err == nil {
							var condition map[string]interface{}
							condition, err = libgounstructuredv1.GetConditionByType(mwcrd, "Applied")
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
					klog.V(1).Infof("manifestwork %s applied", manifestWorkCRDsName)
				})

				manifestWorkYAMLsName := clusterName + manifestWorkNamePostfix
				By(fmt.Sprintf("Checking manfestwork %s to be applied", manifestWorkYAMLsName), func() {
					klog.V(1).Infof("Checking manfestwork %s to be applied", manifestWorkYAMLsName)
					Eventually(func() error {
						klog.V(1).Infof("Wait manifestwork %s to be applied...", manifestWorkYAMLsName)
						gvr := schema.GroupVersionResource{Group: "work.open-cluster-management.io", Version: "v1", Resource: "manifestworks"}
						mwyaml, err := clientHubDynamic.Resource(gvr).Namespace(clusterName).Get(context.TODO(), manifestWorkYAMLsName, metav1.GetOptions{})
						if err == nil {
							var condition map[string]interface{}
							condition, err = libgounstructuredv1.GetConditionByType(mwyaml, "Applied")
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
					klog.V(1).Infof("manifestwork %s applied", manifestWorkYAMLsName)
				})
			})

			By(fmt.Sprintf("Detaching the %s CR on the hub", clusterName), func() {
				klog.V(1).Infof("Detaching the %s CR on the hub", clusterName)
				gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
				Expect(clientHubDynamic.Resource(gvr).Delete(context.TODO(), clusterName, metav1.DeleteOptions{})).Should(BeNil())

			})

			When("the deletion of the cluster is requested, wait for the effective deletion", func() {
				By(fmt.Sprintf("Checking the deletion of the %s managedCluster on the hub", clusterName), func() {
					klog.V(1).Infof("Checking the deletion of the %s managedCluster on the hub", clusterName)
					gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s managedCluster deletion...", clusterName)
						_, err := clientHubDynamic.Resource(gvr).Get(context.TODO(), clusterName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
					klog.V(1).Infof("%s managedCluster deleted", clusterName)
				})
			})

			When("the deletion of the cluster is done, wait for the namespace deletion", func() {
				By(fmt.Sprintf("Checking the deletion of the %s namespace on the hub", clusterName), func() {
					klog.V(1).Infof("Checking the deletion of the %s namespace on the hub", clusterName)
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s namespace deletion...", clusterName)
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

			When("the namespace is deleted, check if managed cluster is well cleaned", func() {
				By(fmt.Sprintf("Checking if the %s is deleted", clusterName), func() {
					klog.V(1).Infof("Checking if the %s is deleted", clusterName)
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s namespace deletion...", clusterName)
						_, err := clientCluster.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
				By(fmt.Sprintf("Checking if the %s namespace is deleted", openClusterManagementAgentAddonNamespace), func() {
					klog.V(1).Infof("Checking if the %s is deleted", openClusterManagementAgentAddonNamespace)
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s namespace deletion...", openClusterManagementAgentAddonNamespace)
						_, err := clientCluster.CoreV1().Namespaces().Get(context.TODO(), openClusterManagementAgentAddonNamespace, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
				By(fmt.Sprintf("Checking if the %s namespace is deleted", openClusterManagementAgentNamespace), func() {
					klog.V(1).Infof("Checking if the %s is deleted", openClusterManagementAgentNamespace)
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s namespace deletion...", openClusterManagementAgentNamespace)
						_, err := clientCluster.CoreV1().Namespaces().Get(context.TODO(), openClusterManagementAgentNamespace, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
				By(fmt.Sprintf("Checking if the %s namespace is deleted", openClusterManagementAgentAddonNamespace), func() {
					klog.V(1).Infof("Checking if the %s is deleted", openClusterManagementAgentAddonNamespace)
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s namespace deletion...", openClusterManagementAgentAddonNamespace)
						_, err := clientCluster.CoreV1().Namespaces().Get(context.TODO(), openClusterManagementAgentAddonNamespace, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
				By(fmt.Sprintf("Checking if the %s crd is deleted", klusterletCRDName), func() {
					klog.V(1).Infof("Checking if the %s crd is deleted", klusterletCRDName)
					gvr := schema.GroupVersionResource{Group: "operator.open-cluster-management.io", Version: "v1", Resource: "klusterlets"}
					Eventually(func() bool {
						klog.V(1).Infof("Wait %s crd deletion...", klusterletCRDName)
						_, err := clientClusterDynamic.Resource(gvr).Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Info(err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
			})
		}

	})

})
