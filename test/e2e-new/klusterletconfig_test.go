// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e-new/framework"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

var _ = ginkgo.Describe("Use KlusterletConfig to customize klusterlet manifests", ginkgo.Label("config"), func() {
	var managedClusterName string
	var klusterletConfigName string
	var tolerationSeconds int64 = 20
	var cl *framework.ClusterLifecycle

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("klusterletconfig-test-%s", utilrand.String(6))
		klusterletConfigName = fmt.Sprintf("klusterletconfig-%s", utilrand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hub.KubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		cl = framework.ForDefaultMode(hub, managedClusterName)
	})

	ginkgo.AfterEach(func() {
		// reset the custom controller config
		util.RemoveControllerConfigConfigMap(hub.KubeClient)

		cl.Teardown()
	})

	ginkgo.It("Should deploy the klusterlet with nodePlacement", func() {
		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithAnnotations(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
					"open-cluster-management/nodeSelector":               "{}",
					"open-cluster-management/tolerations":                "[]",
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Create KlusterletConfig", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
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
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertKlusterletNodePlacement(
			map[string]string{"kubernetes.io/os": "linux"},
			[]corev1.Toleration{{
				Key:               "foo",
				Operator:          corev1.TolerationOpExists,
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: &tolerationSeconds,
			}},
		)

		ginkgo.By("Update KlusterletConfig", func() {
			gomega.Eventually(func() error {
				oldkc, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Get(context.TODO(), klusterletConfigName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				newkc := oldkc.DeepCopy()
				newkc.Spec.NodePlacement = &operatorv1.NodePlacement{}
				_, err = hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Update(context.TODO(), newkc, metav1.UpdateOptions{})
				return err
			}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		// klusterletconfig's nodeplacement is nil, expect to use values in managed cluster annotations which is empty
		hub.AssertKlusterletNodePlacement(map[string]string{}, []corev1.Toleration{})

		ginkgo.By("Delete Klusterletconfig", func() {
			err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// expect default values
		hub.AssertKlusterletNodePlacement(
			map[string]string{},
			[]corev1.Toleration{
				{
					Effect:   corev1.TaintEffectNoSchedule,
					Key:      "node-role.kubernetes.io/infra",
					Operator: corev1.TolerationOpExists,
				},
			},
		)

		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
	})

	ginkgo.It("Should deploy the klusterlet with proxy config", func() {
		ginkgo.By("Use ImportAndSync as auto-import strategy", func() {
			err := util.SetAutoImportStrategy(hub.KubeClient, apiconstants.AutoImportStrategyImportAndSync)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			secret.Annotations = map[string]string{
				constants.AnnotationKeepingAutoImportSecret: "true",
			}

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		hub.AssertBootstrapKubeconfigWithProxy("", nil, nil)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		ginkgo.By("Create KlusterletConfig with http proxy", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPProxy: "http://127.0.0.1:3128",
					},
				},
			}, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertBootstrapKubeconfigWithProxy("http://127.0.0.1:3128", nil, nil)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		hub.RestartAgentPods()
		// cluster should become offline because there is no proxy server listening on the specified endpoint
		hub.AssertClusterOffline(managedClusterName, 120*time.Second)

		proxyCAData, _, err := newCert("proxy server cert")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By("Update KlusterletConfig with a https proxy", func() {
			gomega.Eventually(func() error {
				oldkc, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Get(context.TODO(), klusterletConfigName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				newkc := oldkc.DeepCopy()
				newkc.Spec.HubKubeAPIServerProxyConfig = klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
					HTTPSProxy: "https://127.0.0.1:3129",
					CABundle:   proxyCAData,
				}
				_, err = hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Update(context.TODO(), newkc, metav1.UpdateOptions{})
				return err
			}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		hub.AssertBootstrapKubeconfigWithProxy("https://127.0.0.1:3129", proxyCAData, nil)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		hub.RestartAgentPods()
		// cluster should be offline because there is no proxy server listening on the specified endpoint
		hub.AssertClusterOffline(managedClusterName, 120*time.Second)

		ginkgo.By("Delete Klusterletconfig", func() {
			err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertBootstrapKubeconfigWithProxy("", nil, proxyCAData)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		hub.RestartAgentPods()

		// cluster should become available because no proxy is used
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
	})

	ginkgo.It("Should ignore the proxy config for self managed cluster", func() {
		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		hub.AssertBootstrapKubeconfigWithProxy("", nil, nil)
		hub.AssertClusterAvailable(managedClusterName)

		ginkgo.By("Create KlusterletConfig with http proxy", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "http://127.0.0.1:3128",
					},
				},
			}, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertBootstrapKubeconfigConsistently("https://kubernetes.default.svc:443", "",
			"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, true, 30*time.Second)
	})

	ginkgo.It("Should deploy the klusterlet with custom server URL and CA bundle", func() {
		ginkgo.By("Use ImportAndSync as auto-import strategy", func() {
			err := util.SetAutoImportStrategy(hub.KubeClient, apiconstants.AutoImportStrategyImportAndSync)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			secret.Annotations = map[string]string{
				constants.AnnotationKeepingAutoImportSecret: "true",
			}

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		defaultServerUrl, err := bootstrap.GetKubeAPIServerAddress(context.TODO(), hub.RuntimeClient, nil)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defaultCABundle, err := bootstrap.GetBootstrapCAData(context.TODO(), &helpers.ClientHolder{
			KubeClient:    hub.KubeClient,
			RuntimeClient: hub.RuntimeClient,
		}, defaultServerUrl, managedClusterName, nil)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		hub.AssertBootstrapKubeconfig(defaultServerUrl, "", "", defaultCABundle, false)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		customServerURL := "https://invalid.server.url:6443"
		customCAData, _, err := newCert("custom CA for hub Kube API server")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By("Create KlusterletConfig with custom server URL & CA bundle", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerURL:      customServerURL,
					HubKubeAPIServerCABundle: customCAData,
				},
			}, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertBootstrapKubeconfig(customServerURL, "", "", customCAData, false)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		hub.RestartAgentPods()
		// cluster should become offline because the custom server URL and CA bundle is invalid
		hub.AssertClusterOffline(managedClusterName, 120*time.Second)

		ginkgo.By("Delete Klusterletconfig", func() {
			err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertBootstrapKubeconfig(defaultServerUrl, "", "", defaultCABundle, false)

		// here to restart agent pods to trigger bootstrap secret update to save time.
		hub.RestartAgentPods()
		// cluster should become available because custom server URL and CA bundle is removed
		hub.AssertClusterAvailable(managedClusterName)
	})

	ginkgo.It("Should deploy the klusterlet with custom server URL for self managed cluster", func() {
		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		hub.AssertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
			"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, false)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		customServerURL := "https://invalid.server.url:6443"
		customCAData, _, err := newCert("custom CA for hub Kube API server")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By("Create KlusterletConfig with custom server URL", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerURL:      customServerURL,
					HubKubeAPIServerCABundle: customCAData,
				},
			}, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
		hub.AssertBootstrapKubeconfig(customServerURL, "", "", customCAData, false)

		// cluster should become offline because the custom server URL and CA bundle is invalid
		hub.AssertClusterOffline(managedClusterName, 120*time.Second)

		ginkgo.By("Delete Klusterletconfig and re-create the import secret", func() {
			err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = util.RemoveImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// Restart agent pods to escape CrashLoopBackOff from the invalid server URL.
		// The invalid URL causes the agent to crash repeatedly with increasing backoff
		// delays. Without restarting, the pod may take 10+ minutes to retry, causing
		// assertManagedClusterAvailable to time out.
		hub.RestartAgentPods()

		ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s", managedClusterName), func() {
			err := util.SetImmediateImportAnnotation(hub.ClusterClient, managedClusterName, "")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			hub.AssertClusterAvailable(managedClusterName)
			hub.AssertBootstrapKubeconfig("https://kubernetes.default.svc:443", "",
				"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil, false)
		})
	})

	ginkgo.It("Should deploy the klusterlet with customized namespace", func() {
		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		ginkgo.By("Create KlusterletConfig with customized namespace", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
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
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertKlusterletNamespace(managedClusterName, "klusterlet-local", "open-cluster-management-local")

		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		ginkgo.By("Delete Klusterletconfig", func() {
			err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertKlusterletNamespace(managedClusterName, "klusterlet", "open-cluster-management-agent")

		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
	})

	ginkgo.It("Should deploy the klusterlet with custom AppliedManifestWork eviction grace period", func() {
		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		hub.AssertAppliedManifestWorkEvictionGracePeriod(nil)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		ginkgo.By("Create KlusterletConfig with custom AppliedManifestWork eviction grace period", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: klusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					AppliedManifestWorkEvictionGracePeriod: "120m",
				},
			}, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertAppliedManifestWorkEvictionGracePeriod(&metav1.Duration{
			Duration: 120 * time.Minute,
		})
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		ginkgo.By("Delete Klusterletconfig", func() {
			err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertAppliedManifestWorkEvictionGracePeriod(nil)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
	})

	ginkgo.It("Should deploy the klusterlet with featuregate", func() {
		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": klusterletConfigName,
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// klusterletconfig is missing and it will be ignored
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		ginkgo.By("Create KlusterletConfig with feature gate", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
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
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertFeatureGate("klusterlet", nil, []operatorv1.FeatureGate{
			{
				Feature: string(apifeature.RawFeedbackJsonString),
				Mode:    operatorv1.FeatureGateModeTypeEnable,
			}})
		hub.AssertClusterAvailable(managedClusterName)

		ginkgo.By("Delete Klusterletconfig", func() {
			err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), klusterletConfigName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertFeatureGate("klusterlet", nil, nil)
		hub.AssertClusterAvailable(managedClusterName)
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
