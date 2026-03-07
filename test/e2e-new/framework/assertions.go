// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package framework

import (
	"context"
	"crypto/x509"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	certutil "k8s.io/client-go/util/cert"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
)

// ---------------------------------------------------------------------------
// Composite assertions
// ---------------------------------------------------------------------------

// AssertImportSecretCreated verifies all metadata and the import secret for a managed cluster.
func (h *Hub) AssertImportSecretCreated(clusterName, createdVia string, mode ...operatorv1.InstallMode) {
	h.AssertClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/cleanup")
	h.AssertClusterCreatedVia(clusterName, createdVia)
	h.AssertClusterNameLabel(clusterName)
	h.AssertClusterNamespaceLabel(clusterName)
	h.AssertClusterRBAC(clusterName)
	if len(mode) != 0 && mode[0] == operatorv1.InstallModeHosted {
		h.AssertHostedImportSecret(clusterName)
	} else {
		h.AssertImportSecret(clusterName)
	}
}

// ---------------------------------------------------------------------------
// Cluster metadata assertions
// ---------------------------------------------------------------------------

// AssertClusterFinalizer waits for the managed cluster to have the expected finalizer.
func (h *Hub) AssertClusterFinalizer(clusterName, expected string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should have expected finalizer: %s", clusterName, expected), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for _, finalizer := range cluster.Finalizers {
				if finalizer == expected {
					return nil
				}
			}
			return fmt.Errorf("managed cluster %s does not have expected finalizer %s", clusterName, expected)
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterCreatedVia waits for the managed cluster to have the created-via annotation.
func (h *Hub) AssertClusterCreatedVia(clusterName, expected string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should have expected annotation: %s", clusterName, expected), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			annotation, ok := cluster.Annotations["open-cluster-management/created-via"]
			if !ok {
				return fmt.Errorf("managed cluster %s does not have created-via annotation", clusterName)
			}
			if annotation != expected {
				return fmt.Errorf("managed cluster %s created-via: want %s, got %s", clusterName, expected, annotation)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterNameLabel waits for the managed cluster to have the name label.
func (h *Hub) AssertClusterNameLabel(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should have cluster name label", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			name, ok := cluster.Labels["name"]
			if !ok || name != clusterName {
				return fmt.Errorf("managed cluster %s: want label name=%s, got %s", clusterName, clusterName, name)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterNamespaceLabel waits for the managed cluster namespace to have the cluster label.
func (h *Hub) AssertClusterNamespaceLabel(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster namespace %s should have cluster label", clusterName), func() {
		gomega.Eventually(func() error {
			ns, err := h.KubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			name, ok := ns.Labels["cluster.open-cluster-management.io/managedCluster"]
			if !ok || name != clusterName {
				return fmt.Errorf("namespace %s: want managedCluster label=%s, got %s", clusterName, clusterName, name)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterRBAC verifies the bootstrap RBAC resources exist.
func (h *Hub) AssertClusterRBAC(clusterName string) {
	ginkgo.By("Should have clusterrole", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", clusterName)
			_, err := h.KubeClient.RbacV1().ClusterRoles().Get(context.TODO(), name, metav1.GetOptions{})
			return err
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("Should have clusterrolebinding", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", clusterName)
			_, err := h.KubeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), name, metav1.GetOptions{})
			return err
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("Should have bootstrap sa", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-bootstrap-sa", clusterName)
			_, err := h.KubeClient.CoreV1().ServiceAccounts(clusterName).Get(context.TODO(), name, metav1.GetOptions{})
			return err
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterNamespace waits for the managed cluster namespace to be created.
func (h *Hub) AssertClusterNamespace(clusterName string) {
	ginkgo.By("Should create the managedCluster namespace", func() {
		gomega.Eventually(func() error {
			_, err := h.KubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
			return err
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertNamespaceCreated waits for a namespace to be created.
func (h *Hub) AssertNamespaceCreated(kubeClient kubernetes.Interface, namespace string) {
	ginkgo.By(fmt.Sprintf("Namespace %s should be created", namespace), func() {
		gomega.Eventually(func() error {
			_, err := kubeClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
			return err
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// ---------------------------------------------------------------------------
// Import secret assertions
// ---------------------------------------------------------------------------

// AssertImportSecret verifies the import secret exists and is valid.
func (h *Hub) AssertImportSecret(clusterName string) {
	start := time.Now()
	defer func() {
		Logf("assert managed cluster import secret spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should create the import secret", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-import", clusterName)
			secret, err := h.KubeClient.CoreV1().Secrets(clusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := helpers.ValidateImportSecret(secret); err != nil {
				return fmt.Errorf("invalid import secret: %v", err)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertHostedImportSecret verifies the hosted-mode import secret exists and is valid.
func (h *Hub) AssertHostedImportSecret(clusterName string) {
	ginkgo.By("Should create the import secret", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-import", clusterName)
			secret, err := h.KubeClient.CoreV1().Secrets(clusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := helpers.ValidateImportSecret(secret); err != nil {
				return fmt.Errorf("invalid import secret: %v", err)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterImportConfigSecret verifies the cluster import config secret.
func (h *Hub) AssertClusterImportConfigSecret(clusterName string) {
	start := time.Now()
	defer func() {
		Logf("assert managed cluster import secret spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should create the cluster import config secret", func() {
		gomega.Eventually(func() error {
			secret, err := h.KubeClient.CoreV1().Secrets(clusterName).Get(
				context.TODO(), constants.ClusterImportConfigSecretName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := helpers.ValidateClusterImportConfigSecret(secret); err != nil {
				return fmt.Errorf("invalid cluster import config secret: %v", err)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertImportSecretApplied waits for the import secret to be applied.
func (h *Hub) AssertImportSecretApplied(clusterName string, mode ...operatorv1.InstallMode) {
	start := time.Now()
	defer func() {
		Logf("assert managed cluster %s import secret applied spending time: %.2f seconds",
			clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be imported", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get cluster error: %v", err)
			}

			Logf("assert managed cluster %s import secret applied conditions: %v",
				clusterName, cluster.Status.Conditions)

			if len(mode) != 0 && mode[0] == operatorv1.InstallModeHosted &&
				meta.IsStatusConditionTrue(cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded) {
				return nil
			}

			if helpers.ImportingResourcesApplied(meta.FindStatusCondition(
				cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)) ||
				meta.IsStatusConditionTrue(cluster.Status.Conditions,
					constants.ConditionManagedClusterImportSucceeded) {
				return nil
			}

			return fmt.Errorf("managed cluster %s import not applied", clusterName)
		}, 5*time.Minute, 30*time.Second).Should(gomega.Succeed())
	})
}

// AssertImportSecretNotApplied verifies the import secret is NOT applied (consistently).
func (h *Hub) AssertImportSecretNotApplied(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should not be imported", clusterName), func() {
		gomega.Consistently(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get cluster error: %v", err)
			}

			condition := meta.FindStatusCondition(
				cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
			if condition == nil {
				return nil
			}

			if condition.Reason == constants.ConditionReasonManagedClusterWaitForImporting {
				return nil
			}

			return fmt.Errorf("managed cluster %s import should not be applied", clusterName)
		}, 15*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertAutoImportSecretDeleted waits for the auto-import-secret to be deleted.
func (h *Hub) AssertAutoImportSecretDeleted(clusterName string) {
	start := time.Now()
	defer func() {
		Logf("assert delete auto-import-secret from %s spending time: %.2f seconds",
			clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Should delete the auto-import-secret from managed cluster namespace %s", clusterName), func() {
		gomega.Eventually(func() error {
			_, err := h.KubeClient.CoreV1().Secrets(clusterName).Get(
				context.TODO(), "auto-import-secret", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("auto-import-secret is not deleted")
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// ---------------------------------------------------------------------------
// Cluster status assertions
// ---------------------------------------------------------------------------

// AssertClusterAvailable waits for the managed cluster to be available.
// It ensures agent leader election is complete first.
func (h *Hub) AssertClusterAvailable(clusterName string) {
	h.EnsureAgentReady()

	start := time.Now()
	defer func() {
		Logf("assert managed cluster %s available spending time: %.2f seconds",
			clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be available", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if meta.IsStatusConditionTrue(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
				return nil
			}
			return fmt.Errorf("cluster %s not available, conditions: %v",
				clusterName, cluster.Status.Conditions)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterAvailableUnknown waits for the managed cluster to have Unknown availability.
func (h *Hub) AssertClusterAvailableUnknown(clusterName string) {
	start := time.Now()
	defer func() {
		Logf("assert managed cluster %s available unknown spending time: %.2f seconds",
			clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be available unknown", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if meta.IsStatusConditionPresentAndEqual(cluster.Status.Conditions,
				clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
				return nil
			}
			return fmt.Errorf("cluster %s not unknown, conditions: %v",
				clusterName, cluster.Status.Conditions)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterAvailableUnknownConsistently verifies the cluster stays in Unknown state.
func (h *Hub) AssertClusterAvailableUnknownConsistently(clusterName string, duration time.Duration) {
	start := time.Now()
	defer func() {
		Logf("assert managed cluster %s available unknown consistently spending time: %.2f seconds",
			clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be available unknown for %v", clusterName, duration), func() {
		gomega.Consistently(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if meta.IsStatusConditionPresentAndEqual(cluster.Status.Conditions,
				clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
				return nil
			}
			return fmt.Errorf("cluster %s not unknown consistently, conditions: %v",
				clusterName, cluster.Status.Conditions)
		}, duration, 2*time.Second).Should(gomega.Succeed())
	})
}

// AssertClusterOffline waits for the managed cluster to go offline (Unknown availability).
// It ensures agent leader election is complete first.
func (h *Hub) AssertClusterOffline(clusterName string, timeout time.Duration) {
	h.EnsureAgentReady()

	ginkgo.By(fmt.Sprintf("Managed cluster %s should be offline", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if meta.IsStatusConditionPresentAndEqual(cluster.Status.Conditions,
				clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
				return nil
			}
			return fmt.Errorf("cluster %s not offline, conditions: %v",
				clusterName, cluster.Status.Conditions)
		}, timeout, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertImmediateImportCompleted waits for the immediate-import annotation to be completed.
func (h *Hub) AssertImmediateImportCompleted(clusterName string) {
	start := time.Now()
	defer func() {
		Logf("assert immediate-import of %s completed spending time: %.2f seconds",
			clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("The immediate-import annotation of Managed cluster %s should be completed", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			v := cluster.Annotations["import.open-cluster-management.io/immediate-import"]
			if v == "Completed" {
				return nil
			}
			return fmt.Errorf("immediate-import annotation value: %v", v)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

// ---------------------------------------------------------------------------
// Cluster deletion assertions
// ---------------------------------------------------------------------------

// AssertClusterDeleted performs a full managed cluster deletion with leader election safety.
// This is the primary method for AfterEach cleanup in default-mode tests.
func (h *Hub) AssertClusterDeleted(clusterName string) {
	h.EnsureAgentReady()

	ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", clusterName), func() {
		err := h.ClusterClient.ClusterV1().ManagedClusters().Delete(
			context.TODO(), clusterName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	h.AssertClusterDeletedFromHub(clusterName)
	h.AssertClusterDeletedFromSpoke()
}

// AssertHostedClusterDeleted deletes a hosted-mode cluster and waits for cleanup.
func (h *Hub) AssertHostedClusterDeleted(clusterName, managementCluster string) {
	ginkgo.By(fmt.Sprintf("Delete the hosted mode managed cluster %s", clusterName), func() {
		err := h.ClusterClient.ClusterV1().ManagedClusters().Delete(
			context.TODO(), clusterName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	h.AssertClusterDeletedFromHub(clusterName)
	h.AssertHostedClusterDeletedFromSpoke(clusterName, managementCluster)
}

// AssertClusterDeletedFromHub waits for the cluster and its namespace to be deleted.
func (h *Hub) AssertClusterDeletedFromHub(clusterName string) {
	start := time.Now()
	ginkgo.By(fmt.Sprintf("Should delete the managed cluster %s", clusterName), func() {
		gomega.Eventually(func() error {
			_, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("managed cluster %s still exists", clusterName)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	Logf("spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	ginkgo.By(fmt.Sprintf("Should delete the managed cluster namespace %s", clusterName), func() {
		gomega.Eventually(func() error {
			_, err := h.KubeClient.CoreV1().Namespaces().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("managed cluster namespace %s still exists", clusterName)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

// AssertClusterDeletedFromSpoke waits for the agent namespace and klusterlet CRD to be deleted.
func (h *Hub) AssertClusterDeletedFromSpoke() {
	start := time.Now()
	ginkgo.By("Should delete the open-cluster-management-agent namespace", func() {
		gomega.Eventually(func() error {
			_, err := h.KubeClient.CoreV1().Namespaces().Get(
				context.TODO(), agentNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("namespace %s still exists", agentNamespace)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	Logf("delete the open-cluster-management-agent namespace spending time: %.2f seconds",
		time.Since(start).Seconds())

	start = time.Now()
	ginkgo.By("Should delete the klusterlet crd", func() {
		gomega.Eventually(func() error {
			klusterletCRDName := "klusterlets.operator.open-cluster-management.io"
			_, err := h.CRDClient.ApiextensionsV1().CustomResourceDefinitions().Get(
				context.TODO(), klusterletCRDName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("crd %s still exists", klusterletCRDName)
		}, 120*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
	Logf("delete klusterlet crd spending time: %.2f seconds", time.Since(start).Seconds())
}

// AssertHostedClusterDeletedFromSpoke waits for hosted-mode agent namespace and manifest work deletion.
func (h *Hub) AssertHostedClusterDeletedFromSpoke(cluster, managementCluster string) {
	start := time.Now()
	namespace := fmt.Sprintf("klusterlet-%s", cluster)
	ginkgo.By(fmt.Sprintf("Should delete the %s namespace", namespace), func() {
		gomega.Eventually(func() error {
			_, err := h.KubeClient.CoreV1().Namespaces().Get(
				context.TODO(), namespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("namespace %s still exists", namespace)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	Logf("spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	klusterletManifestWorkName := fmt.Sprintf("%s-klusterlet", cluster)
	ginkgo.By(fmt.Sprintf("Should delete the klusterlet manifest work %s", klusterletManifestWorkName), func() {
		gomega.Eventually(func() error {
			_, err := h.WorkClient.WorkV1().ManifestWorks(managementCluster).Get(
				context.TODO(), klusterletManifestWorkName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("klusterlet manifest work %s still exists", klusterletManifestWorkName)
		}, 1*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

// AssertPullSecretDeleted deletes a pull secret and verifies.
func (h *Hub) AssertPullSecretDeleted(namespace, name string) {
	ginkgo.By(fmt.Sprintf("Delete the pull secret %s/%s", name, namespace), func() {
		err := h.KubeClient.CoreV1().Secrets(namespace).Delete(
			context.TODO(), name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})
}

// ---------------------------------------------------------------------------
// ManifestWork assertions
// ---------------------------------------------------------------------------

// AssertManifestWorks waits for klusterlet ManifestWorks to be created.
func (h *Hub) AssertManifestWorks(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s manifest works should be created", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
			klusterletCRDsName := fmt.Sprintf("%s-klusterlet-crds", clusterName)
			klusterletName := fmt.Sprintf("%s-klusterlet", clusterName)
			manifestWorks := h.WorkClient.WorkV1().ManifestWorks(clusterName)

			if _, err := manifestWorks.Get(context.TODO(), klusterletCRDsName, metav1.GetOptions{}); err != nil {
				return err
			}
			if _, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{}); err != nil {
				return err
			}
			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
		Logf("assert managed cluster manifestworks spending time: %.2f seconds", time.Since(start).Seconds())
	})

	h.AssertClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup")
}

// AssertManifestWorksAvailable waits for ManifestWorks to be Available.
func (h *Hub) AssertManifestWorksAvailable(clusterName string) {
	h.AssertClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup")

	klusterletCRDsName := fmt.Sprintf("%s-klusterlet-crds", clusterName)
	klusterletName := fmt.Sprintf("%s-klusterlet", clusterName)

	h.AssertManifestWorkFinalizer(clusterName, klusterletCRDsName, "cluster.open-cluster-management.io/manifest-work-cleanup")
	h.AssertManifestWorkFinalizer(clusterName, klusterletName, "cluster.open-cluster-management.io/manifest-work-cleanup")

	ginkgo.By(fmt.Sprintf("Managed cluster %s manifest works should be available", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
			manifestWorks := h.WorkClient.WorkV1().ManifestWorks(clusterName)

			klusterletCRDs, err := manifestWorks.Get(context.TODO(), klusterletCRDsName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !meta.IsStatusConditionTrue(klusterletCRDs.Status.Conditions, workv1.WorkAvailable) {
				return fmt.Errorf("klusterletCRDs is not available")
			}

			klusterlet, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !meta.IsStatusConditionTrue(klusterlet.Status.Conditions, workv1.WorkAvailable) {
				return fmt.Errorf("klusterlet is not available")
			}

			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
		Logf("assert managed cluster manifestworks spending time: %.2f seconds", time.Since(start).Seconds())
	})

	Logf("wait for applied manifest works ready to avoid delete prematurely (10s)")
	time.Sleep(10 * time.Second)
}

// AssertHostedManifestWorks waits for hosted klusterlet ManifestWorks to be created.
func (h *Hub) AssertHostedManifestWorks(managementClusterName, managedClusterName string) {
	ginkgo.By(fmt.Sprintf("Hosted cluster %s manifest works should be created", managedClusterName), func() {
		gomega.Eventually(func() error {
			klusterletName := fmt.Sprintf("%s-hosted-klusterlet", managedClusterName)
			manifestWorks := h.WorkClient.WorkV1().ManifestWorks(managementClusterName)
			work, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			clusterLabel := work.Labels["import.open-cluster-management.io/hosted-cluster"]
			if clusterLabel != managedClusterName {
				return fmt.Errorf("expect cluster label on %s/%s but failed", managementClusterName, klusterletName)
			}
			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertHostedManifestWorksAvailable waits for hosted ManifestWorks to be Available.
func (h *Hub) AssertHostedManifestWorksAvailable(clusterName, hostingClusterName string) {
	h.AssertClusterFinalizer(clusterName,
		"managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup")

	klusterletName := fmt.Sprintf("%s-hosted-klusterlet", clusterName)
	h.AssertManifestWorkFinalizer(hostingClusterName, klusterletName, "cluster.open-cluster-management.io/manifest-work-cleanup")

	ginkgo.By(fmt.Sprintf("Hosted managed cluster %s manifest works should be available", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
			manifestWorks := h.WorkClient.WorkV1().ManifestWorks(hostingClusterName)
			klusterlet, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !meta.IsStatusConditionTrue(klusterlet.Status.Conditions, workv1.WorkAvailable) {
				return fmt.Errorf("klusterlet is not available")
			}
			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
		Logf("assert hosted managed cluster manifestworks spending time: %.2f seconds",
			time.Since(start).Seconds())
	})
}

// AssertManifestWorkFinalizer waits for a ManifestWork to have the expected finalizer.
func (h *Hub) AssertManifestWorkFinalizer(namespace, workName, expected string) {
	ginkgo.By(fmt.Sprintf("Manifestwork %s/%s should have expected finalizer: %s", namespace, workName, expected), func() {
		gomega.Eventually(func() error {
			work, err := h.WorkClient.WorkV1().ManifestWorks(namespace).Get(
				context.TODO(), workName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for _, finalizer := range work.Finalizers {
				if finalizer == expected {
					return nil
				}
			}
			return fmt.Errorf("manifestwork %s/%s does not have finalizer %s", namespace, workName, expected)
		}, 3*time.Minute, 10*time.Second).Should(gomega.Succeed())
	})
}

// ---------------------------------------------------------------------------
// KlusterletConfig / Klusterlet assertions
// ---------------------------------------------------------------------------

// AssertPriorityClass verifies the priority class is set correctly in the import secret.
func (h *Hub) AssertPriorityClass(clusterName string) {
	start := time.Now()
	defer func() {
		Logf("assert managed cluster priority class spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should set the priorityclass", func() {
		gomega.Eventually(func() error {
			cluster, err := h.ClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if len(cluster.Status.Version.Kubernetes) == 0 {
				return fmt.Errorf("kube version is unknown")
			}

			supported, err := helpers.SupportPriorityClass(cluster)
			if err != nil {
				return err
			}
			var priorityClassName string
			if supported {
				priorityClassName = constants.DefaultKlusterletPriorityClassName
			}

			name := fmt.Sprintf("%s-import", clusterName)
			secret, err := h.KubeClient.CoreV1().Secrets(clusterName).Get(
				context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			var klusterlet *operatorv1.Klusterlet
			var operator *appsv1.Deployment
			for _, yaml := range helpers.SplitYamls(secret.Data[constants.ImportSecretImportYamlKey]) {
				obj := helpers.MustCreateObject(yaml)
				switch required := obj.(type) {
				case *operatorv1.Klusterlet:
					klusterlet = required
				case *appsv1.Deployment:
					operator = required
				}
			}

			if klusterlet == nil {
				return fmt.Errorf("Klusterlet is not found in import.yaml")
			}
			if klusterlet.Spec.PriorityClassName != priorityClassName {
				return fmt.Errorf("expect Klusterlet PriorityClassName %q, got %q",
					priorityClassName, klusterlet.Spec.PriorityClassName)
			}
			if operator == nil {
				return fmt.Errorf("operator is not found in import.yaml")
			}
			if operator.Spec.Template.Spec.PriorityClassName != priorityClassName {
				return fmt.Errorf("expect operator PriorityClassName %q, got %q",
					priorityClassName, operator.Spec.Template.Spec.PriorityClassName)
			}
			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertPriorityClassHosted verifies priority class for hosted mode clusters.
func (h *Hub) AssertPriorityClassHosted(clusterName string) {
	start := time.Now()
	defer func() {
		Logf("assert managed cluster priority class hosted spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should set the priorityclass", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-import", clusterName)
			secret, err := h.KubeClient.CoreV1().Secrets(clusterName).Get(
				context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			var klusterlet *operatorv1.Klusterlet
			for _, yaml := range helpers.SplitYamls(secret.Data[constants.ImportSecretImportYamlKey]) {
				obj := helpers.MustCreateObject(yaml)
				if k, ok := obj.(*operatorv1.Klusterlet); ok {
					klusterlet = k
				}
			}
			if klusterlet == nil {
				return fmt.Errorf("Klusterlet is not found in import.yaml")
			}
			if klusterlet.Spec.PriorityClassName != constants.DefaultKlusterletPriorityClassName {
				return fmt.Errorf("expect PriorityClassName %q, got %q",
					constants.DefaultKlusterletPriorityClassName, klusterlet.Spec.PriorityClassName)
			}
			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertKlusterletNodePlacement verifies node placement configuration.
func (h *Hub) AssertKlusterletNodePlacement(nodeSelector map[string]string, tolerations []corev1.Toleration) {
	ginkgo.By("Klusterlet should have expected nodePlacement", func() {
		gomega.Eventually(func() error {
			klusterlet, err := h.OperatorClient.OperatorV1().Klusterlets().Get(
				context.TODO(), "klusterlet", metav1.GetOptions{})
			if err != nil && errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			deploy, err := h.KubeClient.AppsV1().Deployments(agentNamespace).Get(
				context.TODO(), "klusterlet", metav1.GetOptions{})
			if err != nil && errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			if !equality.Semantic.DeepEqual(klusterlet.Spec.NodePlacement.NodeSelector, nodeSelector) {
				return fmt.Errorf("klusterlet diff: %s", cmp.Diff(klusterlet.Spec.NodePlacement.NodeSelector, nodeSelector))
			}
			if !equality.Semantic.DeepEqual(deploy.Spec.Template.Spec.NodeSelector, nodeSelector) {
				return fmt.Errorf("deployment diff: %s", cmp.Diff(deploy.Spec.Template.Spec.NodeSelector, nodeSelector))
			}
			if !equality.Semantic.DeepEqual(klusterlet.Spec.NodePlacement.Tolerations, tolerations) {
				return fmt.Errorf("klusterlet diff: %s", cmp.Diff(klusterlet.Spec.NodePlacement.Tolerations, tolerations))
			}
			if !equality.Semantic.DeepEqual(deploy.Spec.Template.Spec.Tolerations, tolerations) {
				return fmt.Errorf("deployment diff: %s", cmp.Diff(deploy.Spec.Template.Spec.Tolerations, tolerations))
			}
			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertKlusterletNamespace verifies the klusterlet is deployed in the expected namespace.
func (h *Hub) AssertKlusterletNamespace(clusterName, name, namespace string) {
	ginkgo.By(fmt.Sprintf("Klusterlet %s should be deployed in the namespace %s", name, namespace), func() {
		gomega.Eventually(func() error {
			klusterlet, err := h.OperatorClient.OperatorV1().Klusterlets().Get(
				context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if klusterlet.Spec.Namespace != namespace {
				return fmt.Errorf("klusterlet namespace: want %s, got %s", namespace, klusterlet.Spec.Namespace)
			}
			if klusterlet.Name != name {
				return fmt.Errorf("klusterlet name: want %s, got %s", name, klusterlet.Name)
			}
			_, err = h.KubeClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
			return err
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertAppliedManifestWorkEvictionGracePeriod verifies the eviction grace period arg.
func (h *Hub) AssertAppliedManifestWorkEvictionGracePeriod(evictionGracePeriod *metav1.Duration) {
	ginkgo.By("Klusterlet should have expected AppliedManifestWorkEvictionGracePeriod", func() {
		gomega.Eventually(func() error {
			deploy, err := h.KubeClient.AppsV1().Deployments(agentNamespace).Get(
				context.TODO(), "klusterlet-agent", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if len(deploy.Spec.Template.Spec.Containers) != 1 {
				return fmt.Errorf("unexpected number of containers: %v", len(deploy.Spec.Template.Spec.Containers))
			}

			found := false
			prefix := "--appliedmanifestwork-eviction-grace-period="
			argValue := ""
			for _, arg := range deploy.Spec.Template.Spec.Containers[0].Args {
				if strings.HasPrefix(arg, prefix) {
					found = true
					argValue = strings.TrimPrefix(arg, prefix)
					break
				}
			}

			switch {
			case evictionGracePeriod == nil && !found:
				return nil
			case evictionGracePeriod == nil && found:
				return fmt.Errorf("expected nil evictionGracePeriod but got %v", argValue)
			case evictionGracePeriod != nil && found:
				if evictionGracePeriod.Duration.String() == argValue {
					return nil
				}
				return fmt.Errorf("expected evictionGracePeriod %q but got %v",
					evictionGracePeriod.Duration.String(), argValue)
			case evictionGracePeriod != nil && !found:
				return fmt.Errorf("expected evictionGracePeriod %q but found no argument",
					evictionGracePeriod.Duration.String())
			default:
				return fmt.Errorf("should not reach this branch")
			}
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertFeatureGate verifies the klusterlet has the expected feature gates.
func (h *Hub) AssertFeatureGate(name string, registrationFeatureGates, workFeatureGates []operatorv1.FeatureGate) {
	ginkgo.By(fmt.Sprintf("Klusterlet %s should have desired feature gate", name), func() {
		gomega.Eventually(func() error {
			klusterlet, err := h.OperatorClient.OperatorV1().Klusterlets().Get(
				context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if len(registrationFeatureGates) > 0 {
				if klusterlet.Spec.RegistrationConfiguration == nil {
					return fmt.Errorf("klusterlet %v has no registration configuration", klusterlet.Name)
				}
				if !equality.Semantic.DeepEqual(registrationFeatureGates, klusterlet.Spec.RegistrationConfiguration.FeatureGates) {
					return fmt.Errorf("feature gate: get %v, desired %v",
						klusterlet.Spec.RegistrationConfiguration.FeatureGates, registrationFeatureGates)
				}
			} else {
				if klusterlet.Spec.RegistrationConfiguration != nil && len(klusterlet.Spec.RegistrationConfiguration.FeatureGates) > 0 {
					return fmt.Errorf("klusterlet %v has unexpected registration feature gates", klusterlet.Name)
				}
			}

			if len(workFeatureGates) > 0 {
				if klusterlet.Spec.WorkConfiguration == nil {
					return fmt.Errorf("klusterlet %v has no work configuration", klusterlet.Name)
				}
				if !equality.Semantic.DeepEqual(workFeatureGates, klusterlet.Spec.WorkConfiguration.FeatureGates) {
					return fmt.Errorf("feature gate: get %v, desired %v",
						klusterlet.Spec.WorkConfiguration.FeatureGates, workFeatureGates)
				}
			} else {
				if klusterlet.Spec.WorkConfiguration != nil && len(klusterlet.Spec.WorkConfiguration.FeatureGates) > 0 {
					return fmt.Errorf("klusterlet %v has unexpected work feature gates", klusterlet.Name)
				}
			}
			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

// ---------------------------------------------------------------------------
// Bootstrap kubeconfig assertions
// ---------------------------------------------------------------------------

// AssertBootstrapKubeconfig verifies the bootstrap kubeconfig secret.
func (h *Hub) AssertBootstrapKubeconfig(serverURL, proxyURL, ca string, caData []byte, verifyHubKubeconfig bool) {
	start := time.Now()
	defer func() {
		Logf("assert kubeconfig spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should have the expected bootstrap kubeconfig", func() {
		gomega.Eventually(func() error {
			err := h.assertKubeconfig("bootstrap-hub-kubeconfig", serverURL, proxyURL, ca, caData)
			if err != nil {
				return err
			}
			if verifyHubKubeconfig {
				return h.assertKubeconfig("hub-kubeconfig-secret", serverURL, proxyURL, ca, caData)
			}
			return nil
		}, 120*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertBootstrapKubeconfigConsistently verifies the kubeconfig stays consistent.
func (h *Hub) AssertBootstrapKubeconfigConsistently(serverURL, proxyURL, ca string, caData []byte, verifyHubKubeconfig bool, duration time.Duration) {
	start := time.Now()
	defer func() {
		Logf("assert kubeconfig consistently spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should use the expected endpoint", func() {
		gomega.Consistently(func() error {
			err := h.assertKubeconfig("bootstrap-hub-kubeconfig", serverURL, proxyURL, ca, caData)
			if err != nil {
				return err
			}
			if verifyHubKubeconfig {
				return h.assertKubeconfig("hub-kubeconfig-secret", serverURL, proxyURL, ca, caData)
			}
			return nil
		}, duration, 1*time.Second).Should(gomega.Succeed())
	})
}

// AssertBootstrapKubeconfigWithProxy verifies the bootstrap kubeconfig has expected proxy settings.
func (h *Hub) AssertBootstrapKubeconfigWithProxy(proxyURL string, caDataIncluded, caDataExcluded []byte) {
	ginkgo.By("Klusterlet should have bootstrap kubeconfig with expected proxy settings", func() {
		gomega.Eventually(func() error {
			secret, err := h.KubeClient.CoreV1().Secrets(agentNamespace).Get(
				context.TODO(), "bootstrap-hub-kubeconfig", metav1.GetOptions{})
			if err != nil {
				return err
			}

			config, err := clientcmd.Load(secret.Data["kubeconfig"])
			if err != nil {
				return err
			}

			ctx, ok := config.Contexts[config.CurrentContext]
			if !ok {
				return fmt.Errorf("current context %s not found", config.CurrentContext)
			}
			cluster, ok := config.Clusters[ctx.Cluster]
			if !ok {
				return fmt.Errorf("cluster %s not found", ctx.Cluster)
			}
			if cluster.ProxyURL != proxyURL {
				return fmt.Errorf("expected proxy url %q but got: %s", proxyURL, cluster.ProxyURL)
			}

			if len(cluster.CertificateAuthorityData) == 0 {
				if len(caDataIncluded) == 0 {
					return nil
				}
				return fmt.Errorf("kubeconfig has no ca bundle specified")
			}

			caCerts, err := certutil.ParseCertsPEM(cluster.CertificateAuthorityData)
			if err != nil {
				return err
			}

			if len(caDataIncluded) > 0 {
				caCertsIncluded, err := certutil.ParseCertsPEM(caDataIncluded)
				if err != nil {
					return err
				}
				for _, cert := range caCertsIncluded {
					if !hasCertificate(caCerts, cert) {
						return fmt.Errorf("kubeconfig ca bundle does not include cert: %s", cert.Subject.CommonName)
					}
				}
			}

			if len(caDataExcluded) > 0 {
				caCertsExcluded, err := certutil.ParseCertsPEM(caDataExcluded)
				if err != nil {
					return err
				}
				for _, cert := range caCertsExcluded {
					if hasCertificate(caCerts, cert) {
						return fmt.Errorf("kubeconfig ca bundle should not include cert: %s", cert.Subject.CommonName)
					}
				}
			}

			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func (h *Hub) assertKubeconfig(secretName, serverURL, proxyURL, ca string, caData []byte) error {
	namespace := agentNamespace
	secret, err := h.KubeClient.CoreV1().Secrets(namespace).Get(
		context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	kubeconfigData, ok := secret.Data["kubeconfig"]
	if !ok {
		return fmt.Errorf("secret %s/%s has no kubeconfig", namespace, secretName)
	}

	config, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return err
	}

	ctx, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return fmt.Errorf("kubeconfig has no context %q", config.CurrentContext)
	}
	cluster, ok := config.Clusters[ctx.Cluster]
	if !ok {
		return fmt.Errorf("kubeconfig has no cluster %q", ctx.Cluster)
	}

	if cluster.Server != serverURL {
		return fmt.Errorf("kubeconfig server: want %q, got %s", serverURL, cluster.Server)
	}
	if cluster.CertificateAuthority != ca {
		return fmt.Errorf("kubeconfig ca: want %q, got %s", ca, cluster.CertificateAuthority)
	}
	if cluster.ProxyURL != proxyURL {
		return fmt.Errorf("kubeconfig proxy: want %q, got %s", proxyURL, cluster.ProxyURL)
	}
	if !reflect.DeepEqual(cluster.CertificateAuthorityData, caData) {
		return fmt.Errorf("kubeconfig ca data mismatch")
	}
	return nil
}

func hasCertificate(certs []*x509.Certificate, cert *x509.Certificate) bool {
	if cert == nil {
		return true
	}
	for i := range certs {
		if reflect.DeepEqual(certs[i].Raw, cert.Raw) {
			return true
		}
	}
	return false
}
