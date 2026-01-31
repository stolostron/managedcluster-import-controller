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

	apifeature "open-cluster-management.io/api/feature"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

var _ = Describe("Use KlusterletConfig to customize klusterlet manifests", Label("config"), func() {
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
		// reset the custom controller config
		util.RemoveControllerConfigConfigMap(hubKubeClient)

		// Use assertSelfManagedClusterDeleted for self-managed cluster tests (local-cluster=true)
		// which handles klusterlet and CRD cleanup properly
		assertSelfManagedClusterDeleted(managedClusterName)
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

		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
	})

	It("Should deploy the klusterlet with proxy config", func() {
		By("Use ImportAndSync as auto-import strategy", func() {
			err := util.SetAutoImportStrategy(hubKubeClient, apiconstants.AutoImportStrategyImportAndSync)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hubClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				})
			Expect(err).ToNot(HaveOccurred())
		})

		By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName)
			Expect(err).ToNot(HaveOccurred())
			secret.Annotations = map[string]string{
				constants.AnnotationKeepingAutoImportSecret: "true",
			}

			_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		assertBootstrapKubeconfigWithProxyConfig("", nil, nil)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

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

		// here to restart agent pods to trigger bootstrap secret update to save time.
		restartAgentPods()
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

		// here to restart agent pods to trigger bootstrap secret update to save time.
		restartAgentPods()
		// cluster should be offline because there is no proxy server listening on the specified endpoint
		assertManagedClusterOffline(managedClusterName, 120*time.Second)

		By("Delete Klusterletconfig", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertBootstrapKubeconfigWithProxyConfig("", nil, proxyCAData)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		restartAgentPods()

		// cluster should become available because no proxy is used
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
	})

	It("Should ignore the proxy config for self managed cluster", func() {
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
						HTTPSProxy: "http://127.0.0.1:3128",
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertBootstrapKubeconfigConsistently("https://kubernetes.default.svc:443", "",
			"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, true, 30*time.Second)
	})

	It("Should deploy the klusterlet with custom server URL and CA bundle", func() {
		By("Use ImportAndSync as auto-import strategy", func() {
			err := util.SetAutoImportStrategy(hubKubeClient, apiconstants.AutoImportStrategyImportAndSync)
			Expect(err).ToNot(gomega.HaveOccurred())
		})

		By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hubClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				})
			Expect(err).ToNot(HaveOccurred())
		})

		By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName)
			Expect(err).ToNot(HaveOccurred())
			secret.Annotations = map[string]string{
				constants.AnnotationKeepingAutoImportSecret: "true",
			}

			_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		defaultServerUrl, err := bootstrap.GetKubeAPIServerAddress(context.TODO(), hubRuntimeClient, nil)
		Expect(err).ToNot(HaveOccurred())
		defaultCABundle, err := bootstrap.GetBootstrapCAData(context.TODO(), &helpers.ClientHolder{
			KubeClient:    hubKubeClient,
			RuntimeClient: hubRuntimeClient,
		}, defaultServerUrl, managedClusterName, nil)
		Expect(err).ToNot(HaveOccurred())
		assertBootstrapKubeconfig(defaultServerUrl, "", "", defaultCABundle, false)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

		customServerURL := "https://invalid.server.url:6443"
		customCAData, _, err := newCert("custom CA for hub Kube API server")
		Expect(err).ToNot(HaveOccurred())

		By("Create KlusterletConfig with custom server URL & CA bundle", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerURL:      customServerURL,
					HubKubeAPIServerCABundle: customCAData,
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertBootstrapKubeconfig(customServerURL, "", "", customCAData, false)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		restartAgentPods()
		// cluster should become offline because the custom server URL and CA bundle is invalid
		assertManagedClusterOffline(managedClusterName, 120*time.Second)

		By("Delete Klusterletconfig", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertBootstrapKubeconfig(defaultServerUrl, "", "", defaultCABundle, false)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		restartAgentPods()
		// cluster should become available because custom server URL and CA bundle is removed
		assertManagedClusterAvailable(managedClusterName)
	})

	It("Should deploy the klusterlet with custom server URL for self managed cluster", func() {
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
		assertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
			"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, false)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

		customServerURL := "https://invalid.server.url:6443"
		customCAData, _, err := newCert("custom CA for hub Kube API server")
		Expect(err).ToNot(HaveOccurred())

		By("Create KlusterletConfig with custom server URL", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerURL:      customServerURL,
					HubKubeAPIServerCABundle: customCAData,
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})
		assertBootstrapKubeconfig(customServerURL, "", "", customCAData, false)

		// cluster should become offline because the custom server URL and CA bundle is invalid
		assertManagedClusterOffline(managedClusterName, 120*time.Second)

		By("Delete Klusterletconfig and re-create the import secret", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = util.RemoveImportSecret(hubKubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		By(fmt.Sprintf("Should recover the managed cluster %s", managedClusterName), func() {
			err := util.SetImmediateImportAnnotation(hubClusterClient, managedClusterName, "")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			assertManagedClusterAvailable(managedClusterName)
			assertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
				"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, false)
		})
	})

	It("Should deploy the klusterlet with customized namespace", func() {
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
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

		By("Create KlusterletConfig with customized namespace", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					InstallMode: &klusterletconfigv1alpha1.InstallMode{
						Type: klusterletconfigv1alpha1.InstallModeNoOperator,
						NoOperator: &klusterletconfigv1alpha1.NoOperator{
							Postfix: "local",
						},
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		AssertKlusterletNamespace(managedClusterName, "klusterlet-local", "open-cluster-management-local")

		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

		By("Delete Klusterletconfig", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		AssertKlusterletNamespace(managedClusterName, "klusterlet", "open-cluster-management-agent")

		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
	})

	It("Should deploy the klusterlet with custom AppliedManifestWork eviction grace period", func() {
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
		assertAppliedManifestWorkEvictionGracePeriod(nil)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

		By("Create KlusterletConfig with custom AppliedManifestWork eviction grace period", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					AppliedManifestWorkEvictionGracePeriod: "120m",
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertAppliedManifestWorkEvictionGracePeriod(&metav1.Duration{
			Duration: 120 * time.Minute,
		})
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

		By("Delete Klusterletconfig", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertAppliedManifestWorkEvictionGracePeriod(nil)
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)
	})

	It("Should deploy the klusterlet with featuregate", func() {
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
		assertManagedClusterAvailable(managedClusterName)
		assertManagedClusterManifestWorksAvailable(managedClusterName)

		By("Create KlusterletConfig with feature gate", func() {
			_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{
						{
							Feature: string(apifeature.RawFeedbackJsonString),
							Mode:    operatorv1.FeatureGateModeTypeEnable,
						},
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertFeatureGate("klusterlet", nil, []operatorv1.FeatureGate{
			{
				Feature: string(apifeature.RawFeedbackJsonString),
				Mode:    operatorv1.FeatureGateModeTypeEnable,
			}})
		assertManagedClusterAvailable(managedClusterName)

		By("Delete Klusterletconfig", func() {
			err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		assertFeatureGate("klusterlet", nil, nil)
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

func restartAgentPods(namespaces ...string) {
	if len(namespaces) == 0 {
		namespaces = []string{"open-cluster-management-agent"}
	}
	nspodsnum := map[string]int{}
	for _, ns := range namespaces {
		pods, err := hubKubeClient.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "app=klusterlet-agent"})
		Expect(err).ToNot(HaveOccurred())

		nspodsnum[ns] = len(pods.Items)
		for _, pod := range pods.Items {
			err = hubKubeClient.CoreV1().Pods(ns).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		}
	}
	gomega.Eventually(func() error {
		for _, ns := range namespaces {
			pods, err := hubKubeClient.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "app=klusterlet-agent"})
			if err != nil {
				return err
			}
			if len(pods.Items) != nspodsnum[ns] {
				return fmt.Errorf("waiting for pods restart in namespace %s", ns)
			}
		}

		return nil
	}, 120*time.Second, 1*time.Second).Should(gomega.Succeed())

	assertAgentLeaderElection()
}
