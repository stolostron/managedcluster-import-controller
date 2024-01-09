// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"os/user"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/openshift/library-go/pkg/operator/events"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	certutil "k8s.io/client-go/util/cert"
	addonclient "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	klusterletconfigclient "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/clientset/versioned"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var (
	clusterCfg             *rest.Config
	hubKubeClient          kubernetes.Interface
	hubDynamicClient       dynamic.Interface
	crdClient              apiextensionsclient.Interface
	hubClusterClient       clusterclient.Interface
	hubWorkClient          workclient.Interface
	hubOperatorClient      operatorclient.Interface
	addonClient            addonclient.Interface
	klusterletconfigClient klusterletconfigclient.Interface
	hubRuntimeClient       crclient.Client
	hubRecorder            events.Recorder
	hubMapper              meta.RESTMapper
)

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "End-to-end Test Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	var err error

	kubeconfig, err := getKubeConfigFile()
	gomega.Expect(err).Should(gomega.BeNil())

	clusterCfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	gomega.Expect(err).Should(gomega.BeNil())

	hubKubeClient, err = kubernetes.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	hubDynamicClient, err = dynamic.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	crdClient, err = apiextensionsclient.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	hubClusterClient, err = clusterclient.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	hubWorkClient, err = workclient.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	hubOperatorClient, err = operatorclient.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	addonClient, err = addonclient.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	klusterletconfigClient, err = klusterletconfigclient.NewForConfig(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())

	hubRuntimeClient, err = crclient.New(clusterCfg, crclient.Options{})
	gomega.Expect(err).Should(gomega.BeNil())

	hubRecorder = helpers.NewEventRecorder(hubKubeClient, "e2e-test")
	httpclient, err := rest.HTTPClientFor(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())
	hubMapper, err = apiutil.NewDiscoveryRESTMapper(clusterCfg, httpclient)
	gomega.Expect(err).Should(gomega.BeNil())
})

// asserters
func assertManagedClusterImportSecretCreated(clusterName, createdVia string, mode ...operatorv1.InstallMode) {
	assertManagedClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/cleanup")
	assertManagedClusterCreatedViaAnnotation(clusterName, createdVia)
	assertManagedClusterNameLabel(clusterName)
	assertManagedClusterNamespaceLabel(clusterName)
	assertManagedClusterRBAC(clusterName)
	if len(mode) != 0 && mode[0] == operatorv1.InstallModeHosted {
		assertHostedManagedClusterImportSecret(clusterName)
	} else {
		assertManagedClusterImportSecret(clusterName)
	}
}

func assertManagedClusterFinalizer(clusterName, expected string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should has expected finalizer: %s", clusterName, expected), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
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

func assertManagedClusterCreatedViaAnnotation(clusterName, expected string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should has expected annotation: %s", clusterName, expected), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			annotation, ok := cluster.Annotations["open-cluster-management/created-via"]
			if !ok {
				return fmt.Errorf("managed cluster %s does not have expected annotation %s", clusterName, expected)
			}

			if annotation != expected {
				return fmt.Errorf("managed cluster %s does not have expected annotation %s, get %s", clusterName, expected, annotation)
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterNameLabel(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should has cluster name label", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			name, ok := cluster.Labels["name"]
			if !ok {
				return fmt.Errorf("managed cluster %s does not have expected label \"name\"", clusterName)
			}

			if name != clusterName {
				return fmt.Errorf("managed cluster %s does not have expected label \"name\", expect %s, get %s", clusterName, clusterName, name)
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterNamespaceLabel(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster namespace %s should has cluster label", clusterName), func() {
		gomega.Eventually(func() error {
			ns, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			name, ok := ns.Labels["cluster.open-cluster-management.io/managedCluster"]
			if !ok {
				return fmt.Errorf("managed cluster namespace %s does not have expected label \"cluster.open-cluster-management.io/managedCluster\"", clusterName)
			}

			if name != clusterName {
				return fmt.Errorf("managed cluster namespace %s does not have expected label \"cluster.open-cluster-management.io/managedCluster\", expect %s, get %s", clusterName, clusterName, name)
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterRBAC(managedClusterName string) {
	ginkgo.By("Should has clusterrole", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			_, err := hubKubeClient.RbacV1().ClusterRoles().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("Should has cluserrolebiding", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			_, err := hubKubeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("Should has bootstrap sa", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-bootstrap-sa", managedClusterName)
			_, err := hubKubeClient.CoreV1().ServiceAccounts(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterImportSecret(managedClusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster import secret spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should create the import secret", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-import", managedClusterName)
			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if err := helpers.ValidateImportSecret(secret); err != nil {
				return fmt.Errorf("invalidated import secret:%v", err)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertHostedManagedClusterImportSecret(managedClusterName string) {
	ginkgo.By("Should create the import secret", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-import", managedClusterName)
			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if err := helpers.ValidateHostedImportSecret(secret); err != nil {
				return fmt.Errorf("invalidated import secret:%v", err)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterDeleted(clusterName string) {
	ginkgo.By(fmt.Sprintf("Delete the managed cluster %s", clusterName), func() {
		err := hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), clusterName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	assertManagedClusterDeletedFromHub(clusterName)
	assertManagedClusterDeletedFromSpoke()
}

func assertPullSecretDeleted(namespace, name string) {
	ginkgo.By(fmt.Sprintf("Delete the pull secret %s/%s", name, namespace), func() {
		err := hubKubeClient.CoreV1().Secrets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})
}

func assertHostedManagedClusterDeleted(clusterName, managementCluster string) {
	ginkgo.By(fmt.Sprintf("Delete the hosted mode managed cluster %s", clusterName), func() {
		err := hubClusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), clusterName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	assertManagedClusterDeletedFromHub(clusterName)
	assertHostedManagedClusterDeletedFromSpoke(clusterName, managementCluster)
}

func assertManagedClusterDeletedFromHub(clusterName string) {
	start := time.Now()
	ginkgo.By(fmt.Sprintf("Should delete the managed cluster %s", clusterName), func() {
		gomega.Eventually(func() error {
			_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			return fmt.Errorf("managed cluster %s still exists", clusterName)
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	ginkgo.By(fmt.Sprintf("Should delete the managed cluster namespace %s", clusterName), func() {
		gomega.Eventually(func() error {
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("managed cluster namespace %s still exists", clusterName)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertManagedClusterDeletedFromSpoke() {
	start := time.Now()
	ginkgo.By("Should delete the open-cluster-management-agent namespace", func() {
		gomega.Eventually(func() error {
			klusterletNamespace := "open-cluster-management-agent"
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("namespace %s still exists", klusterletNamespace)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("delete the open-cluster-management-agent namespace spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	ginkgo.By("Should delete the klusterlet crd", func() {
		gomega.Eventually(func() error {
			klusterletCRDName := "klusterlets.operator.open-cluster-management.io"
			_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("crd %s still exists", klusterletCRDName)
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("delete klusterlet crd spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertHostedManagedClusterDeletedFromSpoke(cluster, managementCluster string) {
	start := time.Now()
	namespace := fmt.Sprintf("klusterlet-%s", cluster)
	ginkgo.By(fmt.Sprintf("Should delete the %s namespace", namespace), func() {
		gomega.Eventually(func() error {
			klusterletNamespace := namespace
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("namespace %s still exists", klusterletNamespace)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	klusterletManifestWorkName := fmt.Sprintf("%s-klusterlet", cluster)
	ginkgo.By(fmt.Sprintf("Should delete the klusterlet manifest work %s", klusterletManifestWorkName), func() {
		gomega.Eventually(func() error {
			_, err := hubWorkClient.WorkV1().ManifestWorks(managementCluster).Get(context.TODO(), klusterletManifestWorkName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("klusterlet manifest work %s still exists", klusterletManifestWorkName)
		}, 1*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertManagedClusterImportSecretApplied(clusterName string, mode ...operatorv1.InstallMode) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster %s import secret applied spending time: %.2f seconds",
			clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be imported", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("assert managed cluster %s import secret applied get cluster error: %v", clusterName, err)
			}

			util.Logf("assert managed cluster %s import secret applied cluster conditions: %v",
				clusterName, cluster.Status.Conditions)
			if len(mode) != 0 && mode[0] == operatorv1.InstallModeHosted && meta.IsStatusConditionTrue(
				cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded) {
				return nil
			}

			if helpers.ImportingResourcesApplied(meta.FindStatusCondition(
				cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)) ||
				meta.IsStatusConditionTrue(cluster.Status.Conditions,
					constants.ConditionManagedClusterImportSucceeded) {
				return nil
			}

			return fmt.Errorf("assert managed cluster %s import secret applied failed", clusterName)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterImportSecretNotApplied(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should not be imported", clusterName), func() {
		gomega.Consistently(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(
				context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("assert managed cluster %s import secret not applied get cluster error: %v", clusterName, err)
			}

			util.Logf("assert managed cluster %s import secret not applied cluster conditions: %v",
				clusterName, cluster.Status.Conditions)

			condition := meta.FindStatusCondition(
				cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
			if condition == nil {
				return nil
			}

			if condition.Reason == constants.ConditionReasonManagedClusterWaitForImporting {
				return nil
			}

			return fmt.Errorf("assert managed cluster %s import secret not applied failed", clusterName)
		}, 3*time.Minute, 5*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterAvailable(clusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster %s available spending time: %.2f seconds", clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be available", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if meta.IsStatusConditionTrue(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
				return nil
			}

			return fmt.Errorf("assert managed cluster %s available failed, cluster conditions: %v", clusterName, cluster.Status.Conditions)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertHostedKlusterletManifestWorks(managementClusterName, managedClusterName string) {
	ginkgo.By(fmt.Sprintf("Hosted cluster %s manifest works should be created", managedClusterName), func() {
		gomega.Eventually(func() error {
			klusterletName := fmt.Sprintf("%s-hosted-klusterlet", managedClusterName)
			manifestWorks := hubWorkClient.WorkV1().ManifestWorks(managementClusterName)
			work, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			clusterLable := work.Labels["import.open-cluster-management.io/hosted-cluster"]
			if clusterLable != managedClusterName {
				return fmt.Errorf("Expect cluster label on %s/%s but failed", managementClusterName, klusterletName)
			}

			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterManifestWorks(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s manifest works should be created", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
			klusterletCRDsName := fmt.Sprintf("%s-klusterlet-crds", clusterName)
			klusterletName := fmt.Sprintf("%s-klusterlet", clusterName)
			manifestWorks := hubWorkClient.WorkV1().ManifestWorks(clusterName)

			if _, err := manifestWorks.Get(context.TODO(), klusterletCRDsName, metav1.GetOptions{}); err != nil {
				return err
			}

			if _, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{}); err != nil {
				return err
			}

			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
		util.Logf("assert managed cluster manifestworks spending time: %.2f seconds", time.Since(start).Seconds())
	})

	assertManagedClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup")
}

func assertManagedClusterManifestWorksAvailable(clusterName string) {
	assertManagedClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup")

	ginkgo.By(fmt.Sprintf("Managed cluster %s manifest works should be available", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
			klusterletCRDsName := fmt.Sprintf("%s-klusterlet-crds", clusterName)
			klusterletName := fmt.Sprintf("%s-klusterlet", clusterName)
			manifestWorks := hubWorkClient.WorkV1().ManifestWorks(clusterName)

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
		util.Logf("assert managed cluster manifestworks spending time: %.2f seconds", time.Since(start).Seconds())
	})

	util.Logf("wait for applied manifest works ready to avoid delete prematurely (10s)")
	time.Sleep(10 * time.Second)
}

func assertHostedManagedClusterManifestWorksAvailable(clusterName, hostingClusterName string) {
	assertManagedClusterFinalizer(clusterName,
		"managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup")

	ginkgo.By(fmt.Sprintf("Hosted managed cluster %s manifest works should be available", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
			klusterletName := fmt.Sprintf("%s-hosted-klusterlet", clusterName)
			manifestWorks := hubWorkClient.WorkV1().ManifestWorks(hostingClusterName)

			klusterlet, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !meta.IsStatusConditionTrue(klusterlet.Status.Conditions, workv1.WorkAvailable) {
				return fmt.Errorf("klusterlet is not available")
			}

			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
		util.Logf("assert hosted managed cluster manifestworks spending time: %.2f seconds",
			time.Since(start).Seconds())
	})
}

func assertAutoImportSecretDeleted(managedClusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert delete the auto-import-secret from managed cluster namespace %s spending time: %.2f seconds",
			managedClusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Should delete the auto-import-secret from managed cluster namespace %s", managedClusterName), func() {
		gomega.Eventually(func() error {
			_, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
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

func assertManagedClusterNamespace(managedClusterName string) {
	ginkgo.By("Should create the managedCluster namespace", func() {
		gomega.Eventually(func() error {
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertKlusterletNodePlacement(nodeSelecor map[string]string, tolerations []corev1.Toleration) {
	ginkgo.By("Klusterlet should have expected nodePlacement", func() {
		gomega.Eventually(func() error {
			klusterlet, err := hubOperatorClient.OperatorV1().Klusterlets().Get(context.TODO(), "klusterlet", metav1.GetOptions{})
			if err != nil && errors.IsNotFound(err) {
				return nil
			}

			if err != nil {
				return err
			}

			deploy, err := hubKubeClient.AppsV1().Deployments("open-cluster-management-agent").Get(context.TODO(), "klusterlet", metav1.GetOptions{})
			if err != nil && errors.IsNotFound(err) {
				return nil
			}

			if err != nil {
				return err
			}

			if !equality.Semantic.DeepEqual(klusterlet.Spec.NodePlacement.NodeSelector, nodeSelecor) {
				return fmt.Errorf("klusterlet diff: %s", cmp.Diff(klusterlet.Spec.NodePlacement.NodeSelector, nodeSelecor))
			}

			if !equality.Semantic.DeepEqual(deploy.Spec.Template.Spec.NodeSelector, nodeSelecor) {
				return fmt.Errorf("deployment diff: %s", cmp.Diff(deploy.Spec.Template.Spec.NodeSelector, nodeSelecor))
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

func assertBootstrapKubeconfigWithProxyConfig(proxyURL string, caDataIncluded, caDataExcluded []byte) {
	ginkgo.By("Klusterlet should have bootstrap kubeconfig with expected proxy settings", func() {
		var bootstrapKubeconfigSecret *corev1.Secret
		gomega.Eventually(func() error {
			var err error
			bootstrapKubeconfigSecret, err = hubKubeClient.CoreV1().Secrets("open-cluster-management-agent").Get(context.TODO(), "bootstrap-hub-kubeconfig", metav1.GetOptions{})
			if err != nil {
				return err
			}

			config, err := clientcmd.Load(bootstrapKubeconfigSecret.Data["kubeconfig"])
			if err != nil {
				return err
			}

			// check proxy url
			cluster, ok := config.Clusters["default-cluster"]
			if !ok {
				return fmt.Errorf("default-cluster not found")
			}
			if cluster.ProxyURL != proxyURL {
				return fmt.Errorf("expected proxy url %q but got: %s", proxyURL, cluster.ProxyURL)
			}

			caCerts, err := certutil.ParseCertsPEM(cluster.CertificateAuthorityData)
			if err != nil {
				return err
			}

			// check included ca data
			if len(caDataIncluded) > 0 {
				caCertsIncluded, err := certutil.ParseCertsPEM(caDataIncluded)
				if err != nil {
					return err
				}

				for _, cert := range caCertsIncluded {
					if !hasCertificate(caCerts, cert) {
						return fmt.Errorf("kubeconfig ca bundle does not include proxy cert: %s", cert.Subject.CommonName)
					}
				}
			}

			// check excluded ca data
			if len(caDataExcluded) > 0 {
				caCertsExcluded, err := certutil.ParseCertsPEM(caDataExcluded)
				if err != nil {
					return err
				}

				for _, cert := range caCertsExcluded {
					if hasCertificate(caCerts, cert) {
						return fmt.Errorf("kubeconfig ca bundle should not include proxy cert: %s", cert.Subject.CommonName)
					}
				}
			}

			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
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

func assertManagedClusterOffline(clusterName string, timeout time.Duration) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be offline", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if meta.IsStatusConditionPresentAndEqual(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
				return nil
			}

			return fmt.Errorf("assert managed cluster %s offline failed, cluster conditions: %v", clusterName, cluster.Status.Conditions)
		}, timeout, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertNamespaceCreated(kubeClient kubernetes.Interface, namespace string) {
	ginkgo.By(fmt.Sprintf("Namespace %s should be created", namespace), func() {
		gomega.Eventually(func() error {
			_, err := kubeClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
			if err != nil {
				return err
			}
			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func getKubeConfigFile() (string, error) {
	kubeConfigFile := os.Getenv("KUBECONFIG")
	if kubeConfigFile == "" {
		user, err := user.Current()
		if err != nil {
			return "", err
		}
		kubeConfigFile = path.Join(user.HomeDir, ".kube", "config")
	}

	return kubeConfigFile, nil
}
