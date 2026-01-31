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

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a self managed cluster", ginkgo.Label("core"), func() {
	ginkgo.Context("Importing a local-cluster", func() {
		const localClusterName = "local-cluster"

		ginkgo.BeforeEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hubKubeClient)

			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedCluster(hubClusterClient, localClusterName, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hubKubeClient)

			// Use assertSelfManagedClusterDeleted for self-managed cluster tests
			assertSelfManagedClusterDeleted(localClusterName)
		})

		ginkgo.It("Should import the local-cluster", func() {
			assertManagedClusterImportSecretCreated(localClusterName, "other")
			assertManagedClusterManifestWorks(localClusterName)
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorksAvailable(localClusterName)
			assertManagedClusterPriorityClass(localClusterName)
			assertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
				"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, true)
		})
	})

	ginkgo.Context("Importing a local-cluster with custom auto-import strategy", func() {
		const localClusterName = "local-cluster"

		ginkgo.BeforeEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hubKubeClient)

			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedClusterWithShortLeaseDuration(hubClusterClient, localClusterName, nil, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hubKubeClient)

			// Use assertSelfManagedClusterDeleted for self-managed cluster tests
			assertSelfManagedClusterDeleted(localClusterName)
		})

		ginkgo.It("Should not recover the agent once joined if auto-import strategy is ImportOnly", func() {
			ginkgo.By(fmt.Sprintf("Should import the managed cluster %s successfully", localClusterName), func() {
				assertManagedClusterImportSecretApplied(localClusterName)
				assertManagedClusterAvailable(localClusterName)
			})

			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hubKubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", localClusterName), func() {
				err := util.RemoveKlusterlet(hubOperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				assertManagedClusterAvailableUnknown(localClusterName)
			})

			ginkgo.By(fmt.Sprintf("Should not recover the managed cluster %s after deleting import secret", localClusterName), func() {
				err := util.RemoveImportSecret(hubKubeClient, localClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				assertManagedClusterImportSecretCreated(localClusterName, "other")
				assertManagedClusterAvailableUnknownConsistently(localClusterName, 30*time.Second)
			})

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the auto-import strategy is set to ImportAndSync", localClusterName), func() {
				err := util.SetAutoImportStrategy(hubKubeClient, apiconstants.AutoImportStrategyImportAndSync)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				err = util.RemoveImportSecret(hubKubeClient, localClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				assertManagedClusterImportSecretCreated(localClusterName, "other")
				assertManagedClusterAvailable(localClusterName)
			})
		})

		ginkgo.It("Should trigger auto-import with immediate-import annotation", func() {
			ginkgo.By(fmt.Sprintf("Should import the managed cluster %s successfully", localClusterName), func() {
				assertManagedClusterAvailable(localClusterName)
			})

			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hubKubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", localClusterName), func() {
				err := util.RemoveKlusterlet(hubOperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				assertManagedClusterAvailableUnknown(localClusterName)
			})

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the immediate-import annotation is added", localClusterName), func() {
				err := util.SetImmediateImportAnnotation(hubClusterClient, localClusterName, "")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				assertManagedClusterImportSecretCreated(localClusterName, "other")
				assertManagedClusterAvailable(localClusterName)
			})

			assertImmediateImportCompleted(localClusterName)
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
			// Use assertSelfManagedClusterDeleted for self-managed cluster tests
			assertSelfManagedClusterDeleted(managedClusterName)
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
			assertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
				"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, true)
		})
	})
})
