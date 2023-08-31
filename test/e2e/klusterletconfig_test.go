package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

var _ = Describe("Use KlusterletConfig to customize klusterlet manifests", func() {
	var managedClusterName string
	var tolerationSeconds int64 = 20

	BeforeEach(func() {
		managedClusterName = fmt.Sprintf("klusterletconfig-test-%s", rand.String(6))

		By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
	})

	It("Should deploy the klusterlet with nodePlacement", func() {
		By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithAnnotations(
				hubClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": "test",
					"open-cluster-management/nodeSelector":               "{}",
					"open-cluster-management/tolerations":                "[]",
				},
				util.NewLable("local-cluster", "true"))
			Expect(err).ToNot(HaveOccurred())
		})

		By("Create KlusterletConfig", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					NodePlacement: &operatorv1.NodePlacement{
						NodeSelector: map[string]string{
							"kubernetes.io/os": "linux",
						},
						Tolerations: []corev1.Toleration{
							{
								Key:               "foo",
								Operator:          corev1.TolerationOpExists,
								Effect:            corev1.TaintEffectNoExecute,
								TolerationSeconds: &tolerationSeconds,
							},
						},
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertKlusterletNodePlacement(
			map[string]string{"kubernetes.io/os": "linux"},
			[]corev1.Toleration{{
				Key:               "foo",
				Operator:          corev1.TolerationOpExists,
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: &tolerationSeconds,
			}},
		)

		By("Update KlusterletConfig", func() {
			Eventually(func() error {
				oldkc, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Get(context.TODO(), "test", metav1.GetOptions{})
				if err != nil {
					return err
				}

				newkc := oldkc.DeepCopy()
				newkc.Spec.NodePlacement = &operatorv1.NodePlacement{}
				_, err = klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Update(context.TODO(), newkc, metav1.UpdateOptions{})
				return err
			}, 60*time.Second, 1*time.Second).Should(Succeed())
		})

		// klusterletconfig's nodeplacement is nil, expect to use values in managed cluster annotations which is empty
		assertKlusterletNodePlacement(map[string]string{}, []corev1.Toleration{})

		By("Delete Klusterletconfig", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), "test", metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		// expect default values
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
	})
})
