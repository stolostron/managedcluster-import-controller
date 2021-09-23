// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/open-cluster-management/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster manually", func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("cluster-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
	})

	ginkgo.It("Should create the meta object and the import secret of the managed clsuter", func() {
		assertManagedClusterFinalizer(managedClusterName, "managedcluster-import-controller.open-cluster-management.io/cleanup")
		assertManagedClusterCreatedViaAnntation(managedClusterName, "other")
		assertManagedClusterNameLabel(managedClusterName)
		assertManagedClusterNamespaceLabel(managedClusterName)
		assertManagedClusterRBAC(managedClusterName)
		assertManagedClusterImportSecret(managedClusterName)
	})

	ginkgo.It("Should recover the meta objet of the managed cluster", func() {
		assertManagedClusterCreatedViaAnntation(managedClusterName, "other")
		assertManagedClusterNameLabel(managedClusterName)

		ginkgo.By("Modify the managed cluster annotation", func() {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			copied := cluster.DeepCopy()
			copied.Annotations["open-cluster-management/created-via"] = "wrong"
			_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(), copied, metav1.UpdateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after modify", func() {
			assertManagedClusterCreatedViaAnntation(managedClusterName, "other")
			assertManagedClusterNameLabel(managedClusterName)
		})
	})

	ginkgo.It("Should recover the label of the managed cluster namespace", func() {
		assertManagedClusterNamespaceLabel(managedClusterName)

		ginkgo.By("Remove the managed cluster namespace label", func() {
			ns, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			copied := ns.DeepCopy()
			copied.Labels = map[string]string{}
			_, err = hubKubeClient.CoreV1().Namespaces().Update(context.TODO(), copied, metav1.UpdateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after remove", func() { assertManagedClusterNamespaceLabel(managedClusterName) })
	})

	ginkgo.It("Should recover the required rbac of the managed clsuter", func() {
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

	ginkgo.It("Should recover the import secret of the managed clsuter", func() {
		assertManagedClusterImportSecret(managedClusterName)

		name := fmt.Sprintf("%s-import", managedClusterName)
		ginkgo.By(fmt.Sprintf("Remove the managed cluster import secret %s", name), func() {
			err := hubKubeClient.CoreV1().Secrets(managedClusterName).Delete(context.TODO(), name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Recover after delete", func() { assertManagedClusterImportSecret(managedClusterName) })
	})
})
