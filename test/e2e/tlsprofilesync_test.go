// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var _ = ginkgo.Describe("TLS profile sync sidecar injection", ginkgo.Label("tls"), func() {
	var managedClusterName string

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
	})

	ginkgo.It("Should inject tls-profile-sync sidecar for OpenShift managed cluster",
		ginkgo.Serial, func() {
			managedClusterName = fmt.Sprintf("cluster-ocp-test-%s", rand.String(6))

			ginkgo.By(fmt.Sprintf("Create OpenShift managed cluster %s", managedClusterName), func() {
				_, err := hubClusterClient.ClusterV1().ManagedClusters().Create(
					context.TODO(),
					&clusterv1.ManagedCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: managedClusterName,
							Labels: map[string]string{
								"vendor": "OpenShift",
							},
						},
						Spec: clusterv1.ManagedClusterSpec{
							HubAcceptsClient: true,
						},
					},
					metav1.CreateOptions{},
				)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("Import secret should contain klusterlet deployment with tls-profile-sync sidecar",
				func() {
					gomega.Eventually(func() error {
						secretName := fmt.Sprintf("%s-import", managedClusterName)
						secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(
							context.TODO(), secretName, metav1.GetOptions{})
						if err != nil {
							return err
						}

						importYAML := secret.Data[constants.ImportSecretImportYamlKey]
						if len(importYAML) == 0 {
							return fmt.Errorf("import.yaml is empty")
						}

						for _, yaml := range helpers.SplitYamls(importYAML) {
							obj := helpers.MustCreateObject(yaml)
							dep, ok := obj.(*appsv1.Deployment)
							if !ok || dep.Name != "klusterlet" {
								continue
							}

							// Verify sidecar container exists
							for _, c := range dep.Spec.Template.Spec.Containers {
								if c.Name == "tls-profile-sync" {
									util.Logf("Found tls-profile-sync sidecar with image %s", c.Image)
									return nil
								}
							}
							return fmt.Errorf(
								"klusterlet deployment has %d containers but no tls-profile-sync sidecar",
								len(dep.Spec.Template.Spec.Containers))
						}
						return fmt.Errorf("klusterlet deployment not found in import secret")
					}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
				})
		})

	ginkgo.It("Should NOT inject tls-profile-sync sidecar for non-OpenShift managed cluster",
		ginkgo.Serial, func() {
			managedClusterName = fmt.Sprintf("cluster-k8s-test-%s", rand.String(6))

			ginkgo.By(fmt.Sprintf("Create non-OpenShift managed cluster %s", managedClusterName), func() {
				_, err := hubClusterClient.ClusterV1().ManagedClusters().Create(
					context.TODO(),
					&clusterv1.ManagedCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: managedClusterName,
							Labels: map[string]string{
								"vendor": "Kubernetes",
							},
						},
						Spec: clusterv1.ManagedClusterSpec{
							HubAcceptsClient: true,
						},
					},
					metav1.CreateOptions{},
				)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("Import secret should contain klusterlet deployment without sidecar",
				func() {
					gomega.Eventually(func() error {
						secretName := fmt.Sprintf("%s-import", managedClusterName)
						secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(
							context.TODO(), secretName, metav1.GetOptions{})
						if err != nil {
							return err
						}

						importYAML := secret.Data[constants.ImportSecretImportYamlKey]
						if len(importYAML) == 0 {
							return fmt.Errorf("import.yaml is empty")
						}

						for _, yaml := range helpers.SplitYamls(importYAML) {
							obj := helpers.MustCreateObject(yaml)
							dep, ok := obj.(*appsv1.Deployment)
							if !ok || dep.Name != "klusterlet" {
								continue
							}

							for _, c := range dep.Spec.Template.Spec.Containers {
								if c.Name == "tls-profile-sync" {
									return fmt.Errorf(
										"tls-profile-sync sidecar should not be injected for non-OpenShift cluster")
								}
							}
							if len(dep.Spec.Template.Spec.Containers) != 1 {
								return fmt.Errorf("expected 1 container, got %d",
									len(dep.Spec.Template.Spec.Containers))
							}
							return nil
						}
						return fmt.Errorf("klusterlet deployment not found in import secret")
					}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
				})
		})
})
