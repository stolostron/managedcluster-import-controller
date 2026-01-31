// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
)

var _ = ginkgo.Describe("test cleanup resource after a cluster is detached", ginkgo.Label("cleanup"), func() {
	ginkgo.Context("Importing a self managed cluster and detach the cluster", func() {
		var (
			start    time.Time
			caseName string
		)
		const localClusterName = "local-cluster"
		ginkgo.BeforeEach(func() {
			start = time.Now()
			ginkgo.By(fmt.Sprintf("Create managed cluster %s", localClusterName), func() {
				_, err := util.CreateManagedClusterWithShortLeaseDuration(hubClusterClient, localClusterName, nil, util.NewLable("local-cluster", "true"))
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			assertManagedClusterImportSecretCreated(localClusterName, "other")
			assertManagedClusterImportSecretApplied(localClusterName)
			assertManagedClusterAvailable(localClusterName)
			assertManagedClusterManifestWorksAvailable(localClusterName)
		})

		ginkgo.AfterEach(func() {
			// Use assertSelfManagedClusterDeleted for self-managed cluster tests
			assertSelfManagedClusterDeleted(localClusterName)
			defer func() {
				util.Logf("run case: %s, spending time: %.2f seconds", caseName, time.Since(start).Seconds())
			}()
		})

		ginkgo.It("Should delete addons and manifestWorks", func() {
			// apply a manifestWork
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

			// check the work has added finalizer before detaching the cluster
			assertManifestworkFinalizer(localClusterName, manifestwork.Name, "cluster.open-cluster-management.io/manifest-work-cleanup")
			addon := &v1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: localClusterName,
				},
				Spec: v1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "default",
				},
			}
			_, err = addonClient.AddonV1alpha1().ManagedClusterAddOns(localClusterName).Create(context.TODO(), addon, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("wait for the 2 finalizers to be applied for cluster %s", localClusterName))
			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), localClusterName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return len(cluster.Finalizers) > 2
			}, 1*time.Minute, 1*time.Second).ShouldNot(gomega.BeFalse())

			// detach the cluster
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), localClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// the addon and manifestWork should be deleted.
			gomega.Eventually(func() error {
				addons, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(localClusterName).List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				if len(addons.Items) > 0 {
					return fmt.Errorf("addons still exist: %v", addons.Items)
				}
				return nil
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
			gomega.Eventually(func() error {
				allManifestWorks, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				if len(allManifestWorks.Items) > 0 {
					return fmt.Errorf("manifestworks still exist: %v", allManifestWorks.Items)
				}
				return nil
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), localClusterName, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					return nil
				}
				if err == nil {
					return fmt.Errorf("cluster still exists")
				}
				return err
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
			gomega.Eventually(func() error {
				_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), localClusterName, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					return nil
				}
				if err == nil {
					return fmt.Errorf("cluster namespace still exists")
				}
				return err
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should delete addons and manifestWorks by force", func() {
			// apply a manifestWork
			manifestwork := &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon-helloworld-deploy",
					Finalizers: []string{
						"test.open-cluster-management.io/test-delete",
					},
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

			addon := &v1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-addon",
					Finalizers: []string{
						"cluster.open-cluster-management.io/addon-pre-delete",
					},
					Namespace: localClusterName,
				},
				Spec: v1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "default",
				},
			}
			_, err = addonClient.AddonV1alpha1().ManagedClusterAddOns(localClusterName).Create(context.TODO(), addon, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), localClusterName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return len(cluster.Finalizers) > 2
			}, 1*time.Minute, 1*time.Second).ShouldNot(gomega.BeFalse())

			// detach the cluster
			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), localClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// there is addon manifestwork, so wait for the cluster to be unavailable
			ginkgo.By(fmt.Sprintf("wait for the cluster %s to be unavailable", localClusterName), func() {
				gomega.Eventually(func() bool {
					cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), localClusterName, metav1.GetOptions{})
					if err != nil {
						return errors.IsNotFound(err)
					}

					return helpers.IsClusterUnavailable(cluster)
				}, 3*time.Minute, 5*time.Second).ShouldNot(gomega.BeFalse())
			})

			ginkgo.By("the addon should be deleted", func() {
				gomega.Eventually(func() error {
					addons, err := addonClient.AddonV1alpha1().ManagedClusterAddOns(localClusterName).List(context.TODO(), metav1.ListOptions{})
					if err != nil {
						if errors.IsNotFound(err) {
							return nil
						}
						return err
					}
					if len(addons.Items) > 0 {
						return fmt.Errorf("addons still exist: %v", addons.Items)
					}
					return nil
				}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("the manifestworks should be deleted", func() {
				gomega.Eventually(func() error {
					allManifestWorks, err := hubWorkClient.WorkV1().ManifestWorks(localClusterName).List(context.TODO(), metav1.ListOptions{})
					if err != nil {
						if errors.IsNotFound(err) {
							return nil
						}
						return err
					}
					if len(allManifestWorks.Items) > 0 {
						return fmt.Errorf("manifestworks still exist: %v", allManifestWorks.Items)
					}
					return nil
				}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("the managed cluster should be deleted", func() {
				gomega.Eventually(func() error {
					_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), localClusterName, metav1.GetOptions{})
					if errors.IsNotFound(err) {
						return nil
					}
					if err == nil {
						return fmt.Errorf("cluster still exists")
					}
					return err
				}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("the managed cluster namespace should be deleted", func() {
				gomega.Eventually(func() error {
					_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), localClusterName, metav1.GetOptions{})
					if errors.IsNotFound(err) {
						return nil
					}
					if err == nil {
						return fmt.Errorf("cluster namespace still exists")
					}
					return err
				}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())
			})

		})

		// This case will take about several minutes to wait for the cluster state to become unavailable,
		ginkgo.It("should keep the ns when infraenv exists", func() {
			managedClusterName := localClusterName
			assertManagedClusterNamespace(managedClusterName)

			infraGVR := asv1beta1.GroupVersion.WithResource("infraenvs")

			// create a infraenv in the cluster namespace
			infra := &asv1beta1.InfraEnv{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "agent-install.openshift.io/v1beta1",
					Kind:       "InfraEnv",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-infra",
					Namespace: managedClusterName,
				},
				Spec: asv1beta1.InfraEnvSpec{
					PullSecretRef: &corev1.LocalObjectReference{
						Name: "test",
					},
				},
			}

			unstructuredInfra := &unstructured.Unstructured{}
			object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(infra)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			unstructuredInfra.Object = object
			_, err = hubDynamicClient.Resource(infraGVR).Namespace(managedClusterName).Create(context.TODO(), unstructuredInfra, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), managedClusterName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					return err
				}
				return fmt.Errorf("expected no cluster, but got %v", managedClusterName)
			}, 60*time.Second, 3*time.Second).ShouldNot(gomega.HaveOccurred())

			checkCount := 0
			gomega.Eventually(func() error {
				_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				checkCount++
				if checkCount > 4 {
					return nil
				}
				return fmt.Errorf("wait 20s to check if ns is deleted")
			}, 1*time.Minute, 5*time.Second).ShouldNot(gomega.HaveOccurred())

			err = hubDynamicClient.Resource(infraGVR).Namespace(managedClusterName).Delete(context.TODO(), infra.Name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			assertManagedClusterDeletedFromHub(managedClusterName)
		})
	})

})
