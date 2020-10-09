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
	"k8s.io/client-go/dynamic"

	libgocrdv1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1/crd"
	libgodeploymentv1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1/deployment"

	"k8s.io/klog"
)

var _ = Describe("Import cluster", func() {

	var wasAlreadyImported bool

	BeforeEach(func() {
		SetDefaultEventuallyTimeout(15 * time.Minute)
		SetDefaultEventuallyPollingInterval(10 * time.Second)
	})

	It("Given a list of clusters to import (cluster/g2/import-hub)", func() {
		clusterName := "self-import"
		klog.V(1).Infof("========================= Test cluster import hub %s ===============================", clusterName)
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

		wasAlreadyImported = false
		if checkClusterImported(hubClientDynamic, clusterName) == nil {
			wasAlreadyImported = true
			unimport(hubClientDynamic, clusterName)
		}

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
			Expect(hubSelfImportApplier.CreateOrUpdateAsset("managedcluster_cr.yaml", values)).To(BeNil())
		})

		When("the managedcluster is created, wait for import secret", func() {
			var err error
			Eventually(func() error {
				klog.V(1).Infof("Cluster %s: Wait import secret %s...", clusterName, clusterName)
				_, err = hubClient.CoreV1().Secrets(clusterName).Get(context.TODO(), clusterName+"-import", metav1.GetOptions{})
				if err != nil {
					klog.V(1).Infof("Cluster %s: %s", clusterName, err)
				}
				return err
			}).Should(BeNil())
			klog.V(1).Infof("Cluster %s: bootstrap import secret %s created", clusterName, clusterName+"-import")
		})

		When(fmt.Sprintf("Import launched, wait for cluster %s to be ready", clusterName), func() {
			waitClusterImported(hubClientDynamic, clusterName)
		})

		When(fmt.Sprintf("Cluster %s ready, wait manifestWorks to be applied", clusterName), func() {
			checkManifestWorksApplied(hubClientDynamic, clusterName)
		})

		if !wasAlreadyImported {
			klog.V(1).Infof("Cluster %s: Wait 10 min to settle", clusterName)
			time.Sleep(10 * time.Minute)
			unimport(hubClientDynamic, clusterName)
		}
	})

})

func unimport(hubClientDynamic dynamic.Interface, clusterName string) {
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
				_, err := hubClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
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
				_, err := hubClient.CoreV1().Namespaces().Get(context.TODO(), openClusterManagementAgentAddonNamespace, metav1.GetOptions{})
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
				_, err := hubClient.CoreV1().Namespaces().Get(context.TODO(), openClusterManagementAgentNamespace, metav1.GetOptions{})
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
				_, err := hubClientDynamic.Resource(gvr).Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
				if err != nil {
					klog.V(1).Infof("Cluster %s: %s", clusterName, err)
					return errors.IsNotFound(err)
				}
				return false
			}).Should(BeTrue())
		})
	})

}
