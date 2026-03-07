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
	"github.com/stolostron/managedcluster-import-controller/test/e2e-new/framework"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster with clusterdeployment", ginkgo.Label("core"), func() {
	var managedClusterName string
	var cl *framework.ClusterLifecycle

	ginkgo.BeforeEach(func() {
		// reset the custom controller config
		util.RemoveControllerConfigConfigMap(hub.KubeClient)

		managedClusterName = fmt.Sprintf("clusterdeployment-test-%s", rand.String(6))
		cl = framework.ForDefaultMode(hub, managedClusterName)

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hub.KubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(hub.ClusterClient, managedClusterName, nil)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create a clusterdeployment for managed cluster %s", managedClusterName), func() {
			err := util.CreateClusterDeployment(hub.DynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Install the clusterdeployment %s", managedClusterName), func() {
			gomega.Eventually(func() error {
				return util.InstallClusterDeployment(hub.KubeClient, hub.DynamicClient, managedClusterName)
			}, 30, 1).ShouldNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(managedClusterName, "hive")

		ginkgo.By(fmt.Sprintf("removed create-via hive annotation from %s, and the annotation will be added back",
			managedClusterName), func() {
			gomega.Eventually(func() error {
				cluster, err := hub.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(),
					managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				delete(cluster.Annotations, "open-cluster-management/created-via")
				_, err = hub.ClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(),
					cluster, metav1.UpdateOptions{})
				return err
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())

			hub.AssertClusterCreatedVia(managedClusterName, "hive")
		})

		hub.AssertManifestWorks(managedClusterName)
		hub.AssertImportSecretApplied(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
		hub.AssertPriorityClass(managedClusterName)
	})

	ginkgo.Context("with custom auto-import-strategy", func() {
		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hub.KubeClient)

			ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
				err := util.DeleteClusterDeployment(hub.DynamicClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
			cl.Teardown()
		})

		ginkgo.It("Should not recover the agent once joined if auto-import strategy is ImportOnly", func() {
			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hub.KubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", managedClusterName), func() {
				err := util.RemoveKlusterlet(hub.OperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				hub.AssertClusterAvailableUnknown(managedClusterName)
			})

			ginkgo.By(fmt.Sprintf("Should not recover the managed cluster %s after deleting import secret", managedClusterName), func() {
				err := util.RemoveImportSecret(hub.KubeClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				hub.AssertImportSecretCreated(managedClusterName, "hive")
				hub.AssertClusterAvailableUnknownConsistently(managedClusterName, 30*time.Second)
			})

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the auto-import strategy is set to ImportAndSync", managedClusterName), func() {
				err := util.SetAutoImportStrategy(hub.KubeClient, apiconstants.AutoImportStrategyImportAndSync)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				err = util.RemoveImportSecret(hub.KubeClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				hub.AssertImportSecretCreated(managedClusterName, "hive")
				hub.AssertClusterAvailable(managedClusterName)
			})
		})
	})

	ginkgo.Context("with immediate-import annotation", func() {
		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hub.KubeClient)

			ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
				err := util.DeleteClusterDeployment(hub.DynamicClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
			cl.Teardown()
		})

		ginkgo.It("Should trigger auto-import with immediate-import annotation", func() {
			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hub.KubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", managedClusterName), func() {
				err := util.RemoveKlusterlet(hub.OperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				hub.AssertClusterAvailableUnknown(managedClusterName)
			})

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the immediate-import annotation is added", managedClusterName), func() {
				err := util.SetImmediateImportAnnotation(hub.ClusterClient, managedClusterName, "")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				hub.AssertImportSecretCreated(managedClusterName, "hive")

				// Wait for leader election before checking the managed cluster status.
				// The initial import always triggers a rolling update, and the new pod must be leader
				// before the managed cluster can become available. See test/e2e/README.md for details.
				hub.EnsureAgentReady()
				hub.AssertClusterAvailable(managedClusterName)
			})

			hub.AssertImmediateImportCompleted(managedClusterName)
		})
	})

	ginkgo.It(fmt.Sprintf("Should destroy the managed cluster %s", managedClusterName), func() {
		// Wait for leader election before deleting the ManagedCluster. The initial
		// import always triggers a rolling update, and the new pod must be leader
		// before cleanup can proceed correctly. See test/e2e/README.md for details.
		hub.EnsureAgentReady()

		ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
			err := util.DeleteClusterDeployment(hub.DynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", managedClusterName), func() {
			err := hub.ClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertClusterDeletedFromHub(managedClusterName)

		assertKlusterletNamespaceDeleted()
		assertKlusterletDeleted()
	})

	ginkgo.It(fmt.Sprintf("Should detach the managed cluster %s", managedClusterName), func() {
		// Wait for leader election before deleting the ManagedCluster. The initial
		// import always triggers a rolling update, and the new pod must be leader
		// before cleanup can proceed correctly. See test/e2e/README.md for details.
		hub.EnsureAgentReady()

		ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", managedClusterName), func() {
			err := hub.ClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertOnlyManagedClusterDeleted(managedClusterName)
		assertKlusterletNamespaceDeleted()
		assertKlusterletDeleted()

		ginkgo.By("Should have the managed cluster namespace", func() {
			_, err := hub.KubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Delete the clusterdeployment %s", managedClusterName), func() {
			err := util.DeleteClusterDeployment(hub.DynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Should delete the managed cluster namespace", func() {
			gomega.Eventually(func() error {
				_, err := hub.KubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
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
			_, err := hub.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
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
			_, err := hub.KubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
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
			crd, err := hub.CRDClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			if crd.DeletionTimestamp.IsZero() {
				return fmt.Errorf("the klusterlet crd %s deletionTimestamp should not be zero", klusterletCRDName)
			}

			klusterlet, err := hub.OperatorClient.OperatorV1().Klusterlets().Get(context.TODO(), "klusterlet", metav1.GetOptions{})
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
			_, err = hub.OperatorClient.OperatorV1().Klusterlets().Update(context.TODO(), klusterlet, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return fmt.Errorf("the klusterlet crd %s should be deleted, try remove all finalizers again", klusterletCRDName)
		}, 10*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}
