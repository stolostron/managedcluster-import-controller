// Copyright (c) 2020 Red Hat, Inc.

// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
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
			clusterApplier, err = libgoapplier.NewApplier(templateProcessor, clusterClientClient, nil, nil, nil)
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
				klog.V(1).Info("Creating the namesapce in which the cluster will be imported")
				namespaces := clientHub.CoreV1().Namespaces()
				Expect(namespaces.Create(context.TODO(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: clusterName,
					},
				}, metav1.CreateOptions{})).NotTo(BeNil())
				Expect(namespaces.Get(context.TODO(), clusterName, metav1.GetOptions{})).NotTo(BeNil())
			})

			// By("creating the secret to retrieve the images", func() {
			// 	klog.V(1).Info("Creating the secret to retrieve the images from the repository")
			// 	userpw64 := base64.StdEncoding.EncodeToString([]byte(registryUser + ":" + registryPassword))
			// 	//Create the cluster NS on master
			// 	Expect(clientHub.CoreV1().Secrets(clusterName).Create(context.TODO(), &corev1.Secret{
			// 		ObjectMeta: metav1.ObjectMeta{
			// 			Name: MANUAL_IMPORT_IMAGE_PULL_SECRET,
			// 		},
			// 		Type: corev1.SecretTypeDockerConfigJson,
			// 		StringData: map[string]string{
			// 			corev1.DockerConfigJsonKey: "{\"auths\":{\"" + registry + "\":{\"username\":\"" + registryUser + "\",\"password\":\"" + registryPassword + "\",\"auth\":\"" + userpw64 + "\"}}}",
			// 		},
			// 	}, metav1.CreateOptions{})).NotTo(BeNil())
			// 	Expect(clientHub.CoreV1().Secrets(clusterName).Get(context.TODO(), MANUAL_IMPORT_IMAGE_PULL_SECRET, metav1.GetOptions{})).NotTo(BeNil())
			// })

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

			var csrFound certificatesv1beta1.CertificateSigningRequest
			When("Import launched, wait for csr", func() {
				signingRequest := clientHub.CertificatesV1beta1().CertificateSigningRequests()
				Eventually(func() error {
					klog.V(1).Info("Waiting CSR...")
					csrs, err := signingRequest.List(context.TODO(), metav1.ListOptions{})
					if err != nil {
						return err
					}
					for _, csr := range csrs.Items {
						if strings.HasPrefix(csr.Name, clusterName) {
							csrFound = csr
							return nil
						}
					}
					return fmt.Errorf("CSR starting with %s not found", clusterName)
				}).Should(BeNil())
			})

			By("Approving CSR", func() {
				signingRequest := clientHub.CertificatesV1beta1().CertificateSigningRequests()
				csrFound.Status.Conditions = append(csrFound.Status.Conditions, certificatesv1beta1.CertificateSigningRequestCondition{
					Type:           certificatesv1beta1.CertificateApproved,
					Reason:         "e2e test manual-approval",
					Message:        "This CSR was approved by e2e test manual-approval",
					LastUpdateTime: metav1.Now(),
				})
				_, err := signingRequest.UpdateApproval(context.TODO(), &csrFound, metav1.UpdateOptions{})
				Expect(err).Should(BeNil())
			})

			When("CSR Approved, wait for cluster ready", func() {
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
		}
	})

	// 	By(fmt.Sprintf("Deleting the %s CR on the hub", managedClusterForManualImport.Name), func() {
	// 		klog.V(1).Infof("Deleting the %s CR on the hub", managedClusterForManualImport.Name)
	// 		gvr := schema.GroupVersionResource{Group: "clusterregistry.k8s.io", Version: "v1alpha1", Resource: "clusters"}
	// 		Expect(clientHubDynamic.Resource(gvr).Namespace(managedClusterForManualImport.Name).Delete(managedClusterForManualImport.Name, &metav1.DeleteOptions{})).NotTo(HaveOccurred())
	// 	})

	// 	When("the deletion of the cluster is requested, wait for the effective deletion", func() {
	// 		By(fmt.Sprintf("Checking the deletion of the %s CR on the hub", managedClusterForManualImport.Name), func() {
	// 			klog.V(1).Infof("Checking the deletion of the %s CR on the hub", managedClusterForManualImport.Name)
	// 			gvr := schema.GroupVersionResource{Group: "clusterregistry.k8s.io", Version: "v1alpha1", Resource: "clusters"}
	// 			Eventually(func() bool {
	// 				klog.V(1).Infof("Wait %s CR deletion...", managedClusterForManualImport.Name)
	// 				_, err := clientHubDynamic.Resource(gvr).Namespace(managedClusterForManualImport.Name).Get(managedClusterForManualImport.Name, metav1.GetOptions{})
	// 				if err != nil {
	// 					klog.V(1).Info(err)
	// 					return errors.IsNotFound(err)
	// 				}
	// 				return false
	// 			}).Should(BeTrue())
	// 			klog.V(1).Infof("%s CR deleted", managedClusterForManualImport.Name)
	// 		})

	// 		By("Checking the deletion of the namespace multicluster-endpoint on the managed cluster", func() {
	// 			klog.V(1).Info("Checking the deletion of the namespace multicluster-endpoint on the managed cluster")
	// 			Eventually(func() bool {
	// 				klog.V(1).Info("Wait namespace multicluster-endpoint deletion...")
	// 				_, err := clientManagedCluster.CoreV1().Namespaces().Get("multicluster-endpoint", metav1.GetOptions{})
	// 				if err != nil {
	// 					klog.V(1).Info(err)
	// 					return errors.IsNotFound(err)
	// 				}
	// 				return false
	// 			}).Should(BeTrue())
	// 			klog.V(1).Info("namespace multicluster-endpoint deleted")
	// 		})
	// 	})

	// 	When("the deletion of the namespace multicluster-endpoint is done, delete the namespace on the hub", func() {
	// 		klog.V(1).Info("the deletion of the namespace multicluster-endpoint is done, delete the namespace on the hub")
	// 		By(fmt.Sprintf("deleting the cluster namespace %s on the hub", managedClusterForManualImport.Name), func() {
	// 			Expect(clientHub.CoreV1().Namespaces().Delete(managedClusterForManualImport.Name, &metav1.DeleteOptions{})).NotTo(HaveOccurred())
	// 		})
	// 		By(fmt.Sprintf("Checking the deletion of the hub cluster namespace %s", managedClusterForManualImport.Name), func() {
	// 			Eventually(func() bool {
	// 				klog.V(1).Infof("Wait cluster namespace %s deletion...", managedClusterForManualImport.Name)
	// 				_, err = clientHub.CoreV1().Namespaces().Get(managedClusterForManualImport.Name, metav1.GetOptions{})
	// 				if err != nil {
	// 					return errors.IsNotFound(err)
	// 				}
	// 				return false
	// 			}).Should(BeTrue())
	// 			klog.V(1).Infof("cluster namespace %s deleted", managedClusterForManualImport.Name)
	// 		})
	// 	})
})
