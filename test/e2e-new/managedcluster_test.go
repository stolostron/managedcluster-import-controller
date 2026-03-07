// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/test/e2e-new/framework"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster manually", ginkgo.Label("core"), func() {
	var managedClusterName string
	var cl *framework.ClusterLifecycle

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("cluster-test-%s", rand.String(6))
		cl = framework.ForCreatedOnly(hub, managedClusterName)

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
		ginkgo.By(fmt.Sprintf("enable cluster import config secret in cluster %s", managedClusterName), func() {
			err := util.SetClusterImportConfig(hub.KubeClient)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		cl.Teardown()
	})

	ginkgo.It("Should create the meta object and the import secret of the managed cluster", ginkgo.Serial, func() {
		hub.AssertClusterFinalizer(managedClusterName, "managedcluster-import-controller.open-cluster-management.io/cleanup")
		hub.AssertClusterCreatedVia(managedClusterName, "other")
		hub.AssertClusterNameLabel(managedClusterName)
		hub.AssertClusterNamespaceLabel(managedClusterName)
		hub.AssertClusterRBAC(managedClusterName)
		hub.AssertImportSecret(managedClusterName)
	})

	ginkgo.It("Should recover the meta objet of the managed cluster", ginkgo.Serial, func() {
		hub.AssertClusterCreatedVia(managedClusterName, "other")
		hub.AssertClusterNameLabel(managedClusterName)

		ginkgo.By("Modify the managed cluster annotation", func() {
			gomega.Eventually(func() error {
				cluster, err := hub.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				copied := cluster.DeepCopy()
				copied.Annotations["open-cluster-management/created-via"] = "wrong"
				_, err = hub.ClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(), copied, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		ginkgo.By("Recover after modify", func() {
			hub.AssertClusterCreatedVia(managedClusterName, "other")
			hub.AssertClusterNameLabel(managedClusterName)
		})
	})

	ginkgo.It("Should recover the label of the managed cluster namespace", ginkgo.Serial, func() {
		hub.AssertClusterNamespaceLabel(managedClusterName)

		ginkgo.By("Remove the managed cluster namespace label", func() {
			gomega.Eventually(func() error {
				ns, err := hub.KubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				copied := ns.DeepCopy()
				delete(copied.Labels, "cluster.open-cluster-management.io/managedCluster")
				_, err = hub.KubeClient.CoreV1().Namespaces().Update(context.TODO(), copied, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		ginkgo.By("Recover after remove", func() { hub.AssertClusterNamespaceLabel(managedClusterName) })
	})

	ginkgo.It("Should recover the required rbac of the managed cluster", ginkgo.Serial, func() {
		hub.AssertClusterRBAC(managedClusterName)

		ginkgo.By("Remove the managed cluster rbac", func() {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			err := hub.KubeClient.RbacV1().ClusterRoles().Delete(context.TODO(), name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			err = hub.KubeClient.RbacV1().ClusterRoleBindings().Delete(context.TODO(), name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			saname := fmt.Sprintf("%s-bootstrap-sa", managedClusterName)
			err = hub.KubeClient.CoreV1().ServiceAccounts(managedClusterName).Delete(context.TODO(), saname, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after delete", func() { hub.AssertClusterRBAC(managedClusterName) })
	})

	ginkgo.It("Should recover the import secret of the managed cluster", ginkgo.Serial, func() {
		hub.AssertImportSecret(managedClusterName)

		name := fmt.Sprintf("%s-import", managedClusterName)
		ginkgo.By(fmt.Sprintf("Remove the managed cluster import secret %s", name), func() {
			err := hub.KubeClient.CoreV1().Secrets(managedClusterName).Delete(context.TODO(), name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after delete", func() { hub.AssertImportSecret(managedClusterName) })
	})

	ginkgo.It("Should recover the cluster import config secret of the managed cluster", ginkgo.Serial, func() {
		hub.AssertClusterImportConfigSecret(managedClusterName)

		ginkgo.By(fmt.Sprintf("Remove the cluster import config secret %s", managedClusterName), func() {
			err := hub.KubeClient.CoreV1().Secrets(managedClusterName).Delete(context.TODO(), constants.ClusterImportConfigSecretName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after delete", func() { hub.AssertClusterImportConfigSecret(managedClusterName) })
	})
})
