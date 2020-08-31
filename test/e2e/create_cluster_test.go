// Copyright (c) 2020 Red Hat, Inc.

// +build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	libgooptions "github.com/open-cluster-management/library-e2e-go/pkg/options"
	libgocrdv1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1/crd"
	libgodeploymentv1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1/deployment"
	libgoapplier "github.com/open-cluster-management/library-go/pkg/applier"
	"github.com/open-cluster-management/library-go/pkg/templateprocessor"

	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
)

var _ = Describe("Create AWS OpenShift cluster", func() {
	createCluster("aws", "OpenShift")
})

var _ = Describe("Create Azure OpenShift cluster", func() {
	createCluster("azure", "OpenShift")
})

var _ = Describe("Create GCP OpenShift cluster", func() {
	createCluster("gcp", "OpenShift")
})

func createCluster(cloud, vendor string) {
	var clusterNameObj *libgooptions.ClusterName
	var clusterName string
	var err error
	var imageRefName string

	BeforeEach(func() {
		if cloudProviders != "" && !isRequestedCloudProvider(cloud) {
			Skip(fmt.Sprintf("Cloud provider %s skipped", cloud))
		}
		clusterNameObj, err = libgooptions.NewClusterName(libgooptions.GetOwner(), cloud)
		Expect(err).To(BeNil())
		clusterName = clusterNameObj.String()
		klog.V(1).Infof(`========================= Start Test create cluster %s 
with image %s ===============================`, clusterName, imageRefName)
		SetDefaultEventuallyTimeout(10 * time.Minute)
		SetDefaultEventuallyPollingInterval(10 * time.Second)
	})

	AfterEach(func() {

	})

	It(fmt.Sprintf("Create cluster %s on %s with vendor %s (cluster/g1/create-cluster)", clusterName, cloud, vendor), func() {
		By("Checking the minimal requirements", func() {
			klog.V(1).Infof("Cluster %s: Checking the minimal requirements", clusterName)
			Eventually(func() bool {
				klog.V(2).Infof("Cluster %s: Check CRDs", clusterName)
				has, missing, _ := libgocrdv1.HasCRDs(hubClientAPIExtension,
					[]string{
						"managedclusters.cluster.open-cluster-management.io",
						"clusterdeployments.hive.openshift.io",
						"syncsets.hive.openshift.io",
					})
				if !has {
					klog.Errorf("Cluster %s: Missing CRDs\n%#v", clusterName, missing)
				}
				return has
			}).Should(BeTrue())

			Eventually(func() bool {
				has, missing, _ := libgodeploymentv1.HasDeploymentsInNamespace(hubClient,
					"open-cluster-management",
					[]string{"managedcluster-import-controller"})
				if !has {
					klog.Errorf("Cluster %s: Missing deployments\n%#v", clusterName, missing)
				}
				return has
			}).Should(BeTrue())
			Eventually(func() bool {
				has, missing, _ := libgodeploymentv1.HasDeploymentsInNamespace(hubClient,
					"open-cluster-management-hub",
					[]string{"cluster-manager-registration-controller"})
				if !has {
					klog.Errorf("Cluster %s: Missing deployments\n%#v", clusterName, missing)
				}
				return has
			}).Should(BeTrue())
			Eventually(func() bool {
				has, missing, _ := libgodeploymentv1.HasDeploymentsInNamespace(hubClient,
					"hive",
					[]string{"hive-controllers"})
				if !has {
					klog.Errorf("Missing deployments\n%#v", missing)
				}
				return has
			}).Should(BeTrue())

		})

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

		By("Creating the needed resources", func() {
			klog.V(1).Infof("Cluster %s: Creating the needed resources", clusterName)
			pullSecret := &corev1.Secret{}
			Expect(hubClientClient.Get(context.TODO(),
				types.NamespacedName{
					Name:      "pull-secret",
					Namespace: "openshift-config",
				},
				pullSecret)).To(BeNil())
			values := struct {
				ManagedClusterName          string
				ManagedClusterCloud         string
				ManagedClusterVendor        string
				ManagedClusterSSHPrivateKey string
				ManagedClusterPullSecret    string
			}{
				ManagedClusterName:          clusterName,
				ManagedClusterCloud:         cloud,
				ManagedClusterVendor:        vendor,
				ManagedClusterSSHPrivateKey: libgooptions.TestOptions.Connection.SSHPrivateKey,
				ManagedClusterPullSecret:    string(pullSecret.Data[".dockerconfigjson"]),
			}
			Expect(hubCreateApplier.CreateOrUpdateInPath(".",
				[]string{
					"install_config_secret_cr.yaml",
					"cluster_deployment_cr.yaml",
					"managed_cluster_cr.yaml",
					"klusterlet_addon_config_cr.yaml",
					"clusterimageset_cr.yaml",
				},
				false,
				values)).To(BeNil())

			klog.V(1).Infof("Cluster %s: Creating the %s cred secret", clusterName, cloud)
			Expect(createCredentialsSecret(hubCreateApplier, clusterName, cloud)).To(BeNil())

			klog.V(1).Infof("Cluster %s: Creating install config secret", clusterName)
			Expect(createInstallConfig(hubCreateApplier, createTemplateProcessor, clusterName, cloud)).To(BeNil())

			imageRefName = libgooptions.TestOptions.ManagedClusters.ImageSetRefName
			if imageRefName == "" && ocpImageRelease != "" {
				imageRefName, err = createClusterImageSet(hubCreateApplier, clusterNameObj, ocpImageRelease)
				Expect(err).To(BeNil())
			}
			if imageRefName == "" {
				imageRefName, err = createClusterImageSet(hubCreateApplier, clusterNameObj, libgooptions.TestOptions.ManagedClusters.OCPImageRelease)
				Expect(err).To(BeNil())
			}
		})

		By("creating the clusterDeployment", func() {
			region, err := libgooptions.GetRegion(cloud)
			Expect(err).To(BeNil())
			baseDomain, err := libgooptions.GetBaseDomain(cloud)
			Expect(err).To(BeNil())
			values := struct {
				ManagedClusterName          string
				ManagedClusterCloud         string
				ManagedClusterRegion        string
				ManagedClusterVendor        string
				ManagedClusterBaseDomain    string
				ManagedClusterImageRefName  string
				ManagedClusterBaseDomainRGN string
			}{
				ManagedClusterName:       clusterName,
				ManagedClusterCloud:      cloud,
				ManagedClusterRegion:     region,
				ManagedClusterVendor:     vendor,
				ManagedClusterBaseDomain: baseDomain,
				//TODO: parametrize the image
				ManagedClusterImageRefName:  imageRefName,
				ManagedClusterBaseDomainRGN: libgooptions.TestOptions.Connection.Keys.Azure.BaseDomainRGN,
			}
			klog.V(1).Infof("Cluster %s: Creating the clusterDeployment", clusterName)
			Expect(hubCreateApplier.CreateOrUpdateResource("cluster_deployment_cr.yaml", values)).To(BeNil())
		})

		By("Attaching the cluster by creating the managedCluster and klusterletaddonconfig", func() {
			// createKlusterletAddonConfig(hubCreateApplier, clusterName, cloud, vendor)
			createManagedCluster(hubCreateApplier, clusterName, cloud, vendor)
		})

		When("Import launched, wait for cluster to be installed", func() {
			gvr := schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"}
			Eventually(func() error {
				klog.V(1).Infof("Cluster %s: Wait %s to be installed...", clusterName, clusterName)
				clusterDeployment, err := hubClientDynamic.Resource(gvr).Namespace(clusterName).Get(context.TODO(), clusterName, metav1.GetOptions{})
				if err == nil {
					if si, ok := clusterDeployment.Object["status"]; ok {
						s := si.(map[string]interface{})
						if ti, ok := s["installedTimestamp"]; ok && ti != nil {
							return nil
						}
					}
					return fmt.Errorf("No status available")
				} else {
					klog.V(4).Info(err)
				}
				return err
			}, 3600, 60).Should(BeNil())
		})

		When(fmt.Sprintf("Import launched, wait for cluster %s to be ready", clusterName), func() {
			waitClusterImported(hubClientDynamic, clusterName)
		})

		When("Imported, validate...", func() {
			validateClusterImported(hubClientDynamic, hubClient, clusterName)
		})

		By(fmt.Sprintf("Detaching the %s CR on the hub", clusterName), func() {
			klog.V(1).Infof("Cluster %s: Detaching the %s CR on the hub", clusterName, clusterName)
			gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}
			Expect(hubClientDynamic.Resource(gvr).Delete(context.TODO(), clusterName, metav1.DeleteOptions{})).Should(BeNil())
		})

		When(fmt.Sprintf("the detach of the cluster %s is requested, wait for the effective detach", clusterName), func() {
			waitDetached(hubClientDynamic, clusterName)
		})

		// When("Detached, validate...", func() {
		// 	validateClusterDetached(hubClientDynamic, hubClient, clusterName)
		// })

		// By("Re-attaching the cluster by recreating the managedCluster and klusterletaddonconfig", func() {
		// 	// createKlusterletAddonConfig(hubCreateApplier, clusterName, cloud, vendor)
		// 	createManagedCluster(hubCreateApplier, clusterName, cloud, vendor)
		// })

		// When(fmt.Sprintf("Checking if the cluster %s gets re-imported", clusterName), func() {
		// 	waitClusterImported(hubClientDynamic, clusterName)
		// })

		When(fmt.Sprintf("Detached, delete the clusterDeployment %s", clusterName), func() {
			klog.V(1).Infof("Cluster %s: Deleting the clusterDeployment for cluster %s", clusterName, clusterName)
			gvr := schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"}
			Expect(hubClientDynamic.Resource(gvr).Namespace(clusterName).Delete(context.TODO(), clusterName, metav1.DeleteOptions{})).Should(BeNil())
		})

		When(fmt.Sprintf("Wait clusterDeployment %s to be deleted", clusterName), func() {
			waitDetroyed(hubClientDynamic, clusterName)
		})

		klog.V(1).Infof("========================= End Test create cluster %s ===============================", clusterName)

	})

}

func isRequestedCloudProvider(cloud string) bool {
	cloudProviderstSlice := strings.Split(cloudProviders, ",")
	klog.V(5).Infof("cloudProviderSlice %v", cloudProviderstSlice)
	for _, cloudProvider := range cloudProviderstSlice {
		cp := strings.TrimSpace(cloudProvider)
		klog.V(5).Infof("cloudProvider %s", cp)
		if cp == cloud {
			return true
		}
	}
	return false
}

func createCredentialsSecret(hubCreateApplier *libgoapplier.Applier, clusterName, cloud string) error {
	switch cloud {
	case "aws":
		cloudCredSecretValues := struct {
			ManagedClusterName string
			AWSAccessKeyID     string
			AWSSecretAccessKey string
		}{
			ManagedClusterName: clusterName,
			AWSAccessKeyID:     libgooptions.TestOptions.Connection.Keys.AWS.AWSAccessKeyID,
			AWSSecretAccessKey: libgooptions.TestOptions.Connection.Keys.AWS.AWSAccessSecret,
		}
		return hubCreateApplier.CreateOrUpdateResource(filepath.Join(cloud, "creds_secret_cr.yaml"), cloudCredSecretValues)
	case "azure":
		cloudCredSecretValues := struct {
			ManagedClusterName           string
			ManagedClusterClientId       string
			ManagedClusterClientSecret   string
			ManagedClusterTenantId       string
			ManagedClusterSubscriptionId string
		}{
			ManagedClusterName:           clusterName,
			ManagedClusterClientId:       libgooptions.TestOptions.Connection.Keys.Azure.ClientId,
			ManagedClusterClientSecret:   libgooptions.TestOptions.Connection.Keys.Azure.ClientSecret,
			ManagedClusterTenantId:       libgooptions.TestOptions.Connection.Keys.Azure.TenantId,
			ManagedClusterSubscriptionId: libgooptions.TestOptions.Connection.Keys.Azure.SubscriptionId,
		}
		return hubCreateApplier.CreateOrUpdateAsset(filepath.Join(cloud, "creds_secret_cr.yaml"), cloudCredSecretValues)
	case "gcp":
		cloudCredSecretValues := struct {
			ManagedClusterName      string
			GCPOSServiceAccountJson string
		}{
			ManagedClusterName:      clusterName,
			GCPOSServiceAccountJson: libgooptions.TestOptions.Connection.Keys.GCP.ServiceAccountJsonKey,
		}
		return hubCreateApplier.CreateOrUpdateAsset(filepath.Join(cloud, "creds_secret_cr.yaml"), cloudCredSecretValues)

	default:
		return fmt.Errorf("Unsupporter cloud %s", cloud)
	}
}

func createInstallConfig(hubCreateApplier *libgoapplier.Applier,
	createTemplateProcessor *templateprocessor.TemplateProcessor,
	clusterName,
	cloud string) error {
	baseDomain, err := libgooptions.GetBaseDomain(cloud)
	if err != nil {
		return err
	}
	region, err := libgooptions.GetRegion(cloud)
	if err != nil {
		return err
	}
	var b []byte
	switch cloud {
	case "aws":
		installConfigValues := struct {
			ManagedClusterName         string
			ManagedClusterBaseDomain   string
			ManagedClusterRegion       string
			ManagedClusterSSHPublicKey string
		}{
			ManagedClusterName:         clusterName,
			ManagedClusterBaseDomain:   baseDomain,
			ManagedClusterRegion:       region,
			ManagedClusterSSHPublicKey: libgooptions.TestOptions.Connection.SSHPublicKey,
		}
		b, err = createTemplateProcessor.TemplateAsset(filepath.Join(cloud, "install_config.yaml"), installConfigValues)
	case "azure":
		installConfigValues := struct {
			ManagedClusterName          string
			ManagedClusterBaseDomain    string
			ManagedClusterBaseDomainRGN string
			ManagedClusterRegion        string
			ManagedClusterSSHPublicKey  string
		}{
			ManagedClusterName:          clusterName,
			ManagedClusterBaseDomain:    baseDomain,
			ManagedClusterBaseDomainRGN: libgooptions.TestOptions.Connection.Keys.Azure.BaseDomainRGN,
			ManagedClusterRegion:        region,
			ManagedClusterSSHPublicKey:  libgooptions.TestOptions.Connection.SSHPublicKey,
		}
		b, err = createTemplateProcessor.TemplateAsset(filepath.Join(cloud, "install_config.yaml"), installConfigValues)
	case "gcp":
		installConfigValues := struct {
			ManagedClusterName         string
			ManagedClusterBaseDomain   string
			ManagedClusterProjectID    string
			ManagedClusterRegion       string
			ManagedClusterSSHPublicKey string
		}{
			ManagedClusterName:         clusterName,
			ManagedClusterBaseDomain:   baseDomain,
			ManagedClusterProjectID:    libgooptions.TestOptions.Connection.Keys.GCP.ProjectID,
			ManagedClusterRegion:       region,
			ManagedClusterSSHPublicKey: libgooptions.TestOptions.Connection.SSHPublicKey,
		}
		b, err = createTemplateProcessor.TemplateAsset(filepath.Join(cloud, "install_config.yaml"), installConfigValues)
	default:
		return fmt.Errorf("Unsupporter cloud %s", cloud)

	}
	if err != nil {
		return err
	}
	installConfigSecretValues := struct {
		ManagedClusterName          string
		ManagedClusterInstallConfig string
	}{
		ManagedClusterName:          clusterName,
		ManagedClusterInstallConfig: base64.StdEncoding.EncodeToString(b),
	}
	return hubCreateApplier.CreateOrUpdateAsset("install_config_secret_cr.yaml", installConfigSecretValues)
}

func createKlusterletAddonConfig(hubCreateApplier *libgoapplier.Applier, clusterName, cloud, vendor string) {
	By("creating the klusterletaddonconfig", func() {
		values := struct {
			ManagedClusterName   string
			ManagedClusterCloud  string
			ManagedClusterVendor string
		}{
			ManagedClusterName:   clusterName,
			ManagedClusterCloud:  cloud,
			ManagedClusterVendor: vendor,
		}
		klog.V(1).Infof("Cluster %s: Creating the klusterletaddonconfig", clusterName)
		Expect(hubCreateApplier.CreateOrUpdateAsset("klusterlet_addon_config_cr.yaml", values)).To(BeNil())
	})
}

func createClusterImageSet(hubCreateApplier *libgoapplier.Applier, clusterNameObj *libgooptions.ClusterName, ocpImageRelease string) (string, error) {
	ocpImageReleaseSlice := strings.Split(ocpImageRelease, ":")
	if len(ocpImageReleaseSlice) != 2 {
		return "", fmt.Errorf("OCPImageRelease malformed: %s (no tag)", ocpImageRelease)
	}
	normalizedOCPImageRelease := strings.ReplaceAll(ocpImageReleaseSlice[1], "_", "-")
	normalizedOCPImageRelease = strings.ToLower(normalizedOCPImageRelease)
	clusterImageSetName := fmt.Sprintf("%s-%s-%s", clusterNameObj.Owner, normalizedOCPImageRelease, clusterNameObj.GetUID())
	values := struct {
		ClusterImageSetName string
		OCPReleaseImage     string
	}{
		ClusterImageSetName: clusterImageSetName,
		OCPReleaseImage:     ocpImageRelease,
	}
	klog.V(1).Infof("Cluster %s: Creating the imageSetName %s", clusterNameObj, clusterImageSetName)
	err := hubCreateApplier.CreateOrUpdateAsset("clusterimageset_cr.yaml", values)
	if err != nil {
		return "", err
	}
	return clusterImageSetName, nil
}

func createManagedCluster(hubCreateApplier *libgoapplier.Applier, clusterName, cloud, vendor string) {
	By("creating the managedCluster", func() {
		values := struct {
			ManagedClusterName   string
			ManagedClusterCloud  string
			ManagedClusterVendor string
		}{
			ManagedClusterName:   clusterName,
			ManagedClusterCloud:  cloud,
			ManagedClusterVendor: vendor,
		}
		klog.V(1).Infof("Cluster %s: Creating the managedCluster", clusterName)
		Expect(hubCreateApplier.CreateOrUpdateAsset("managed_cluster_cr.yaml", values)).To(BeNil())
	})
}

func waitDetroyed(hubClientDynamic dynamic.Interface, clusterName string) {
	By(fmt.Sprintf("Checking the deletion of the %s clusterDeployment on the hub", clusterName), func() {
		klog.V(1).Infof("Cluster %s: Checking the deletion of the %s clusterDeployment on the hub", clusterName, clusterName)
		gvr := schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"}
		Eventually(func() bool {
			klog.V(1).Infof("Cluster %s: Wait %s clusterDeployment deletion...", clusterName, clusterName)
			_, err := hubClientDynamic.Resource(gvr).Namespace(clusterName).Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				klog.V(1).Info(err)
				return errors.IsNotFound(err)
			}
			return false
		}, 3600, 60).Should(BeTrue())
		klog.V(1).Infof("Cluster %s: %s clusterDeployment deleted", clusterName, clusterName)
	})
}
