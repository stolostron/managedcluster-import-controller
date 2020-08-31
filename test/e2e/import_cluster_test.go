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
	libgodeploymentv1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1/deployment"
	libgoapplier "github.com/open-cluster-management/library-go/pkg/applier"
	libgoclient "github.com/open-cluster-management/library-go/pkg/client"
	"github.com/open-cluster-management/library-go/pkg/templateprocessor"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

var _ = Describe("Import cluster", func() {

	var err error
	var managedClustersForManualImport map[string]string
	var managedClusterClient client.Client
	var managedClusterKubeClient kubernetes.Interface
	var managedClusterDynamicClient dynamic.Interface
	var managedClusterApplier *libgoapplier.Applier

	BeforeEach(func() {
		managedClustersForManualImport, err = libgooptions.GetManagedClusterKubeConfigs(libgooptions.TestOptions.ManagedClusters.ConfigDir, importClusterScenario)
		Expect(err).To(BeNil())
		if len(managedClustersForManualImport) == 0 {
			Skip("Manual import not executed because no managed cluster defined for import")
		}
		SetDefaultEventuallyTimeout(15 * time.Minute)
		SetDefaultEventuallyPollingInterval(10 * time.Second)
	})

	It("Given a list of clusters to import (cluster/g0/import-service-resources)", func() {
		for clusterName, clusterKubeconfig := range managedClustersForManualImport {
			klog.V(1).Infof("========================= Test cluster import cluster %s ===============================", clusterName)
			managedClusterClient, err = libgoclient.NewDefaultClient(clusterKubeconfig, client.Options{})
			Expect(err).To(BeNil())
			managedClusterApplier, err = libgoapplier.NewApplier(importTamlReader, &templateprocessor.Options{}, managedClusterClient, nil, nil, libgoapplier.DefaultKubernetesMerger, nil)
			Expect(err).To(BeNil())
			managedClusterKubeClient, err = libgoclient.NewDefaultKubeClient(clusterKubeconfig)
			Expect(err).To(BeNil())
			managedClusterDynamicClient, err = libgoclient.NewDefaultKubeClientDynamic(clusterKubeconfig)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				klog.V(1).Infof("Cluster %s: Check CRDs", clusterName)
				has, _, _ := libgocrdv1.HasCRDs(hubClientAPIExtension,
					[]string{
						"managedclusters.cluster.open-cluster-management.io",
						"manifestworks.work.open-cluster-management.io",
					})
				return has
			}).Should(BeTrue())

			Eventually(func() error {
				_, _, err := libgodeploymentv1.HasDeploymentsInNamespace(hubClient,
					"open-cluster-management",
					[]string{"managedcluster-import-controller"})
				return err
			}).Should(BeNil())

			Eventually(func() error {
				_, _, err := libgodeploymentv1.HasDeploymentsInNamespace(hubClient,
					"open-cluster-management-hub",
					[]string{"cluster-manager-registration-controller"})
				return err
			}).Should(BeNil())

			By("creating the namespace in which the cluster will be imported", func() {
				//Create the cluster NS on master
				klog.V(1).Infof("Cluster %s: Creating the namespace in which the cluster will be imported", clusterName)
				namespaces := hubClient.CoreV1().Namespaces()
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
				klog.V(1).Infof("Cluster %s: Creating the managedCluster", clusterName)
				values := struct {
					ManagedClusterName string
				}{
					ManagedClusterName: clusterName,
				}
				Expect(hubImportApplier.CreateOrUpdateAsset("managedcluster_cr.yaml", values)).To(BeNil())
			})

			var importSecret *corev1.Secret
			When("the managedcluster is created, wait for import secret", func() {
				var err error
				Eventually(func() error {
					klog.V(1).Infof("Cluster %s: Wait import secret %s...", clusterName, clusterName)
					importSecret, err = hubClient.CoreV1().Secrets(clusterName).Get(context.TODO(), clusterName+"-import", metav1.GetOptions{})
					if err != nil {
						klog.V(1).Infof("Cluster %s: %s", clusterName, err)
					}
					return err
				}).Should(BeNil())
				klog.V(1).Infof("Cluster %s: bootstrap import secret %s created", clusterName, clusterName+"-import")
			})

			By("Launching the manual import", func() {
				klog.V(1).Infof("Cluster %s: Apply the crds.yaml", clusterName)
				klog.V(5).Infof("Cluster %s: importSecret.Data[crds.yaml]: %s\n", clusterName, importSecret.Data["crds.yaml"])
				Expect(managedClusterApplier.CreateOrUpdateAssets(importSecret.Data["crds.yaml"], nil, "---")).NotTo(HaveOccurred())
				//Wait 2 sec to make sure the CRDs are effective. The UI does the same.
				time.Sleep(2 * time.Second)
				klog.V(1).Infof("Cluster %s: Apply the import.yaml", clusterName)
				klog.V(5).Infof("Cluster %s: importSecret.Data[import.yaml]: %s\n", clusterName, importSecret.Data["import.yaml"])
				Expect(managedClusterApplier.CreateOrUpdateAssets(importSecret.Data["import.yaml"], nil, "---")).NotTo(HaveOccurred())
			})

			When(fmt.Sprintf("Import launched, wait for cluster %s to be ready", clusterName), func() {
				waitClusterImported(hubClientDynamic, clusterName)
			})

			When(fmt.Sprintf("Cluster %s ready, wait manifestWorks to be applied", clusterName), func() {
				checkManifestWorksApplied(hubClientDynamic, clusterName)
			})

			klog.V(1).Infof("Cluster %s: Wait 10 min to settle", clusterName)
			time.Sleep(10 * time.Minute)

			By(fmt.Sprintf("Detaching the %s CR on the hub", clusterName), func() {
				klog.V(1).Infof("Cluster %s: Detaching the %s CR on the hub", clusterName, clusterName)
				gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
				Expect(hubClientDynamic.Resource(gvr).Delete(context.TODO(), clusterName, metav1.DeleteOptions{})).Should(BeNil())
			})

			When(fmt.Sprintf("the detach of the cluster %s is requested, wait for the effective detach", clusterName), func() {
				waitDetached(hubClientDynamic, clusterName)
			})

			When("the deletion of the cluster is done, wait for the namespace deletion", func() {
				By(fmt.Sprintf("Checking the deletion of the %s namespace on the hub", clusterName), func() {
					klog.V(1).Infof("Cluster %s: Checking the deletion of the %s namespace on the hub", clusterName, clusterName)
					Eventually(func() bool {
						klog.V(1).Infof("Cluster %s: Wait %s namespace deletion...", clusterName, clusterName)
						_, err := hubClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Infof("Cluster %s: %s", clusterName, err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
					klog.V(1).Infof("Cluster %s: %s namespace deleted", clusterName, clusterName)
				})
			})

			When("the namespace is deleted, check if managed cluster is well cleaned", func() {
				By(fmt.Sprintf("Checking if the %s is deleted", clusterName), func() {
					klog.V(1).Infof("Cluster %s: Checking if the %s is deleted", clusterName, clusterName)
					Eventually(func() bool {
						klog.V(1).Infof("Cluster %s: Wait %s namespace deletion...", clusterName, clusterName)
						_, err := managedClusterKubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Infof("Cluster %s: %s", clusterName, err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
				By(fmt.Sprintf("Checking if the %s namespace is deleted", openClusterManagementAgentAddonNamespace), func() {
					klog.V(1).Infof("Cluster %s: Checking if the %s is deleted", clusterName, openClusterManagementAgentAddonNamespace)
					Eventually(func() bool {
						klog.V(1).Infof("Cluster %s: Wait %s namespace deletion...", clusterName, openClusterManagementAgentAddonNamespace)
						_, err := managedClusterKubeClient.CoreV1().Namespaces().Get(context.TODO(), openClusterManagementAgentAddonNamespace, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Infof("Cluster %s: %s", clusterName, err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
				By(fmt.Sprintf("Checking if the %s namespace is deleted", openClusterManagementAgentNamespace), func() {
					klog.V(1).Infof("Cluster %s: Checking if the %s is deleted", clusterName, openClusterManagementAgentNamespace)
					Eventually(func() bool {
						klog.V(1).Infof("Cluster %s: Wait %s namespace deletion...", clusterName, openClusterManagementAgentNamespace)
						_, err := managedClusterKubeClient.CoreV1().Namespaces().Get(context.TODO(), openClusterManagementAgentNamespace, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Infof("Cluster %s: %s", clusterName, err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
				By(fmt.Sprintf("Checking if the %s crd is deleted", klusterletCRDName), func() {
					klog.V(1).Infof("Cluster %s: Checking if the %s crd is deleted", clusterName, klusterletCRDName)
					gvr := schema.GroupVersionResource{Group: "operator.open-cluster-management.io", Version: "v1", Resource: "klusterlets"}
					Eventually(func() bool {
						klog.V(1).Infof("Cluster %s: Wait %s crd deletion...", clusterName, klusterletCRDName)
						_, err := managedClusterDynamicClient.Resource(gvr).Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
						if err != nil {
							klog.V(1).Infof("Cluster %s: %s", clusterName, err)
							return errors.IsNotFound(err)
						}
						return false
					}).Should(BeTrue())
				})
			})
		}

	})

})
