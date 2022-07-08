// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
)

var _ = ginkgo.Describe("test cleanup resource after a cluster is detached", func() {
	ginkgo.Context("Importing a self managed cluster and detach the cluster", func() {
		var (
			start    time.Time
			caseName string
		)
		const localClusterName = "local-cluster"
		ginkgo.BeforeEach(func() {
			start = time.Now()
			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedClusterWithShortLeaseDuration(hubClusterClient, localClusterName, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(localClusterName, "other")
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorksAvailable(localClusterName)
		})

		ginkgo.AfterEach(func() {
			assertManagedClusterDeleted(localClusterName)
			defer func() {
				util.Logf("run case: %s, spending time: %.2f seconds", caseName, time.Since(start).Seconds())
			}()
		})

		ginkgo.It("Should not delete addon manifest", func() {
			caseName = "do not delete addon manifest in default mode"
			// apply an add manifestWork
			manifestwork := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "addon-helloworld-deploy",
					Namespace: localClusterName,
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							{
								RawExtension: runtime.RawExtension{Raw: []byte("{\"apiVersion\": \"v1\", \"kind\": " +
									"\"Namespace\", \"metadata\": {\"name\": \"open-cluster-management-agent-addon\"}}")},
							},
						},
					},
				},
			}
			_, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).Create(context.TODO(), manifestwork, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// detach the cluster
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), localClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// addon manifestWork should not be deleted
			checkCount := 0
			gomega.Eventually(func() error {
				_, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).Get(context.TODO(), manifestwork.GetName(), metav1.GetOptions{})
				if err != nil {
					return err
				}
				checkCount++
				if checkCount > 4 {
					return nil
				}
				return fmt.Errorf("wait 20s to check if manifestwork is deleted")
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())

			err = hubWorkClient.WorkV1().ManifestWorks(localClusterName).Delete(context.TODO(), manifestwork.GetName(), metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				allManifestWorks, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				if len(allManifestWorks.Items) == 0 {
					return nil
				}
				return fmt.Errorf("all of the manifestworks should be deleted")
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("should clean up the addons", func() {
			caseName = "clean up the addons in default mode"
			managedClusterName := localClusterName
			assertManagedClusterNamespace(managedClusterName)
			// deploy an addon
			addon := &v1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: managedClusterName,
				},
				Spec: v1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "default",
				},
			}
			_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Create(context.TODO(), addon, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("wait for the 2 finalizers to be applied for cluster %s", managedClusterName))
			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return len(cluster.Finalizers) > 2
			}, 1*time.Minute, 1*time.Second).ShouldNot(gomega.BeFalse())

			ginkgo.By(fmt.Sprintf("detach the cluster %s after the finalizers are applied", managedClusterName))
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// the addon manifestWork should be deleted.
			ginkgo.By(fmt.Sprintf("the addon manifestWork %s for cluster %s should be deleted", addon.Name, managedClusterName))
			gomega.Eventually(func() error {
				_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.TODO(), addon.Name, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no addon, but got %v", addon.Name)
			}, 6*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
		})

		// This case will take about several minutes to wait for the cluster state to become unavailable,
		ginkgo.It("should clean up the addons with finalizer", func() {
			caseName = "clean up the addons with finalizer in default mode"
			start := time.Now()
			defer func() {
				util.Logf("run case: clean up the addons with finalizer, spending time: %.2f seconds", time.Since(start).Seconds())
			}()

			managedClusterName := localClusterName
			assertManagedClusterNamespace(managedClusterName)
			// deploy an addon with finalizer
			addon := &v1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{
						"cluster.open-cluster-management.io/addon-pre-delete",
					},
					Name:      "test-addon",
					Namespace: managedClusterName,
				},
				Spec: v1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "default",
				},
			}
			_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Create(context.TODO(), addon, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// apply an add manifestWork
			manifestwork := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "addon-helloworld-deploy",
					Namespace: managedClusterName,
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							{
								RawExtension: runtime.RawExtension{Raw: []byte("{\"apiVersion\": \"v1\", \"kind\": " +
									"\"Namespace\", \"metadata\": {\"name\": \"open-cluster-management-agent-addon\"}}")},
							},
						},
					},
				},
			}
			_, err = hubWorkClient.WorkV1().ManifestWorks(managedClusterName).Create(context.TODO(), manifestwork, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("wait for the 2 finalizers to be applied for cluster %s", managedClusterName))
			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return len(cluster.Finalizers) > 2
			}, 1*time.Minute, 1*time.Second).ShouldNot(gomega.BeFalse())

			ginkgo.By(fmt.Sprintf("detach the cluster %s after the finalizers are applied", managedClusterName))
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// there is addon manifestwork, so wait for the cluster to be unavailable
			ginkgo.By(fmt.Sprintf("wait for the cluster %s to be unavailable", managedClusterName))
			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return errors.IsNotFound(err)
				}

				return helpers.IsClusterUnavailable(cluster)
			}, 10*time.Minute, 5*time.Second).ShouldNot(gomega.BeFalse())

			// the addon should be force deleted when the cluster is unavailable
			ginkgo.By(fmt.Sprintf("the addon %s for cluster %s should be deleted", addon.Name, managedClusterName))
			gomega.Eventually(func() error {
				_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.TODO(), addon.Name, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no addon, but got %v", addon.Name)
			}, 30*time.Second, 3*time.Second).ShouldNot(gomega.HaveOccurred())

			// the addon manifestWork should be force deleted when the cluster is unavailable
			ginkgo.By(fmt.Sprintf("the addon manifestWork %s for cluster %s should be deleted", manifestwork.Name, managedClusterName))
			gomega.Eventually(func() error {
				_, err := hubWorkClient.WorkV1().ManifestWorks(managedClusterName).Get(context.TODO(), manifestwork.Name, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no addon manifestwork, but got %v", manifestwork.Name)
			}, 30*time.Second, 3*time.Second).ShouldNot(gomega.HaveOccurred())
		})
	})

})

var _ = ginkgo.Describe("test cleanup resource after a cluster is detached in Hosted mode", func() {
	var (
		start    time.Time
		caseName string
	)

	ginkgo.Context("Local-cluster as hosting cluster", func() {
		const localClusterName = "local-cluster"
		var managedClusterName string
		ginkgo.BeforeEach(func() {
			start = time.Now()
			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedCluster(hubClusterClient, localClusterName, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(localClusterName, "other")
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorksAvailable(localClusterName)
		})

		ginkgo.JustBeforeEach(func() {
			managedClusterName = fmt.Sprintf("autoimport-test-hosted-%s", rand.String(6))
			ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
				_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
				secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName, constants.KlusterletDeployModeHosted)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create hosted mode managed cluster %s", managedClusterName), func() {
				_, err := util.CreateHostedManagedClusterWithShortLeaseDuration(hubClusterClient, managedClusterName, localClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(managedClusterName, "other", constants.KlusterletDeployModeHosted)
			assertManagedClusterImportSecretApplied(managedClusterName, constants.KlusterletDeployModeHosted)
			assertManagedClusterAvailable(managedClusterName)
		})

		ginkgo.JustAfterEach(func() {
			assertAutoImportSecretDeleted(managedClusterName)
			assertHostedManagedClusterDeleted(managedClusterName, localClusterName)
		})

		ginkgo.AfterEach(func() {
			assertManagedClusterDeleted(localClusterName)
			defer func() {
				util.Logf("run case: %s, spending time: %.2f seconds", caseName, time.Since(start).Seconds())
			}()
		})

		ginkgo.It("should clean up the addons", func() {
			caseName = "clean up the addons in hosted mode"
			assertManagedClusterNamespace(managedClusterName)
			// deploy an addon
			addon := &v1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: managedClusterName,
				},
				Spec: v1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "default",
				},
			}
			_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Create(context.TODO(), addon, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// wait for the 2 finalizers to be applied
			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return len(cluster.Finalizers) > 2
			}, 1*time.Minute, 1*time.Second).ShouldNot(gomega.BeFalse())

			// detach the cluster
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// the addon manifestWork should be deleted.
			gomega.Eventually(func() error {
				_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.TODO(), addon.Name, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no addon, but got %v", addon.Name)
			}, 6*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
		})

		// This case will take about several minutes to wait for the managed cluster state to become unavailable,
		ginkgo.It("should clean up the addons with finalizer", func() {
			caseName = "clean up the addons with finalizer in hosted mode"
			assertManagedClusterNamespace(managedClusterName)
			// deploy an addon with finalizer
			addon := &v1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{
						"cluster.open-cluster-management.io/addon-pre-delete",
					},
					Name:      "test-addon",
					Namespace: managedClusterName,
				},
				Spec: v1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "default",
				},
			}
			_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Create(context.TODO(), addon, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// apply an add manifestWork
			manifestwork := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "addon-helloworld-deploy",
					Namespace: managedClusterName,
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							{
								RawExtension: runtime.RawExtension{Raw: []byte("{\"apiVersion\": \"v1\", \"kind\": " +
									"\"Namespace\", \"metadata\": {\"name\": \"open-cluster-management-agent-addon\"}}")},
							},
						},
					},
				},
			}
			_, err = hubWorkClient.WorkV1().ManifestWorks(managedClusterName).Create(context.TODO(), manifestwork, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("wait for the 2 finalizers to be applied for cluster %s", managedClusterName))
			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return len(cluster.Finalizers) > 2
			}, 1*time.Minute, 1*time.Second).ShouldNot(gomega.BeFalse())

			ginkgo.By(fmt.Sprintf("detach the cluster %s after the finalizers are applied", managedClusterName))
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// there is addon manifestwork, so wait for the cluster to be unavailable
			ginkgo.By(fmt.Sprintf("wait for the cluster %s to be unavailable", managedClusterName))
			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return errors.IsNotFound(err)
				}

				return helpers.IsClusterUnavailable(cluster)
			}, 5*time.Minute, 5*time.Second).ShouldNot(gomega.BeFalse())

			// the addon should be force deleted when the cluster is unavailable
			ginkgo.By(fmt.Sprintf("the addon %s for cluster %s should be deleted", addon.Name, managedClusterName))
			gomega.Eventually(func() error {
				_, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.TODO(), addon.Name, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no addon, but got %v", addon.Name)
			}, 30*time.Second, 3*time.Second).ShouldNot(gomega.HaveOccurred())

			// the addon manifestWork should be force deleted when the cluster is unavailable
			ginkgo.By(fmt.Sprintf("the addon manifestWork %s for cluster %s should be deleted", manifestwork.Name, managedClusterName))
			gomega.Eventually(func() error {
				_, err := hubWorkClient.WorkV1().ManifestWorks(managedClusterName).Get(context.TODO(), manifestwork.Name, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no addon manifestwork, but got %v", manifestwork.Name)
			}, 30*time.Second, 3*time.Second).ShouldNot(gomega.HaveOccurred())
		})
	})
})
