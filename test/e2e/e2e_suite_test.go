// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	"github.com/open-cluster-management/managedcluster-import-controller/test/e2e/util"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	clusterCfg        *rest.Config
	hubKubeClient     kubernetes.Interface
	hubDynamicClient  dynamic.Interface
	crdClient         apiextensionsclient.Interface
	hubClusterClient  clusterclient.Interface
	hubWorkClient     workclient.Interface
	hubOperatorClient operatorclient.Interface
)

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "End-to-end Test Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	err := func() error {
		var err error

		kubeconfig := os.Getenv("KUBECONFIG")

		clusterCfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return err
		}

		hubKubeClient, err = kubernetes.NewForConfig(clusterCfg)
		if err != nil {
			return err
		}

		hubDynamicClient, err = dynamic.NewForConfig(clusterCfg)
		if err != nil {
			return err
		}

		crdClient, err = apiextensionsclient.NewForConfig(clusterCfg)
		if err != nil {
			return err
		}

		hubClusterClient, err = clusterclient.NewForConfig(clusterCfg)
		if err != nil {
			return err
		}

		hubWorkClient, err = workclient.NewForConfig(clusterCfg)
		if err != nil {
			return err
		}

		hubOperatorClient, err = operatorclient.NewForConfig(clusterCfg)
		if err != nil {
			return err
		}

		return nil
	}()

	gomega.Expect(err).ToNot(gomega.HaveOccurred())
})

// asserters
func assertManagedClusterImportSecretCreated(clusterName, createdVia string) {
	assertManagedClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/cleanup")
	if len(createdVia) > 0 {
		assertManagedClusterCreatedViaAnnotation(clusterName, createdVia)
	}
	assertManagedClusterNameLabel(clusterName)
	assertManagedClusterNamespaceLabel(clusterName)
	assertManagedClusterRBAC(clusterName)
	assertManagedClusterImportSecret(clusterName)
}

func assertManagedClusterFinalizer(clusterName, expected string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should has expected finalizer: %s", clusterName, expected), func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			for _, finalizer := range cluster.Finalizers {
				if finalizer == expected {
					return true, nil
				}
			}

			return false, nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterCreatedViaAnnotation(clusterName, expected string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should has expected annotation: %s", clusterName, expected), func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			annotation, ok := cluster.Annotations["open-cluster-management/created-via"]
			if !ok {
				return false, nil
			}

			if annotation != expected {
				return false, nil
			}

			return true, nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterNameLabel(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should has cluster name label", clusterName), func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			name, ok := cluster.Labels["name"]
			if !ok {
				return false, nil
			}

			if name != clusterName {
				return false, nil
			}

			return true, nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterNamespaceLabel(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster namespace %s should has cluster label", clusterName), func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			ns, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			name, ok := ns.Labels["cluster.open-cluster-management.io/managedCluster"]
			if !ok {
				return false, nil
			}

			if name != clusterName {
				return false, nil
			}

			return true, nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterRBAC(managedClusterName string) {
	ginkgo.By("Should has clusterrole", func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			_, err := hubKubeClient.RbacV1().ClusterRoles().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			return true, nil
		})).ToNot(gomega.HaveOccurred())
	})

	ginkgo.By("Should has cluserrolebiding", func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			_, err := hubKubeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			return true, nil
		})).ToNot(gomega.HaveOccurred())
	})

	ginkgo.By("Should has bootstrap sa", func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			name := fmt.Sprintf("%s-bootstrap-sa", managedClusterName)
			_, err := hubKubeClient.CoreV1().ServiceAccounts(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			return true, nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterImportSecret(managedClusterName string) {
	ginkgo.By("Should create the import secret", func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			name := fmt.Sprintf("%s-import", managedClusterName)
			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}

			if err := util.ValidateImportSecret(secret); err != nil {
				util.Logf("invalidated import secret:%v", err)
				return false, err
			}
			return true, nil
		})).ToNot(gomega.HaveOccurred())
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

func assertManagedClusterDeletedFromHub(clusterName string) {
	start := time.Now()
	ginkgo.By("Should delete the managed cluster", func() {
		gomega.Expect(wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
			_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	ginkgo.By("Should delete the managed cluster namespace", func() {
		gomega.Expect(wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertManagedClusterDeletedFromSpoke() {
	start := time.Now()
	ginkgo.By("Should delete the open-cluster-management-agent namespace", func() {
		gomega.Expect(wait.Poll(1*time.Second, 5*time.Minute, func() (bool, error) {
			klusterletNamespace := "open-cluster-management-agent"
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	ginkgo.By("Should delete the klusterlet crd", func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			klusterletCRDName := "klusterlets.operator.open-cluster-management.io"
			_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())

}

func assertManagedClusterImportSecretApplied(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be imported", clusterName), func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			return meta.IsStatusConditionTrue(cluster.Status.Conditions, "ManagedClusterImportSucceeded"), nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterAvailable(clusterName string) {
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be available", clusterName), func() {
		gomega.Expect(wait.Poll(1*time.Second, 5*time.Minute, func() (bool, error) {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			return meta.IsStatusConditionTrue(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable), nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterManifestWorks(clusterName string) {
	assertManagedClusterFinalizer(clusterName, "managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup")

	ginkgo.By(fmt.Sprintf("Managed cluster %s manifest works should be available", clusterName), func() {
		start := time.Now()
		gomega.Expect(wait.Poll(1*time.Second, 5*time.Minute, func() (bool, error) {
			klusterletCRDsName := fmt.Sprintf("%s-klusterlet-crds", clusterName)
			klusterletName := fmt.Sprintf("%s-klusterlet", clusterName)
			manifestWorks := hubWorkClient.WorkV1().ManifestWorks(clusterName)

			klusterletCRDs, err := manifestWorks.Get(context.TODO(), klusterletCRDsName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if !meta.IsStatusConditionTrue(klusterletCRDs.Status.Conditions, workv1.WorkAvailable) {
				return false, nil
			}

			klusterlet, err := manifestWorks.Get(context.TODO(), klusterletName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if !meta.IsStatusConditionTrue(klusterlet.Status.Conditions, workv1.WorkAvailable) {
				return false, nil
			}

			return true, nil
		})).ToNot(gomega.HaveOccurred())
		util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
	})

	util.Logf("wait for applied manifest works ready to avoid delete prematurely (10s)")
	time.Sleep(10 * time.Second)
}

func assertAutoImportSecretDeleted(managedClusterName string) {
	ginkgo.By(fmt.Sprintf("Should delete the auto-import-secret from managed cluster namespace %s", managedClusterName), func() {
		gomega.Expect(wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
			_, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertManagedClusterNamespace(managedClusterName string) {
	ginkgo.By("Should create the managedCluster namespace", func() {
		gomega.Expect(wait.Poll(1*time.Second, 60*time.Second, func() (bool, error) {
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			if err != nil && errors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			return true, nil
		})).ToNot(gomega.HaveOccurred())
	})
}

func assertOnlyManagedClusterDeleted(managedClusterName string) {
	start := time.Now()
	ginkgo.By("Should delete the managed cluster", func() {
		gomega.Expect(wait.Poll(1*time.Second, 10*time.Minute, func() (bool, error) {
			_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertKlusterletNamespaceDeleted() {
	start := time.Now()
	ginkgo.By("Should delete the open-cluster-management-agent namespace", func() {
		gomega.Expect(wait.Poll(1*time.Second, 10*time.Minute, func() (bool, error) {
			klusterletNamespace := "open-cluster-management-agent"
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertKlusterletDeleted() {
	start := time.Now()
	ginkgo.By("Should delete the klusterlet crd", func() {
		gomega.Expect(wait.Poll(1*time.Second, 10*time.Minute, func() (bool, error) {
			klusterletCRDName := "klusterlets.operator.open-cluster-management.io"
			crd, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), klusterletCRDName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}

			if crd.DeletionTimestamp.IsZero() {
				// klusterlet crd is not in
				return false, nil
			}

			klusterlet, err := hubOperatorClient.OperatorV1().Klusterlets().Get(context.TODO(), "klusterlet", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}

			if klusterlet.DeletionTimestamp.IsZero() {
				return false, nil
			}

			// klusterlet is not deleted yet
			klusterlet = klusterlet.DeepCopy()
			klusterlet.Finalizers = []string{}
			_, err = hubOperatorClient.OperatorV1().Klusterlets().Update(context.TODO(), klusterlet, metav1.UpdateOptions{})
			return false, err
		})).ToNot(gomega.HaveOccurred())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}
