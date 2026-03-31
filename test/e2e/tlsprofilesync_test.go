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
	rbacv1 "k8s.io/api/rbac/v1"
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

			ginkgo.By("Import secret should contain sidecar and RBAC for tls-profile-sync",
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

						foundSidecar := false
						foundClusterRole := false
						foundClusterRoleBinding := false

						for _, yaml := range helpers.SplitYamls(importYAML) {
							obj := helpers.MustCreateObject(yaml)
							switch o := obj.(type) {
							case *appsv1.Deployment:
								if o.Name != "klusterlet" {
									continue
								}
								for _, c := range o.Spec.Template.Spec.Containers {
									if c.Name == "tls-profile-sync" {
										util.Logf("Found tls-profile-sync sidecar with image %s",
											c.Image)
										foundSidecar = true
									}
								}
							case *rbacv1.ClusterRole:
								if o.Name == "open-cluster-management:klusterlet-tls-profile-sync" {
									foundClusterRole = true
								}
							case *rbacv1.ClusterRoleBinding:
								if o.Name == "open-cluster-management:klusterlet-tls-profile-sync" {
									foundClusterRoleBinding = true
								}
							}
						}

						if !foundSidecar {
							return fmt.Errorf("tls-profile-sync sidecar not found in klusterlet deployment")
						}
						if !foundClusterRole {
							return fmt.Errorf("tls-profile-sync ClusterRole not found")
						}
						if !foundClusterRoleBinding {
							return fmt.Errorf("tls-profile-sync ClusterRoleBinding not found")
						}
						return nil
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

			ginkgo.By("Import secret should not contain sidecar or RBAC for tls-profile-sync",
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

						foundDeployment := false
						for _, yaml := range helpers.SplitYamls(importYAML) {
							obj := helpers.MustCreateObject(yaml)
							switch o := obj.(type) {
							case *appsv1.Deployment:
								if o.Name != "klusterlet" {
									continue
								}
								foundDeployment = true
								for _, c := range o.Spec.Template.Spec.Containers {
									if c.Name == "tls-profile-sync" {
										return fmt.Errorf(
											"tls-profile-sync sidecar should not exist for non-OpenShift")
									}
								}
							case *rbacv1.ClusterRole:
								if o.Name == "open-cluster-management:klusterlet-tls-profile-sync" {
									return fmt.Errorf(
										"tls-profile-sync ClusterRole should not exist for non-OpenShift")
								}
							case *rbacv1.ClusterRoleBinding:
								if o.Name == "open-cluster-management:klusterlet-tls-profile-sync" {
									return fmt.Errorf(
										"tls-profile-sync ClusterRoleBinding should not exist for non-OpenShift")
								}
							}
						}
						if !foundDeployment {
							return fmt.Errorf("klusterlet deployment not found")
						}
						return nil
					}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
				})
		})
})
