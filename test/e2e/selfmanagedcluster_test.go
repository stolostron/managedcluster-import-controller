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

	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
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
			assertManagedClusterManifestWorks(localClusterName)
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorksAvailable(localClusterName)
			assertManagedClusterPriorityClass(localClusterName)
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
				gomega.Eventually(func() error {
					cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					cluster = cluster.DeepCopy()
					cluster.Labels["local-cluster"] = "true"

					_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(), cluster, metav1.UpdateOptions{})
					if err != nil {
						return err
					}
					return nil
				}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			assertManagedClusterManifestWorks(managedClusterName)
			assertManagedClusterImportSecretApplied(managedClusterName)
			assertManagedClusterAvailable(managedClusterName)
			assertManagedClusterManifestWorksAvailable(managedClusterName)
			assertManagedClusterPriorityClass(managedClusterName)
		})
	})
})
