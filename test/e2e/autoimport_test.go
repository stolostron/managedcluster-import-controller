// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = ginkgo.Describe("Importing a managed cluster with auto-import-secret", func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("autoimport-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
	})

	ginkgo.It("Should import the cluster with auto-import-secret with kubeconfig", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertManagedClusterImportSecretCreated(managedClusterName, "other")
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorks(managedClusterName)

		assertAutoImportSecretDeleted(managedClusterName)
	})

	ginkgo.It("Should import the cluster with auto-import-secret with token", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with token", managedClusterName), func() {
			secret, err := util.NewAutoImportSecretWithToken(hubKubeClient, hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertManagedClusterImportSecretCreated(managedClusterName, "other")
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorks(managedClusterName)

		assertAutoImportSecretDeleted(managedClusterName)
	})

	ginkgo.It("Should delete the invalid auto-import-secret after retry times exceeded", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with token", managedClusterName), func() {
			secret, err := util.NewInvalidAutoImportSecret(hubKubeClient, hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Managed cluster should not be imported", func() {
			gomega.Expect(wait.Poll(1*time.Second, 60*time.Second, func() (bool, error) {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return false, err
				}

				return meta.IsStatusConditionFalse(cluster.Status.Conditions, "ManagedClusterImportSucceeded"), nil
			})).ToNot(gomega.HaveOccurred())
		})

		assertAutoImportSecretDeleted(managedClusterName)
	})

	ginkgo.It("Should keep the auto-import-secret after the cluster was imported", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			secret.Annotations = map[string]string{"managedcluster-import-controller.open-cluster-management.io/keeping-auto-import-secret": ""}

			_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertManagedClusterImportSecretCreated(managedClusterName, "other")
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorks(managedClusterName)

		ginkgo.By(fmt.Sprintf("Should keep the auto-import-secret in managed cluster namespace %s", managedClusterName), func() {
			gomega.Consistently(func() error {
				_, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
				return err
			}, 30*time.Second, 1*time.Second).ShouldNot(gomega.HaveOccurred())
		})
	})
})

var _ = ginkgo.Describe("Importing a managed cluster with auto-import-secret for Hosted mode", func() {
	ginkgo.Context("Local-cluster as management cluster", func() {
		const localClusterName = "local-cluster"
		var managedClusterName string
		ginkgo.BeforeEach(func() {
			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedCluster(hubClusterClient, localClusterName, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(localClusterName, "other")
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorks(localClusterName)
		})

		ginkgo.JustBeforeEach(func() {
			managedClusterName = fmt.Sprintf("autoimport-test-hosted-%s", rand.String(6))
			ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
				_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.JustAfterEach(func() {
			assertAutoImportSecretDeleted(managedClusterName)
			assertHostedManagedClusterDeleted(managedClusterName, localClusterName)
		})

		ginkgo.AfterEach(func() {
			assertManagedClusterDeleted(localClusterName)
		})

		ginkgo.It("Should import the cluster with auto-import-secret with kubeconfig", func() {
			ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
				secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName, constants.KlusterletDeployModeHosted)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create hosted mode managed cluster %s", managedClusterName), func() {
				_, err := util.CreateHostedManagedCluster(hubClusterClient, managedClusterName, localClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(managedClusterName, "other", constants.KlusterletDeployModeHosted)
			assertManagedClusterImportSecretApplied(managedClusterName, constants.KlusterletDeployModeHosted)
			assertManagedClusterAvailable(managedClusterName)
		})

	})
})
