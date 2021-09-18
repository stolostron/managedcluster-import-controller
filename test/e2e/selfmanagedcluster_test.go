// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	"github.com/open-cluster-management/managedcluster-import-controller/test/e2e/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = ginkgo.Describe("Importing a self managed cluster", func() {
	ginkgo.Context("Importing a local-cluster", func() {
		const localClusterName = "local-cluster"

		ginkgo.BeforeEach(func() {
			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedCluster(hubClusterClient, localClusterName, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			assertManagedClusterDeleted(localClusterName)
		})

		ginkgo.It("Should import the local-cluster", func() {
			assertManagedClusterImportSecretCreated(localClusterName, "other")
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorks(localClusterName)
		})
	})

	ginkgo.Context("Importing a cluster with self managed cluster label", func() {
		var managedClusterName string

		ginkgo.BeforeEach(func() {
			managedClusterName = fmt.Sprintf("selfmanaged-test-%s", rand.String(6))

			ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
				_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			assertManagedClusterDeleted(managedClusterName)
		})

		ginkgo.It("Should import the self managed cluster", func() {
			assertManagedClusterImportSecretCreated(managedClusterName, "other")

			ginkgo.By(fmt.Sprintf("Add self managed label to managed cluster %s", managedClusterName), func() {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				cluster = cluster.DeepCopy()
				cluster.Labels["local-cluster"] = "true"

				_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(), cluster, metav1.UpdateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretApplied(managedClusterName)
			assertManagedClusterAvailable(managedClusterName)
			assertManagedClusterManifestWorks(managedClusterName)
		})
	})
})
