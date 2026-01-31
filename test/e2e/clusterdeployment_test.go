// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster with clusterdeployment", ginkgo.Label("core"), func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		// reset the custom controller config
		util.RemoveControllerConfigConfigMap(hubKubeClient)

		managedClusterName = fmt.Sprintf("clusterdeployment-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(hubClusterClient, managedClusterName, nil)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create a clusterdeployment for managed cluster %s", managedClusterName), func() {
			err := util.CreateClusterDeployment(hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Install the clusterdeployment %s", managedClusterName), func() {
			gomega.Eventually(func() error {
				return util.InstallClusterDeployment(hubKubeClient, hubDynamicClient, managedClusterName)
			}, 30, 1).ShouldNot(gomega.HaveOccurred())
		})

		assertManagedClusterImportSecretCreated(managedClusterName, "hive")

		ginkgo.By(fmt.Sprintf("removed create-via hive annotation from %s, and the annotation will be added back",
			managedClusterName), func() {
			gomega.Eventually(func() error {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(),
					managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				delete(cluster.Annotations, "open-cluster-management/created-via")
				_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(),
					cluster, metav1.UpdateOptions{})
				return err
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())

			assertManagedClusterCreatedViaAnnotation(managedClusterName, "hive")
		})

		assertManagedClusterManifestWorks(managedClusterName)
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
		assertManagedClusterPriorityClass(managedClusterName)
	})

	ginkgo.Context("with custom auto-import-strategy", func() {
		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hubKubeClient)

			ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
				err := util.DeleteClusterDeployment(hubDynamicClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
			assertManagedClusterDeleted(managedClusterName)
		})

		ginkgo.It("Should not recover the agent once joined if auto-import strategy is ImportOnly", func() {
			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hubKubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", managedClusterName), func() {
				err := util.RemoveKlusterlet(hubOperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				assertManagedClusterAvailableUnknown(managedClusterName)
			})

			ginkgo.By(fmt.Sprintf("Should not recover the managed cluster %s after deleting import secret", managedClusterName), func() {
				err := util.RemoveImportSecret(hubKubeClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				assertManagedClusterImportSecretCreated(managedClusterName, "hive")
				assertManagedClusterAvailableUnknownConsistently(managedClusterName, 30*time.Second)
			})

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the auto-import strategy is set to ImportAndSync", managedClusterName), func() {
				err := util.SetAutoImportStrategy(hubKubeClient, apiconstants.AutoImportStrategyImportAndSync)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				err = util.RemoveImportSecret(hubKubeClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				assertManagedClusterImportSecretCreated(managedClusterName, "hive")
				assertManagedClusterAvailable(managedClusterName)
			})
		})
	})

	ginkgo.Context("with immediate-import annotation", func() {
		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hubKubeClient)

			ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
				err := util.DeleteClusterDeployment(hubDynamicClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
			assertManagedClusterDeleted(managedClusterName)
		})

		ginkgo.It("Should trigger auto-import with immediate-import annotation", func() {
			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hubKubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", managedClusterName), func() {
				err := util.RemoveKlusterlet(hubOperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				assertManagedClusterAvailableUnknown(managedClusterName)
			})

			// Wait for namespace to be fully deleted before re-importing to avoid
			// "unable to create new content in namespace because it is being terminated" error
			assertKlusterletNamespaceDeleted()

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the immediate-import annotation is added", managedClusterName), func() {
				err := util.SetImmediateImportAnnotation(hubClusterClient, managedClusterName, "")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				assertManagedClusterImportSecretCreated(managedClusterName, "hive")
				assertManagedClusterAvailable(managedClusterName)
			})

			assertImmediateImportCompleted(managedClusterName)
		})
	})

	ginkgo.It(fmt.Sprintf("Should destroy the managed cluster %s", managedClusterName), func() {
		ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
			err := util.DeleteClusterDeployment(hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", managedClusterName), func() {
			err := hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertManagedClusterDeletedFromHub(managedClusterName)

		// In e2e environment (Hub = Spoke), we need to clean up orphaned AppliedManifestWork
		// and delete Klusterlet explicitly. See docs/e2e-cleanup-analysis.md for details.
		cleanupOrphanedAppliedManifestWork()
		deleteKlusterletIfExists()

		assertKlusterletNamespaceDeleted()
		assertKlusterletDeleted()
	})

	ginkgo.It(fmt.Sprintf("Should detach the managed cluster %s", managedClusterName), func() {
		ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", managedClusterName), func() {
			err := hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertOnlyManagedClusterDeleted(managedClusterName)

		// In e2e environment (Hub = Spoke), we need to clean up orphaned AppliedManifestWork
		// and delete Klusterlet explicitly. See docs/e2e-cleanup-analysis.md for details.
		cleanupOrphanedAppliedManifestWork()
		deleteKlusterletIfExists()

		assertKlusterletNamespaceDeleted()
		assertKlusterletDeleted()

		ginkgo.By("Should have the managed cluster namespace", func() {
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
			err := util.DeleteClusterDeployment(hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Should delete the managed cluster namespace", func() {
			gomega.Eventually(func() error {
				_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("the managed cluster namespace %s should be deleted", managedClusterName)
			}, 10*time.Minute, 1*time.Second).Should(gomega.Succeed())
		})
	})
})

func assertOnlyManagedClusterDeleted(managedClusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert delete the managed cluster %s spending time: %.2f seconds", managedClusterName, time.Since(start).Seconds())
	}()
	ginkgo.By("Should delete the managed cluster", func() {
		gomega.Eventually(func() error {
			_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("the managed cluster %s should be deleted", managedClusterName)
		}, 10*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertKlusterletNamespaceDeleted() {
	start := time.Now()
	defer func() {
		util.Logf("assert delete the open-cluster-management-agent namespace spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should delete the open-cluster-management-agent namespace", func() {
		gomega.Eventually(func() error {
			klusterletNamespace := "open-cluster-management-agent"
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("the klusterlet namespace %s should be deleted", klusterletNamespace)
		}, 10*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})

}

func assertKlusterletDeleted() {
	start := time.Now()
	ginkgo.By("Should delete the klusterlet crd", func() {
		gomega.Eventually(func() error {
			klusterletCRDName := "klusterlets.operator.open-cluster-management.io"
			crd, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			if crd.DeletionTimestamp.IsZero() {
				// klusterlet crd is not in
				return fmt.Errorf("the klusterlet crd %s deletionTimestamp should not be zero", klusterletCRDName)
			}

			klusterlet, err := hubOperatorClient.OperatorV1().Klusterlets().Get(context.TODO(), "klusterlet", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return fmt.Errorf("the klusterlet crd %s should be deleted", klusterletCRDName)
			}
			if err != nil {
				return err
			}

			if klusterlet.DeletionTimestamp.IsZero() {
				return fmt.Errorf("the klusterlet crd %s deletionTimestamp should not be zero", klusterletCRDName)
			}

			// klusterlet is not deleted yet
			klusterlet = klusterlet.DeepCopy()
			klusterlet.Finalizers = []string{}
			_, err = hubOperatorClient.OperatorV1().Klusterlets().Update(context.TODO(), klusterlet, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return fmt.Errorf("the klusterlet crd %s should be deleted, try remove all finalizers again", klusterletCRDName)
		}, 10*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

// cleanupOrphanedAppliedManifestWork cleans up AppliedManifestWork resources that have
// the klusterlet-works label. These may be orphaned if work-agent was deleted before
// it could process them. See docs/e2e-cleanup-analysis.md for details.
func cleanupOrphanedAppliedManifestWork() {
	ginkgo.By("Clean up orphaned AppliedManifestWork", func() {
		gomega.Eventually(func() error {
			// List all AppliedManifestWork with klusterlet-works label
			amwList, err := hubDynamicClient.Resource(appliedManifestWorkGVR).List(context.TODO(), metav1.ListOptions{
				LabelSelector: "import.open-cluster-management.io/klusterlet-works=true",
			})
			if err != nil {
				return err
			}

			// Delete each orphaned AppliedManifestWork
			for _, amw := range amwList.Items {
				util.Logf("Deleting orphaned AppliedManifestWork: %s", amw.GetName())
				err := hubDynamicClient.Resource(appliedManifestWorkGVR).Delete(context.TODO(), amw.GetName(), metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					return err
				}
			}

			// Verify all are deleted
			amwList, err = hubDynamicClient.Resource(appliedManifestWorkGVR).List(context.TODO(), metav1.ListOptions{
				LabelSelector: "import.open-cluster-management.io/klusterlet-works=true",
			})
			if err != nil {
				return err
			}
			if len(amwList.Items) > 0 {
				return fmt.Errorf("still have %d AppliedManifestWork remaining", len(amwList.Items))
			}
			return nil
		}, 2*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

// deleteKlusterletIfExists deletes the klusterlet CR if it exists.
// In e2e environment (Hub = Spoke), we need to explicitly delete the klusterlet
// to clean up the namespace. See docs/e2e-cleanup-analysis.md for details.
func deleteKlusterletIfExists() {
	ginkgo.By("Delete the klusterlet if exists", func() {
		err := hubOperatorClient.OperatorV1().Klusterlets().Delete(context.TODO(), "klusterlet", metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})
}
