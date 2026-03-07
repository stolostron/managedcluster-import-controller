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
	"github.com/stolostron/managedcluster-import-controller/test/e2e-new/framework"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a self managed cluster", ginkgo.Label("core"), func() {
	ginkgo.Context("Importing a local-cluster", func() {
		const localClusterName = "local-cluster"
		var cl *framework.ClusterLifecycle

		ginkgo.BeforeEach(func() {
			cl = framework.ForDefaultMode(hub, localClusterName)

			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hub.KubeClient)

			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedCluster(hub.ClusterClient, localClusterName, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hub.KubeClient)

			cl.Teardown()
		})

		ginkgo.It("Should import the local-cluster", func() {
			hub.AssertImportSecretCreated(localClusterName, "other")
			hub.AssertManifestWorks(localClusterName)
			hub.AssertImportSecretApplied(localClusterName)
			hub.AssertClusterAvailable(localClusterName)
			hub.AssertManifestWorksAvailable(localClusterName)
			hub.AssertPriorityClass(localClusterName)
			hub.AssertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
				"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, true)
		})
	})

	ginkgo.Context("Importing a local-cluster with custom auto-import strategy", func() {
		const localClusterName = "local-cluster"
		var cl *framework.ClusterLifecycle

		ginkgo.BeforeEach(func() {
			cl = framework.ForDefaultMode(hub, localClusterName)

			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hub.KubeClient)

			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedClusterWithShortLeaseDuration(hub.ClusterClient, localClusterName, nil, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			// reset the custom controller config
			util.RemoveControllerConfigConfigMap(hub.KubeClient)

			cl.Teardown()
		})

		ginkgo.It("Should not recover the agent once joined if auto-import strategy is ImportOnly", func() {
			ginkgo.By(fmt.Sprintf("Should import the managed cluster %s successfully", localClusterName), func() {
				hub.AssertImportSecretApplied(localClusterName)
				hub.AssertClusterAvailable(localClusterName)
			})

			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hub.KubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", localClusterName), func() {
				err := util.RemoveKlusterlet(hub.OperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				hub.AssertClusterAvailableUnknown(localClusterName)
			})

			ginkgo.By(fmt.Sprintf("Should not recover the managed cluster %s after deleting import secret", localClusterName), func() {
				err := util.RemoveImportSecret(hub.KubeClient, localClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				hub.AssertImportSecretCreated(localClusterName, "other")
				hub.AssertClusterAvailableUnknownConsistently(localClusterName, 30*time.Second)
			})

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the auto-import strategy is set to ImportAndSync", localClusterName), func() {
				err := util.SetAutoImportStrategy(hub.KubeClient, apiconstants.AutoImportStrategyImportAndSync)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				err = util.RemoveImportSecret(hub.KubeClient, localClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				hub.AssertImportSecretCreated(localClusterName, "other")
				hub.AssertClusterAvailable(localClusterName)
			})
		})

		ginkgo.It("Should trigger auto-import with immediate-import annotation", func() {
			ginkgo.By(fmt.Sprintf("Should import the managed cluster %s successfully", localClusterName), func() {
				hub.AssertClusterAvailable(localClusterName)
			})

			ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
				autoImportStrategy, err := util.GetAutoImportStrategy(hub.KubeClient)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
			})

			ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", localClusterName), func() {
				err := util.RemoveKlusterlet(hub.OperatorClient, "klusterlet")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				hub.AssertClusterAvailableUnknown(localClusterName)
			})

			ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the immediate-import annotation is added", localClusterName), func() {
				err := util.SetImmediateImportAnnotation(hub.ClusterClient, localClusterName, "")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				hub.AssertImportSecretCreated(localClusterName, "other")
				hub.AssertClusterAvailable(localClusterName)
			})

			hub.AssertImmediateImportCompleted(localClusterName)
		})
	})

	ginkgo.Context("Importing a cluster with self managed cluster label", func() {
		var (
			managedClusterName string
			cl                 *framework.ClusterLifecycle
		)

		ginkgo.BeforeEach(func() {
			managedClusterName = fmt.Sprintf("selfmanaged-test-%s", rand.String(6))
			cl = framework.ForDefaultMode(hub, managedClusterName)

			ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
				_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.AfterEach(func() {
			cl.Teardown()
		})

		ginkgo.It("Should import the self managed cluster", func() {
			hub.AssertImportSecretCreated(managedClusterName, "other")

			ginkgo.By(fmt.Sprintf("Add self managed label to managed cluster %s", managedClusterName), func() {
				gomega.Eventually(func() error {
					cluster, err := hub.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					cluster = cluster.DeepCopy()
					cluster.Labels["local-cluster"] = "true"

					_, err = hub.ClusterClient.ClusterV1().ManagedClusters().Update(context.TODO(), cluster, metav1.UpdateOptions{})
					if err != nil {
						return err
					}
					return nil
				}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			hub.AssertManifestWorks(managedClusterName)
			hub.AssertImportSecretApplied(managedClusterName)
			hub.AssertClusterAvailable(managedClusterName)
			hub.AssertManifestWorksAvailable(managedClusterName)
			hub.AssertPriorityClass(managedClusterName)
			hub.AssertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
				"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, true)
		})
	})
})
