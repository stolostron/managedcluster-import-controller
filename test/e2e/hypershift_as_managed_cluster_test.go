// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"
	"github.com/open-cluster-management/managedcluster-import-controller/test/e2e/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = ginkgo.Describe("Importing a managed cluster with hostedcluster", func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("hostedcluster-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(hubClusterClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create hostedcluster namespace %s", fmt.Sprintf("%s-%s", managedClusterName, managedClusterName)), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%s", managedClusterName, managedClusterName)}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create a hostedcluster for managed cluster %s", managedClusterName), func() {
			err := util.CreateHostedCluster(hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Install the hostedcluster %s", managedClusterName), func() {
			err := util.InstallHostedCluster(hubKubeClient, hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertManagedClusterImportSecretCreated(managedClusterName, "")
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorks(managedClusterName)
	})

	ginkgo.It(fmt.Sprintf("Should destroy the managed cluster %s", managedClusterName), func() {
		ginkgo.By(fmt.Sprintf("Delete the hostedcluster %s", managedClusterName), func() {
			err := util.DeleteHostedCluster(hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Delete the hostedcluster namespace %s", fmt.Sprintf("%s-%s", managedClusterName, managedClusterName)), func() {
			err := hubKubeClient.CoreV1().Namespaces().Delete(context.TODO(), fmt.Sprintf("%s-%s", managedClusterName, managedClusterName), metav1.DeleteOptions{})
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

		ginkgo.By(fmt.Sprintf("Delete the hostedcluster %s", managedClusterName), func() {
			err := util.DeleteHostedCluster(hubDynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Delete the hostedcluster namespace %s", fmt.Sprintf("%s-%s", managedClusterName, managedClusterName)), func() {
			err := hubKubeClient.CoreV1().Namespaces().Delete(context.TODO(), fmt.Sprintf("%s-%s", managedClusterName, managedClusterName), metav1.DeleteOptions{})
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
