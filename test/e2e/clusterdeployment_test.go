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

	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster with clusterdeployment", func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
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

		assertKlusterletNamespaceDeleted()
		assertKlusterletDeleted()
	})

	ginkgo.It(fmt.Sprintf("Should detach the managed cluster %s", managedClusterName), func() {
		ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", managedClusterName), func() {
			err := hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertOnlyManagedClusterDeleted(managedClusterName)
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
