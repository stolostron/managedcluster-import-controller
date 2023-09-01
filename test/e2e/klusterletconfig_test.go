package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

var _ = Describe("Use KlusterletConfig to customize klusterlet manifests", func() {
	var managedClusterName string
	var klusterletConfigName string
	var tolerationSeconds int64 = 20

	BeforeEach(func() {
		managedClusterName = fmt.Sprintf("klusterletconfig-test-%s", utilrand.String(6))
		klusterletConfigName = fmt.Sprintf("klusterletconfig-%s", utilrand.String(6))

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
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
					"open-cluster-management/nodeSelector":               "{}",
					"open-cluster-management/tolerations":                "[]",
				},
				util.NewLable("local-cluster", "true"))
			Expect(err).ToNot(HaveOccurred())
		})

		By("Create KlusterletConfig", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
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
				oldkc, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Get(context.TODO(), klusterletConfigName, metav1.GetOptions{})
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
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
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

	It("Should deploy the klusterlet with proxy config", func() {
		By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hubClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				},
				util.NewLable("local-cluster", "true"))
			Expect(err).ToNot(HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		assertBootstrapKubeconfigWithProxyConfig("", nil, nil)
		assertManagedClusterAvailable(managedClusterName)

		By("Create KlusterletConfig with http proxy", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPProxy: "http://127.0.0.1:3128",
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertBootstrapKubeconfigWithProxyConfig("http://127.0.0.1:3128", nil, nil)

		// cluster should become offline because there is no proxy server listening on the specified endpoint
		assertManagedClusterOffline(managedClusterName, 120*time.Second)

		proxyCAData, _, err := newCert("proxy server cert")
		Expect(err).ToNot(HaveOccurred())

		By("Update KlusterletConfig with a https proxy", func() {
			Eventually(func() error {
				oldkc, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Get(context.TODO(), klusterletConfigName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				newkc := oldkc.DeepCopy()
				newkc.Spec.HubKubeAPIServerProxyConfig = klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
					HTTPSProxy: "https://127.0.0.1:3129",
					CABundle:   proxyCAData,
				}
				_, err = klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Update(context.TODO(), newkc, metav1.UpdateOptions{})
				return err
			}, 60*time.Second, 1*time.Second).Should(Succeed())
		})

		assertBootstrapKubeconfigWithProxyConfig("https://127.0.0.1:3129", proxyCAData, nil)

		// cluster should be offline because there is no proxy server listening on the specified endpoint
		assertManagedClusterOffline(managedClusterName, 120*time.Second)

		By("Delete Klusterletconfig", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertBootstrapKubeconfigWithProxyConfig("", nil, proxyCAData)

		// delete agent deployment to rebootstrap
		deploys, err := hubKubeClient.AppsV1().Deployments("open-cluster-management-agent").List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		for _, deploy := range deploys.Items {
			if deploy.Name == "klusterlet" {
				continue
			}
			err = hubKubeClient.AppsV1().Deployments(deploy.Namespace).Delete(context.TODO(), deploy.Name, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		}

		// cluster should become available because no proxy is used
		assertManagedClusterAvailable(managedClusterName)
	})
})

func newCert(commoneName string) ([]byte, []byte, error) {
	// set up our CA certificate
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(2019),
		Subject:               pkix.Name{CommonName: commoneName},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	// pem encode
	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return nil, nil, err
	}

	caPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})
	if err != nil {
		return nil, nil, err
	}

	return caPEM.Bytes(), caPrivKeyPEM.Bytes(), nil
}
