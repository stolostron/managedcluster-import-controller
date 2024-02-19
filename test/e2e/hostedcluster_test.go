// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	"open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = ginkgo.Describe("Importing and detaching a managed cluster with hosted mode", ginkgo.Label("hosted"), func() {
	var hostingClusterName string
	ginkgo.BeforeEach(func() {
		hostingClusterName = fmt.Sprintf("hosting-cluster-%s", rand.String(6))
		ginkgo.By(fmt.Sprintf("Create hosting cluster %s", hostingClusterName), func() {
			_, err := util.CreateManagedCluster(hubClusterClient, hostingClusterName,
				util.NewLable("local-cluster", "true"))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertManagedClusterImportSecretCreated(hostingClusterName, "other")
		assertManagedClusterImportSecretApplied(hostingClusterName)
		assertManagedClusterAvailable(hostingClusterName)
		assertManagedClusterManifestWorksAvailable(hostingClusterName)
	})

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(hostingClusterName)
	})

	ginkgo.Context("Import one hosted managed cluster with auto-import-secret", func() {
		var managedClusterName string

		ginkgo.JustBeforeEach(func() {
			managedClusterName = fmt.Sprintf("autoimport-test-hosted-%s", rand.String(6))
			ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
				_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.JustAfterEach(func() {
			assertAutoImportSecretDeleted(managedClusterName)
			assertHostedManagedClusterDeleted(managedClusterName, hostingClusterName)
		})

		ginkgo.It("Should import the cluster with auto-import-secret with kubeconfig", func() {
			ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
				secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName, operatorv1.InstallModeHosted)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create hosted mode managed cluster %s", managedClusterName), func() {
				_, err := util.CreateHostedManagedCluster(hubClusterClient, managedClusterName, hostingClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(managedClusterName, "other", operatorv1.InstallModeHosted)
			assertManagedClusterImportSecretApplied(managedClusterName, operatorv1.InstallModeHosted)
			assertManagedClusterAvailable(managedClusterName)
			assertManagedClusterPriorityClassHosted(managedClusterName)
		})
	})

	ginkgo.Context("Import one hosted managed cluster manually", func() {
		var managedClusterName string

		ginkgo.JustBeforeEach(func() {
			managedClusterName = fmt.Sprintf("autoimport-test-hosted-%s", rand.String(6))
			ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
				_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.JustAfterEach(func() {
			assertHostedManagedClusterDeleted(managedClusterName, hostingClusterName)
		})

		ginkgo.It("Should import the cluster by creating the external managed kubeconfig secret manually", func() {
			ginkgo.By(fmt.Sprintf("Create hosted mode managed cluster %s", managedClusterName), func() {
				_, err := util.CreateHostedManagedCluster(hubClusterClient, managedClusterName, hostingClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(managedClusterName, "other", operatorv1.InstallModeHosted)
			assertHostedManagedClusterManifestWorksAvailable(managedClusterName, hostingClusterName)

			ginkgo.By(fmt.Sprintf("Create external managed kubeconfig %s with kubeconfig", managedClusterName), func() {
				secret, err := util.NewExternalManagedKubeconfigSecret(hubKubeClient, managedClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				namespace := fmt.Sprintf("klusterlet-%s", managedClusterName)
				assertNamespaceCreated(hubKubeClient, namespace)
				_, err = hubKubeClient.CoreV1().Secrets(namespace).Create(
					context.TODO(), secret, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretApplied(managedClusterName, operatorv1.InstallModeHosted)
			assertManagedClusterAvailable(managedClusterName)
			assertManagedClusterPriorityClassHosted(managedClusterName)
		})
	})

	ginkgo.Context("Detach multiple hosted clusters", func() {
		ginkgo.It("should delete each cluster independently", func() {
			// create one hosted cluster
			hosted1 := fmt.Sprintf("test-hosted-%s", rand.String(6))
			ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", hosted1), func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hosted1}}
				_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create hosted mode managed cluster %s", hosted1), func() {
				_, err := util.CreateHostedManagedClusterWithShortLeaseDuration(hubClusterClient, hosted1, hostingClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(hosted1, "other", operatorv1.InstallModeHosted)
			assertHostedKlusterletManifestWorks(hostingClusterName, hosted1)

			// create another hosted cluster
			hosted2 := fmt.Sprintf("test-hosted-%s", rand.String(6))
			ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", hosted2), func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hosted2}}
				_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create hosted mode managed cluster %s", hosted2), func() {
				_, err := util.CreateHostedManagedClusterWithShortLeaseDuration(hubClusterClient, hosted2, hostingClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(hosted2, "other", operatorv1.InstallModeHosted)
			assertHostedKlusterletManifestWorks(hostingClusterName, hosted2)

			// delete the hosted1
			assertHostedManagedClusterDeleted(hosted1, hostingClusterName)

			// the hosted2 works should be exist
			assertHostedKlusterletManifestWorks(hostingClusterName, hosted2)

			// delete the hosted2
			assertHostedManagedClusterDeleted(hosted2, hostingClusterName)
		})
	})

	ginkgo.Context("Cleanup resources after a hosted cluster is detached", func() {
		var managedClusterName string

		ginkgo.JustBeforeEach(func() {
			managedClusterName = fmt.Sprintf("autoimport-test-hosted-%s", rand.String(6))
			ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
				_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create auto-import-secret for managed cluster %s with kubeconfig", managedClusterName), func() {
				secret, err := util.NewAutoImportSecret(hubKubeClient, managedClusterName, operatorv1.InstallModeHosted)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = hubKubeClient.CoreV1().Secrets(managedClusterName).Create(context.TODO(), secret, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By(fmt.Sprintf("Create hosted mode managed cluster %s", managedClusterName), func() {
				_, err := util.CreateHostedManagedClusterWithShortLeaseDuration(hubClusterClient, managedClusterName, hostingClusterName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(managedClusterName, "other", operatorv1.InstallModeHosted)
			assertManagedClusterImportSecretApplied(managedClusterName, operatorv1.InstallModeHosted)
			assertManagedClusterAvailable(managedClusterName)
		})

		ginkgo.JustAfterEach(func() {
			assertAutoImportSecretDeleted(managedClusterName)
			assertHostedManagedClusterDeleted(managedClusterName, hostingClusterName)
		})

		ginkgo.It("Should clean up the addons", func() {
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
		ginkgo.It("Should clean up the addons with finalizer", func() {
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
