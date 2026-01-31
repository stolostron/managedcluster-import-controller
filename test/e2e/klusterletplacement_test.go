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

	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Adding node placement to the klusterlet", ginkgo.Label("config"), func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("klusterlet-node-placement-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		// Use assertSelfManagedClusterDeleted for self-managed cluster tests
		assertSelfManagedClusterDeleted(managedClusterName)
	})

	ginkgo.It("Should deploy the klusterlet without node placement", func() {
		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedClusterWithAnnotations(
				hubClusterClient,
				managedClusterName,
				map[string]string{
					"open-cluster-management/nodeSelector": "{}",
					"open-cluster-management/tolerations":  "[]",
				},
				util.NewLable("local-cluster", "true"),
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertManagedClusterImportSecretCreated(managedClusterName, "other")
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertKlusterletNodePlacement(map[string]string{}, []corev1.Toleration{})
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
	})

	ginkgo.It("Should update the klusterlet node placement", func() {
		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedClusterWithAnnotations(
				hubClusterClient,
				managedClusterName,
				// the klusterlet cannot be started with the unsatisfied nodeSelector and tolerations
				map[string]string{
					"open-cluster-management/nodeSelector": "{\"kubernetes.io/os\":\"test\"}",
					"open-cluster-management/tolerations":  "[{\"key\":\"foo\",\"operator\":\"Exists\",\"effect\":\"NoExecute\",\"tolerationSeconds\":20}]",
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		var tolerationSeconds int64 = 20

		assertManagedClusterImportSecretCreated(managedClusterName, "other")
		assertManagedClusterImportSecretApplied(managedClusterName)
		assertKlusterletNodePlacement(
			map[string]string{"kubernetes.io/os": "test"},
			[]corev1.Toleration{{
				Key:               "foo",
				Operator:          corev1.TolerationOpExists,
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: &tolerationSeconds,
			}},
		)

		ginkgo.By(fmt.Sprintf("Remove the annotations of managed cluster %s", managedClusterName), func() {
			gomega.Eventually(func() error {
				return util.RemoveManagedClusterAnnotations(hubClusterClient, managedClusterName)
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		assertKlusterletNodePlacement(
			map[string]string{},
			[]corev1.Toleration{
				{
					Effect:   corev1.TaintEffectNoSchedule,
					Key:      "node-role.kubernetes.io/infra",
					Operator: corev1.TolerationOpExists,
				},
			},
		)

		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
	})
})
