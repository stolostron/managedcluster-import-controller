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
	"github.com/stolostron/managedcluster-import-controller/test/e2e-new/framework"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster manually", ginkgo.Label("core"), func() {
	var managedClusterName string
	var cl *framework.ClusterLifecycle

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("import-test-%s", rand.String(6))
		cl = framework.ForDefaultMode(hub, managedClusterName)

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hub.KubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		cl.Teardown()
	})

	ginkgo.It("Should import the cluster manually", func() {
		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName))
		_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Get import-secret for managed cluster %s", managedClusterName))
		var importSecret *corev1.Secret
		gomega.Eventually(func() error {
			importSecret, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(),
				fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix), metav1.GetOptions{})
			return err
		}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())

		hub.AssertImportSecretCreated(managedClusterName, "other")

		clientHolder := &helpers.ClientHolder{
			KubeClient:          hub.KubeClient,
			APIExtensionsClient: hub.CRDClient,
			OperatorClient:      hub.OperatorClient,
			RuntimeClient:       hub.RuntimeClient,
		}
		_, err = helpers.ImportManagedClusterFromSecret(clientHolder, hub.Mapper, hub.Recorder, importSecret)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		hub.AssertManifestWorks(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
		hub.AssertPriorityClass(managedClusterName)
	})
})
