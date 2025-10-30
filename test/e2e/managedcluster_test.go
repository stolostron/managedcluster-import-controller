// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// Trigger e2e run

package e2e

import (
	"context"
	"fmt"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

// ginkgo.Serial promise specs not run in parallel
var _ = ginkgo.Describe("Importing a managed cluster manually", ginkgo.Label("core"), func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("cluster-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
		ginkgo.By(fmt.Sprintf("enable cluster import config secret in cluster %s", managedClusterName), func() {
			err := util.SetClusterImportConfig(hubKubeClient)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
	})

	ginkgo.It("Should create the meta object and the import secret of the managed cluster", ginkgo.Serial, func() {
		assertManagedClusterFinalizer(managedClusterName, "managedcluster-import-controller.open-cluster-management.io/cleanup")
		assertManagedClusterCreatedViaAnnotation(managedClusterName, "other")
		assertManagedClusterNameLabel(managedClusterName)
		assertManagedClusterNamespaceLabel(managedClusterName)
		assertManagedClusterRBAC(managedClusterName)
		assertManagedClusterImportSecret(managedClusterName)
	})

	ginkgo.It("Should recover the meta objet of the managed cluster", ginkgo.Serial, func() {
		assertManagedClusterCreatedViaAnnotation(managedClusterName, "other")
		assertManagedClusterNameLabel(managedClusterName)

		ginkgo.By("Modify the managed cluster annotation", func() {
			gomega.Eventually(func() error {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				copied := cluster.DeepCopy()
				copied.Annotations["open-cluster-management/created-via"] = "wrong"
				_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(), copied, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		ginkgo.By("Recover after modify", func() {
			assertManagedClusterCreatedViaAnnotation(managedClusterName, "other")
			assertManagedClusterNameLabel(managedClusterName)
		})
	})

	ginkgo.It("Should recover the label of the managed cluster namespace", ginkgo.Serial, func() {
		assertManagedClusterNamespaceLabel(managedClusterName)

		ginkgo.By("Remove the managed cluster namespace label", func() {
			gomega.Eventually(func() error {
				ns, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				copied := ns.DeepCopy()
				delete(copied.Labels, "cluster.open-cluster-management.io/managedCluster")
				_, err = hubKubeClient.CoreV1().Namespaces().Update(context.TODO(), copied, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		ginkgo.By("Recover after remove", func() { assertManagedClusterNamespaceLabel(managedClusterName) })
	})

	ginkgo.It("Should recover the required rbac of the managed cluster", ginkgo.Serial, func() {
		assertManagedClusterRBAC(managedClusterName)

		ginkgo.By("Remove the managed cluster rbac", func() {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			err := hubKubeClient.RbacV1().ClusterRoles().Delete(context.TODO(), name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			err = hubKubeClient.RbacV1().ClusterRoleBindings().Delete(context.TODO(), name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			saname := fmt.Sprintf("%s-bootstrap-sa", managedClusterName)
			err = hubKubeClient.CoreV1().ServiceAccounts(managedClusterName).Delete(context.TODO(), saname, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after delete", func() { assertManagedClusterRBAC(managedClusterName) })
	})

	ginkgo.It("Should recover the import secret of the managed cluster", ginkgo.Serial, func() {
		assertManagedClusterImportSecret(managedClusterName)

		name := fmt.Sprintf("%s-import", managedClusterName)
		ginkgo.By(fmt.Sprintf("Remove the managed cluster import secret %s", name), func() {
			err := hubKubeClient.CoreV1().Secrets(managedClusterName).Delete(context.TODO(), name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after delete", func() { assertManagedClusterImportSecret(managedClusterName) })
	})

	ginkgo.It("Should recover the cluster import config secret of the managed cluster", ginkgo.Serial, func() {
		assertClusterImportConfigSecret(managedClusterName)

		ginkgo.By(fmt.Sprintf("Remove the cluster import config secret %s", managedClusterName), func() {
			err := hubKubeClient.CoreV1().Secrets(managedClusterName).Delete(context.TODO(), constants.ClusterImportConfigSecretName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after delete", func() { assertClusterImportConfigSecret(managedClusterName) })
	})

})
