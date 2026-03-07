// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/test/e2e-new/framework"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Importing a managed cluster with auto-import-secret", ginkgo.Label("core"), func() {
	var managedClusterName string
	var cl *framework.ClusterLifecycle

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("autoimport-test-%s", rand.String(6))

		// reset the custom controller config
		util.RemoveControllerConfigConfigMap(hub.KubeClient)

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

	ginkgo.It("Should import the cluster with auto-import-secret with kubeconfig", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(managedClusterName, "other")
		hub.AssertManifestWorks(managedClusterName)
		hub.AssertImportSecretApplied(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
		hub.AssertPriorityClass(managedClusterName)

		hub.AssertAutoImportSecretDeleted(managedClusterName)
	})

	ginkgo.It("Should not import the cluster if auto-import is disabled", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s and disable auto import", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedClusterWithAnnotations(
				hub.ClusterClient,
				managedClusterName,
				map[string]string{
					apiconstants.DisableAutoImportAnnotation: "",
				},
				util.NewLable("local-cluster", "true"))

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(managedClusterName, "other")
		hub.AssertManifestWorks(managedClusterName)
		hub.AssertImportSecretNotApplied(managedClusterName)

		ginkgo.By(fmt.Sprintf("Update managed cluster %s and enable auto import", managedClusterName), func() {
			err := util.RemoveManagedClusterAnnotations(hub.ClusterClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(managedClusterName, "other")
		hub.AssertImportSecretApplied(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
		hub.AssertPriorityClass(managedClusterName)

		hub.AssertAutoImportSecretDeleted(managedClusterName)
	})

	ginkgo.It("Should not recover the agent once joined if auto-import strategy is ImportOnly", func() {
		ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
			autoImportStrategy, err := util.GetAutoImportStrategy(hub.KubeClient)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
		})

		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			secret.Annotations = map[string]string{constants.AnnotationKeepingAutoImportSecret: ""}

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Import managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedClusterWithShortLeaseDuration(hub.ClusterClient, managedClusterName, nil, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			hub.AssertImportSecretApplied(managedClusterName)
			hub.AssertClusterAvailable(managedClusterName)
		})

		ginkgo.By(fmt.Sprintf("Should keep the auto-import-secret in managed cluster namespace %s", managedClusterName), func() {
			gomega.Consistently(func() error {
				_, err := hub.KubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
				return err
			}, 15*time.Second, 1*time.Second).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", managedClusterName), func() {
			err := util.RemoveKlusterlet(hub.OperatorClient, "klusterlet")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			hub.AssertClusterAvailableUnknown(managedClusterName)
		})

		ginkgo.By(fmt.Sprintf("Should not recover the managed cluster %s after deleting import secret", managedClusterName), func() {
			err := util.RemoveImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			hub.AssertImportSecretCreated(managedClusterName, "other")
			hub.AssertClusterAvailableUnknownConsistently(managedClusterName, 30*time.Second)
		})

		ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the auto-import strategy is set to ImportAndSync", managedClusterName), func() {
			err := util.SetAutoImportStrategy(hub.KubeClient, apiconstants.AutoImportStrategyImportAndSync)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = util.RemoveImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			hub.AssertImportSecretCreated(managedClusterName, "other")
			hub.AssertClusterAvailable(managedClusterName)
		})
	})

	ginkgo.It("Should trigger auto-import with immediate-import annotation", func() {
		ginkgo.By("Ensure the auto-import strategy is ImportOnly", func() {
			autoImportStrategy, err := util.GetAutoImportStrategy(hub.KubeClient)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(autoImportStrategy).To(gomega.BeEquivalentTo(apiconstants.AutoImportStrategyImportOnly))
		})

		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			secret.Annotations = map[string]string{constants.AnnotationKeepingAutoImportSecret: ""}

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Import managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedClusterWithShortLeaseDuration(hub.ClusterClient, managedClusterName, nil, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			hub.AssertClusterAvailable(managedClusterName)
		})

		ginkgo.By(fmt.Sprintf("Should keep the auto-import-secret in managed cluster namespace %s", managedClusterName), func() {
			gomega.Consistently(func() error {
				_, err := hub.KubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
				return err
			}, 15*time.Second, 1*time.Second).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Should become offline after removing klusterlet of the managed cluster %s", managedClusterName), func() {
			err := util.RemoveKlusterlet(hub.OperatorClient, "klusterlet")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			hub.AssertClusterAvailableUnknown(managedClusterName)
		})

		ginkgo.By(fmt.Sprintf("Should recover the managed cluster %s once the immediate-import annotation is added", managedClusterName), func() {
			err := util.SetImmediateImportAnnotation(hub.ClusterClient, managedClusterName, "")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			hub.AssertImportSecretCreated(managedClusterName, "other")
			hub.AssertClusterAvailable(managedClusterName)
		})

		hub.AssertImmediateImportCompleted(managedClusterName)
	})

	ginkgo.It("Should import the cluster with auto-import-secret with token", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with token", managedClusterName), func() {
			secret, err := util.NewAutoImportSecretWithToken(hub.KubeClient, hub.DynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(managedClusterName, "other")
		hub.AssertManifestWorks(managedClusterName)
		hub.AssertImportSecretApplied(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
		hub.AssertPriorityClass(managedClusterName)

		hub.AssertAutoImportSecretDeleted(managedClusterName)
	})

	ginkgo.It("Should keep the auto-import-secret after the cluster was imported", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			secret.Annotations = map[string]string{constants.AnnotationKeepingAutoImportSecret: ""}

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(managedClusterName, "other")
		hub.AssertManifestWorks(managedClusterName)
		hub.AssertImportSecretApplied(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
		hub.AssertPriorityClass(managedClusterName)

		ginkgo.By(fmt.Sprintf("Should keep the auto-import-secret in managed cluster namespace %s", managedClusterName), func() {
			gomega.Consistently(func() error {
				_, err := hub.KubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
				return err
			}, 30*time.Second, 1*time.Second).ShouldNot(gomega.HaveOccurred())
		})
	})

	ginkgo.It("Should only update the bootstrap secret", func() {
		ginkgo.By("Use ImportAndSync as auto-import strategy", func() {
			err := util.SetAutoImportStrategy(hub.KubeClient, apiconstants.AutoImportStrategyImportAndSync)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with token", managedClusterName), func() {
			secret, err := util.NewAutoImportSecretWithToken(hub.KubeClient, hub.DynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			// using a local cluster to speed up cluster deletion
			_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(managedClusterName, "other")
		hub.AssertManifestWorks(managedClusterName)
		hub.AssertImportSecretApplied(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)
		hub.AssertPriorityClass(managedClusterName)

		hub.AssertAutoImportSecretDeleted(managedClusterName)

		ginkgo.By(fmt.Sprintf("Create restore auto-import-secret for managed cluster %s", managedClusterName), func() {
			secret, err := util.NewRestoreAutoImportSecret(hub.KubeClient, hub.DynamicClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(
				context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		// make sure no errors happened
		ginkgo.By(fmt.Sprintf("Managed cluster %s should be imported", managedClusterName), func() {
			gomega.Consistently(func() bool {
				cluster, err := hub.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					util.Logf("failed to get cluster %v", err)
					return false
				}

				return meta.IsStatusConditionTrue(
					cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
			}, time.Second*30, time.Second*1).Should(gomega.BeTrue())
		})

		hub.AssertAutoImportSecretDeleted(managedClusterName)
	})

	ginkgo.It("Should auto import the cluster with config", func() {
		ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
			secret, err := util.NewAutoImportSecret(hub.KubeClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = hub.KubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Create local cluster so we have the operator running", func() {
			_, err := util.CreateManagedCluster(hub.ClusterClient, managedClusterName, util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
		hub.AssertImportSecretApplied(managedClusterName)
		hub.AssertClusterAvailable(managedClusterName)
		hub.AssertManifestWorksAvailable(managedClusterName)

		configName := "autoimport-config"
		testcluster := fmt.Sprintf("custom-%s", managedClusterName)
		ginkgo.By("Create KlusterletConfig with customized namespace", func() {
			_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(), &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: configName,
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

		ginkgo.By("Create managed cluster", func() {
			_, err := util.CreateManagedClusterWithShortLeaseDuration(
				hub.ClusterClient,
				testcluster,
				map[string]string{
					"agent.open-cluster-management.io/klusterlet-config": configName,
				},
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		hub.AssertImportSecretCreated(testcluster, "other")
		hub.AssertImportSecretApplied(testcluster)
		hub.AssertClusterAvailable(testcluster)
		klusterletName := fmt.Sprintf("%s-klusterlet", testcluster)
		hub.AssertManifestWorkFinalizer(testcluster, klusterletName, "cluster.open-cluster-management.io/manifest-work-cleanup")

		hub.AssertKlusterletNamespace(testcluster, "klusterlet-local", "open-cluster-management-local")

		ginkgo.By(fmt.Sprintf("Delete the hosted mode managed cluster %s", testcluster), func() {
			err := hub.ClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), testcluster, metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
		hub.AssertClusterDeletedFromHub(testcluster)

		err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Delete(context.TODO(), configName, metav1.DeleteOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})
})
