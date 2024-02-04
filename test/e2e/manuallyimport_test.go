// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster manually", func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("import-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
	})

	ginkgo.It("Should import the cluster manually", func() {
		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName))
		_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Get import-secret for managed cluster %s", managedClusterName))
		var importSecret *corev1.Secret
		gomega.Eventually(func() error {
			importSecret, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(),
				fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix), metav1.GetOptions{})
			return err
		}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())

		assertManagedClusterImportSecretCreated(managedClusterName, "other")

		clientHolder := &helpers.ClientHolder{
			KubeClient:          hubKubeClient,
			APIExtensionsClient: crdClient,
			OperatorClient:      hubOperatorClient,
			RuntimeClient:       hubRuntimeClient,
		}
		_, err = helpers.ImportManagedClusterFromSecret(clientHolder, hubMapper, hubRecorder, importSecret)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		assertManagedClusterManifestWorks(managedClusterName)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
		assertManagedClusterPriorityClass(managedClusterName)
	})
})
