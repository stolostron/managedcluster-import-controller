// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
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
			_, err := util.CreateManagedClusterWithShortLeaseDuration(hubClusterClient, managedClusterName)
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
		assertManagedClusterManifestWorks(managedClusterName)
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
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
			gomega.Expect(wait.Poll(1*time.Second, 10*time.Minute, func() (bool, error) {
				_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					return true, nil
				}

				return false, err
			})).ToNot(gomega.HaveOccurred())
		})
	})
})

func assertOnlyManagedClusterDeleted(managedClusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert delete the managed cluster %s spending time: %.2f seconds", managedClusterName, time.Since(start).Seconds())
	}()
	ginkgo.By("Should delete the managed cluster", func() {
		gomega.Expect(wait.Poll(1*time.Second, 10*time.Minute, func() (bool, error) {
			_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertKlusterletNamespaceDeleted() {
	start := time.Now()
	defer func() {
		util.Logf("assert delete the open-cluster-management-agent namespace spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should delete the open-cluster-management-agent namespace", func() {
		gomega.Expect(wait.Poll(1*time.Second, 10*time.Minute, func() (bool, error) {
			klusterletNamespace := "open-cluster-management-agent"
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})).ToNot(gomega.HaveOccurred())
	})

}

func assertKlusterletDeleted() {
	start := time.Now()
	ginkgo.By("Should delete the klusterlet crd", func() {
		gomega.Expect(wait.Poll(1*time.Second, 10*time.Minute, func() (bool, error) {
			klusterletCRDName := "klusterlets.operator.open-cluster-management.io"
			crd, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}

			if crd.DeletionTimestamp.IsZero() {
				// klusterlet crd is not in
				return false, nil
			}

			klusterlet, err := hubOperatorClient.OperatorV1().Klusterlets().Get(context.TODO(), "klusterlet", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}

			if klusterlet.DeletionTimestamp.IsZero() {
				return false, nil
			}

			// klusterlet is not deleted yet
			klusterlet = klusterlet.DeepCopy()
			klusterlet.Finalizers = []string{}
			_, err = hubOperatorClient.OperatorV1().Klusterlets().Update(context.TODO(), klusterlet, metav1.UpdateOptions{})
			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}
