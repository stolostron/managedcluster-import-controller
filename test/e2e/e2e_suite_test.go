// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
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
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
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

	scheme = k8sruntime.NewScheme()
)

func init() {
	utilruntime.Must(k8sscheme.AddToScheme(scheme))
	utilruntime.Must(ocinfrav1.AddToScheme(scheme))
}

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

	hubRuntimeClient, err = crclient.New(clusterCfg, crclient.Options{
		Scheme: scheme,
	})
	gomega.Expect(err).Should(gomega.BeNil())

	hubRecorder = helpers.NewEventRecorder(hubKubeClient, "e2e-test")
	httpclient, err := rest.HTTPClientFor(clusterCfg)
	gomega.Expect(err).Should(gomega.BeNil())
	hubMapper, err = apiutil.NewDynamicRESTMapper(clusterCfg, httpclient)
	gomega.Expect(err).Should(gomega.BeNil())

	createGlobalKlusterletConfig()
})

func createGlobalKlusterletConfig() {
	ginkgo.By("Create global KlusterletConfig, set work status sync interval", func() {
		_, err := klusterletconfigClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(),
			&klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GlobalKlusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					WorkStatusSyncInterval: &metav1.Duration{Duration: 5 * time.Second},
				},
			}, metav1.CreateOptions{})
		// expect err is nil or is already exists
		if !errors.IsAlreadyExists(err) {
			gomega.Expect(err).Should(gomega.Succeed())
		}
	})
}

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
	ginkgo.By(fmt.Sprintf("Managed cluster %s should have expected finalizer: %s", clusterName, expected), func() {
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
	ginkgo.By(fmt.Sprintf("Managed cluster %s should have expected annotation: %s", clusterName, expected), func() {
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
	ginkgo.By(fmt.Sprintf("Managed cluster %s should have cluster name label", clusterName), func() {
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
	ginkgo.By(fmt.Sprintf("Managed cluster namespace %s should have cluster label", clusterName), func() {
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
	ginkgo.By("Should have clusterrole", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			_, err := hubKubeClient.RbacV1().ClusterRoles().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("Should have cluserrolebiding", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("system:open-cluster-management:managedcluster:bootstrap:%s", managedClusterName)
			_, err := hubKubeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("Should have bootstrap sa", func() {
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

func assertManagedClusterPriorityClass(managedClusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster priority class spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should set the priorityclass", func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			// wait for kube version is available in the status of the managed cluster
			if len(cluster.Status.Version.Kubernetes) == 0 {
				return fmt.Errorf("kube version is unknown")
			}
			// check the priority class name of klusterlet & operator
			supported, err := helpers.SupportPriorityClass(cluster)
			if err != nil {
				return err
			}
			var priorityClassName string
			if supported {
				priorityClassName = constants.DefaultKlusterletPriorityClassName
			}

			name := fmt.Sprintf("%s-import", managedClusterName)
			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
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
				return fmt.Errorf("expect PriorityClassName of Klusterlet in import.yaml is %q, but got %q", priorityClassName,
					klusterlet.Spec.PriorityClassName)
			}
			if operator == nil {
				return fmt.Errorf("operator is not found in import.yaml")
			}
			if operator.Spec.Template.Spec.PriorityClassName != priorityClassName {
				return fmt.Errorf("expect PriorityClassName of Klusterlet operator in import.yaml is %q, but got %q", priorityClassName,
					operator.Spec.Template.Spec.PriorityClassName)
			}
			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertBootstrapKubeconfig(serverURL, proxyURL, ca string, caData []byte, verifyHubKubeconfig bool) {
	start := time.Now()
	defer func() {
		util.Logf("assert kubeconfig spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should have the expected bootstrap kubeconfig", func() {
		gomega.Eventually(func() error {
			err := assertKubeconfig("bootstrap-hub-kubeconfig", serverURL, proxyURL, ca, caData)
			if err != nil {
				return err
			}

			if verifyHubKubeconfig {
				return assertKubeconfig("hub-kubeconfig-secret", serverURL, proxyURL, ca, caData)
			}
			return nil
		}, 120*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertBootstrapKubeconfigConsistently(serverURL, proxyURL, ca string, caData []byte, verifyHubKubeconfig bool, duration time.Duration) {
	start := time.Now()
	defer func() {
		util.Logf("assert kubeconfig with internal endpoint consistently spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should use the internal endpoint", func() {
		gomega.Consistently(func() error {
			err := assertKubeconfig("bootstrap-hub-kubeconfig", serverURL, proxyURL, ca, caData)
			if err != nil {
				return err
			}

			if verifyHubKubeconfig {
				return assertKubeconfig("hub-kubeconfig-secret", serverURL, proxyURL, ca, caData)
			}
			return nil
		}, duration, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertKubeconfig(secretName, serverURL, proxyURL, ca string, caData []byte) error {
	namespace := "open-cluster-management-agent"
	secret, err := hubKubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
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

	context, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return fmt.Errorf("kubeconfig in secret %s/%s has no context %q", namespace, secretName, config.CurrentContext)
	}

	cluster, ok := config.Clusters[context.Cluster]
	if !ok {
		return fmt.Errorf("kubeconfig in secret %s/%s has no cluster %q", namespace, secretName, context.Cluster)
	}

	if cluster.Server != serverURL {
		return fmt.Errorf("kubeconfig in secret %s/%s expects server %q but got: %s", namespace, secretName, serverURL, cluster.Server)
	}

	if cluster.CertificateAuthority != ca {
		return fmt.Errorf("kubeconfig in secret %s/%s expects ca %q but got: %s", namespace, secretName, ca, cluster.CertificateAuthority)
	}

	if cluster.ProxyURL != proxyURL {
		return fmt.Errorf("kubeconfig in secret %s/%s expects proxy %q but got: %s", namespace, secretName, proxyURL, cluster.ProxyURL)
	}

	if !reflect.DeepEqual(cluster.CertificateAuthorityData, caData) {
		return fmt.Errorf("kubeconfig in secret %s/%s expects ca data %q but got: %s", namespace, secretName, string(caData), string(cluster.CertificateAuthorityData))
	}

	return nil
}

func assertManagedClusterPriorityClassHosted(managedClusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster priority class hosted spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should set the priorityclass", func() {
		gomega.Eventually(func() error {
			name := fmt.Sprintf("%s-import", managedClusterName)
			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			var klusterlet *operatorv1.Klusterlet
			for _, yaml := range helpers.SplitYamls(secret.Data[constants.ImportSecretImportYamlKey]) {
				obj := helpers.MustCreateObject(yaml)
				switch required := obj.(type) {
				case *operatorv1.Klusterlet:
					klusterlet = required
				}
			}
			if klusterlet == nil {
				return fmt.Errorf("Klusterlet is not found in import.yaml")
			}
			if klusterlet.Spec.PriorityClassName != constants.DefaultKlusterletPriorityClassName {
				return fmt.Errorf("expect PriorityClassName of Klusterlet in import.yaml is %q, but got %q", constants.DefaultKlusterletPriorityClassName,
					klusterlet.Spec.PriorityClassName)
			}
			return nil
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
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

			if err := helpers.ValidateImportSecret(secret); err != nil {
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
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())

	start = time.Now()
	ginkgo.By(fmt.Sprintf("Should delete the managed cluster namespace %s", clusterName), func() {
		gomega.Eventually(func() error {
			ns, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			diagnostics := getNamespaceDiagnostics(context.TODO(), clusterName)
			return fmt.Errorf("managed cluster namespace %s still exists (Phase: %s, Finalizers: %v)%s",
				clusterName, ns.Status.Phase, ns.Finalizers, diagnostics)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

// getNamespaceDiagnostics collects diagnostic information for a namespace that cannot be deleted.
// It returns namespace metadata (finalizers, deletionTimestamp) and lists remaining resources.
func getNamespaceDiagnostics(ctx context.Context, namespaceName string) string {
	var diagnostics strings.Builder
	diagnostics.WriteString(fmt.Sprintf("\n=== Namespace Diagnostics for %s ===\n", namespaceName))

	// Get namespace metadata
	ns, err := hubKubeClient.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		diagnostics.WriteString(fmt.Sprintf("Failed to get namespace: %v\n", err))
		return diagnostics.String()
	}

	// Namespace metadata
	diagnostics.WriteString(fmt.Sprintf("Namespace Phase: %s\n", ns.Status.Phase))
	if ns.DeletionTimestamp != nil {
		diagnostics.WriteString(fmt.Sprintf("DeletionTimestamp: %s\n", ns.DeletionTimestamp.String()))
	}
	if len(ns.Finalizers) > 0 {
		diagnostics.WriteString(fmt.Sprintf("Finalizers: %v\n", ns.Finalizers))
	}

	// Check namespace conditions
	if len(ns.Status.Conditions) > 0 {
		diagnostics.WriteString("Namespace Conditions:\n")
		for _, cond := range ns.Status.Conditions {
			diagnostics.WriteString(fmt.Sprintf("  - Type: %s, Status: %s, Reason: %s, Message: %s\n",
				cond.Type, cond.Status, cond.Reason, cond.Message))
		}
	}

	// List remaining resources in the namespace
	diagnostics.WriteString("\n--- Remaining Resources ---\n")

	// Check Pods
	pods, err := hubKubeClient.CoreV1().Pods(namespaceName).List(ctx, metav1.ListOptions{})
	if err == nil && len(pods.Items) > 0 {
		diagnostics.WriteString(fmt.Sprintf("Pods (%d):\n", len(pods.Items)))
		for _, pod := range pods.Items {
			diagnostics.WriteString(fmt.Sprintf("  - %s (Phase: %s, Finalizers: %v)\n",
				pod.Name, pod.Status.Phase, pod.Finalizers))
		}
	}

	// Check Secrets
	secrets, err := hubKubeClient.CoreV1().Secrets(namespaceName).List(ctx, metav1.ListOptions{})
	if err == nil && len(secrets.Items) > 0 {
		diagnostics.WriteString(fmt.Sprintf("Secrets (%d):\n", len(secrets.Items)))
		for _, secret := range secrets.Items {
			diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v)\n", secret.Name, secret.Finalizers))
		}
	}

	// Check ConfigMaps
	configMaps, err := hubKubeClient.CoreV1().ConfigMaps(namespaceName).List(ctx, metav1.ListOptions{})
	if err == nil && len(configMaps.Items) > 0 {
		diagnostics.WriteString(fmt.Sprintf("ConfigMaps (%d):\n", len(configMaps.Items)))
		for _, cm := range configMaps.Items {
			diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v)\n", cm.Name, cm.Finalizers))
		}
	}

	// Check ServiceAccounts
	serviceAccounts, err := hubKubeClient.CoreV1().ServiceAccounts(namespaceName).List(ctx, metav1.ListOptions{})
	if err == nil && len(serviceAccounts.Items) > 0 {
		diagnostics.WriteString(fmt.Sprintf("ServiceAccounts (%d):\n", len(serviceAccounts.Items)))
		for _, sa := range serviceAccounts.Items {
			diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v)\n", sa.Name, sa.Finalizers))
		}
	}

	// Check Deployments
	deployments, err := hubKubeClient.AppsV1().Deployments(namespaceName).List(ctx, metav1.ListOptions{})
	if err == nil && len(deployments.Items) > 0 {
		diagnostics.WriteString(fmt.Sprintf("Deployments (%d):\n", len(deployments.Items)))
		for _, deploy := range deployments.Items {
			diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v)\n", deploy.Name, deploy.Finalizers))
		}
	}

	// Check using discovery client for all remaining resources
	// This helps find CRD instances that may have finalizers
	apiResourceLists, err := hubKubeClient.Discovery().ServerPreferredNamespacedResources()
	if err != nil {
		// Log but don't fail - this is just additional diagnostic info
		diagnostics.WriteString(fmt.Sprintf("Warning: Could not get API resources: %v\n", err))
	} else {
		for _, resourceList := range apiResourceLists {
			gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
			if err != nil {
				continue
			}
			for _, resource := range resourceList.APIResources {
				// Skip subresources
				if strings.Contains(resource.Name, "/") {
					continue
				}
				// Skip resources we already checked
				if resource.Name == "pods" || resource.Name == "secrets" ||
					resource.Name == "configmaps" || resource.Name == "serviceaccounts" ||
					resource.Name == "deployments" {
					continue
				}
				gvr := gv.WithResource(resource.Name)
				list, err := hubDynamicClient.Resource(gvr).Namespace(namespaceName).List(ctx, metav1.ListOptions{})
				if err != nil {
					continue
				}
				if len(list.Items) > 0 {
					diagnostics.WriteString(fmt.Sprintf("%s.%s (%d):\n", resource.Name, gv.Group, len(list.Items)))
					for _, item := range list.Items {
						finalizers := item.GetFinalizers()
						diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v)\n", item.GetName(), finalizers))
					}
				}
			}
		}
	}

	// Check Klusterlet CRs (cluster-scoped)
	diagnostics.WriteString("\n--- Klusterlet CRs ---\n")
	klusterlets, err := hubOperatorClient.OperatorV1().Klusterlets().List(ctx, metav1.ListOptions{})
	if err != nil {
		diagnostics.WriteString(fmt.Sprintf("Failed to list klusterlets: %v\n", err))
	} else if len(klusterlets.Items) == 0 {
		diagnostics.WriteString("No klusterlets found\n")
	} else {
		for _, kl := range klusterlets.Items {
			diagnostics.WriteString(fmt.Sprintf("Klusterlet: %s\n", kl.Name))
			diagnostics.WriteString(fmt.Sprintf("  Namespace: %s\n", kl.Spec.Namespace))
			diagnostics.WriteString(fmt.Sprintf("  DeployOption.Mode: %s\n", kl.Spec.DeployOption.Mode))
			if kl.DeletionTimestamp != nil {
				diagnostics.WriteString(fmt.Sprintf("  DeletionTimestamp: %s\n", kl.DeletionTimestamp.String()))
			}
			if len(kl.Finalizers) > 0 {
				diagnostics.WriteString(fmt.Sprintf("  Finalizers: %v\n", kl.Finalizers))
			}
			// Show klusterlet conditions
			if len(kl.Status.Conditions) > 0 {
				diagnostics.WriteString("  Conditions:\n")
				for _, cond := range kl.Status.Conditions {
					diagnostics.WriteString(fmt.Sprintf("    - %s: %s (Reason: %s)\n",
						cond.Type, cond.Status, cond.Reason))
				}
			}
		}
	}

	// Get klusterlet-operator logs for debugging cleanup issues
	diagnostics.WriteString("\n--- Klusterlet Operator Logs (last 50 lines) ---\n")
	operatorPods, err := hubKubeClient.CoreV1().Pods(namespaceName).List(ctx, metav1.ListOptions{
		LabelSelector: "app=klusterlet",
	})
	if err != nil {
		diagnostics.WriteString(fmt.Sprintf("Failed to list klusterlet operator pods: %v\n", err))
	} else if len(operatorPods.Items) == 0 {
		diagnostics.WriteString("No klusterlet operator pods found in namespace\n")
	} else {
		for _, pod := range operatorPods.Items {
			diagnostics.WriteString(fmt.Sprintf("Pod: %s (Phase: %s)\n", pod.Name, pod.Status.Phase))
			// Get logs from the pod
			tailLines := int64(50)
			logOptions := &corev1.PodLogOptions{
				TailLines: &tailLines,
			}
			req := hubKubeClient.CoreV1().Pods(namespaceName).GetLogs(pod.Name, logOptions)
			logStream, err := req.Stream(ctx)
			if err != nil {
				diagnostics.WriteString(fmt.Sprintf("  Failed to get logs: %v\n", err))
				continue
			}
			defer logStream.Close()

			buf := new(strings.Builder)
			_, err = io.Copy(buf, logStream)
			if err != nil {
				diagnostics.WriteString(fmt.Sprintf("  Failed to read logs: %v\n", err))
				continue
			}

			// Filter logs for cleanup-related messages
			logLines := strings.Split(buf.String(), "\n")
			diagnostics.WriteString("  Logs (filtered for cleanup/connectivity/error):\n")
			relevantLogCount := 0
			for _, line := range logLines {
				lineLower := strings.ToLower(line)
				if strings.Contains(lineLower, "cleanup") ||
					strings.Contains(lineLower, "connectivity") ||
					strings.Contains(lineLower, "appliedmanifestwork") ||
					strings.Contains(lineLower, "error") ||
					strings.Contains(lineLower, "unauthorized") ||
					strings.Contains(lineLower, "forbidden") ||
					strings.Contains(lineLower, "failed") ||
					strings.Contains(lineLower, "finalizer") {
					diagnostics.WriteString(fmt.Sprintf("    %s\n", line))
					relevantLogCount++
				}
			}
			if relevantLogCount == 0 {
				diagnostics.WriteString("    (no relevant logs found, showing last 10 lines)\n")
				startIdx := len(logLines) - 10
				if startIdx < 0 {
					startIdx = 0
				}
				for _, line := range logLines[startIdx:] {
					if line != "" {
						diagnostics.WriteString(fmt.Sprintf("    %s\n", line))
					}
				}
			}
		}
	}

	// Check ManifestWorks in all managed cluster namespaces
	diagnostics.WriteString("\n--- ManifestWorks (all namespaces) ---\n")
	managedClusters, err := hubClusterClient.ClusterV1().ManagedClusters().List(ctx, metav1.ListOptions{})
	if err != nil {
		diagnostics.WriteString(fmt.Sprintf("Failed to list managed clusters: %v\n", err))
	} else {
		for _, mc := range managedClusters.Items {
			works, err := hubWorkClient.WorkV1().ManifestWorks(mc.Name).List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			if len(works.Items) > 0 {
				diagnostics.WriteString(fmt.Sprintf("ManifestWorks in namespace %s (%d):\n", mc.Name, len(works.Items)))
				for _, work := range works.Items {
					var workStatus string
					for _, cond := range work.Status.Conditions {
						if cond.Type == "Applied" {
							workStatus = fmt.Sprintf("Applied=%s", cond.Status)
							break
						}
					}
					if work.DeletionTimestamp != nil {
						diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v, DeletionTimestamp: %s, %s)\n",
							work.Name, work.Finalizers, work.DeletionTimestamp.String(), workStatus))
					} else {
						diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v, %s)\n",
							work.Name, work.Finalizers, workStatus))
					}
				}
			}
		}
	}

	// Also check if there are any managed clusters still existing
	diagnostics.WriteString("\n--- ManagedClusters ---\n")
	if managedClusters != nil && len(managedClusters.Items) > 0 {
		for _, mc := range managedClusters.Items {
			var mcStatus string
			for _, cond := range mc.Status.Conditions {
				if cond.Type == "ManagedClusterConditionAvailable" {
					mcStatus = fmt.Sprintf("Available=%s", cond.Status)
					break
				}
			}
			if mc.DeletionTimestamp != nil {
				diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v, DeletionTimestamp: %s, %s)\n",
					mc.Name, mc.Finalizers, mc.DeletionTimestamp.String(), mcStatus))
			} else {
				diagnostics.WriteString(fmt.Sprintf("  - %s (Finalizers: %v, %s)\n",
					mc.Name, mc.Finalizers, mcStatus))
			}
		}
	} else {
		diagnostics.WriteString("No managed clusters found\n")
	}

	diagnostics.WriteString("=== End Diagnostics ===\n")
	return diagnostics.String()
}

func assertManagedClusterDeletedFromSpoke() {
	start := time.Now()
	ginkgo.By("Should delete the open-cluster-management-agent namespace", func() {
		gomega.Eventually(func() error {
			klusterletNamespace := "open-cluster-management-agent"
			ns, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			diagnostics := getNamespaceDiagnostics(context.TODO(), klusterletNamespace)
			return fmt.Errorf("namespace %s still exists (Phase: %s, Finalizers: %v)%s",
				klusterletNamespace, ns.Status.Phase, ns.Finalizers, diagnostics)
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
		}, 120*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("delete klusterlet crd spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertHostedManagedClusterDeletedFromSpoke(cluster, managementCluster string) {
	start := time.Now()
	namespace := fmt.Sprintf("klusterlet-%s", cluster)
	ginkgo.By(fmt.Sprintf("Should delete the %s namespace", namespace), func() {
		gomega.Eventually(func() error {
			klusterletNamespace := namespace
			ns, err := hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), klusterletNamespace, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			diagnostics := getNamespaceDiagnostics(context.TODO(), klusterletNamespace)
			return fmt.Errorf("namespace %s still exists (Phase: %s, Finalizers: %v)%s",
				klusterletNamespace, ns.Status.Phase, ns.Finalizers, diagnostics)
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
		}, 5*time.Minute, 30*time.Second).Should(gomega.Succeed())
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
		}, 15*time.Second, 1*time.Second).Should(gomega.Succeed())
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

func assertImmediateImportCompleted(clusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert immediate-import annotation of managed cluster %s completed spending time: %.2f seconds", clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("The immediate-import annotation of Managed cluster %s should be completed", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			immediateImportValue := cluster.Annotations[apiconstants.AnnotationImmediateImport]
			if immediateImportValue == apiconstants.AnnotationValueImmediateImportCompleted {
				return nil
			}

			return fmt.Errorf("assert immediate-import annotation of managed cluster %s failed, value: %v", clusterName, immediateImportValue)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterAvailableUnknown(clusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster %s available unknown spending time: %.2f seconds", clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be available unknown", clusterName), func() {
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if meta.IsStatusConditionPresentAndEqual(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
				return nil
			}

			return fmt.Errorf("assert managed cluster %s available unknown failed, cluster conditions: %v", clusterName, cluster.Status.Conditions)
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertManagedClusterAvailableUnknownConsistently(clusterName string, duration time.Duration) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster %s available unknown consistently spending time: %.2f seconds", clusterName, time.Since(start).Seconds())
	}()
	ginkgo.By(fmt.Sprintf("Managed cluster %s should be available unknown for %v", clusterName, duration), func() {
		gomega.Consistently(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if meta.IsStatusConditionPresentAndEqual(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
				return nil
			}

			return fmt.Errorf("assert managed cluster %s available unknown consistently failed, cluster conditions: %v", clusterName, cluster.Status.Conditions)
		}, duration, 2*time.Second).Should(gomega.Succeed())
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

	klusterletCRDsName := fmt.Sprintf("%s-klusterlet-crds", clusterName)
	klusterletName := fmt.Sprintf("%s-klusterlet", clusterName)

	assertManifestworkFinalizer(clusterName, klusterletCRDsName, "cluster.open-cluster-management.io/manifest-work-cleanup")
	assertManifestworkFinalizer(clusterName, klusterletName, "cluster.open-cluster-management.io/manifest-work-cleanup")

	ginkgo.By(fmt.Sprintf("Managed cluster %s manifest works should be available", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
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

	klusterletName := fmt.Sprintf("%s-hosted-klusterlet", clusterName)
	assertManifestworkFinalizer(hostingClusterName, klusterletName, "cluster.open-cluster-management.io/manifest-work-cleanup")

	ginkgo.By(fmt.Sprintf("Hosted managed cluster %s manifest works should be available", clusterName), func() {
		start := time.Now()
		gomega.Eventually(func() error {
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
			context, ok := config.Contexts[config.CurrentContext]
			if !ok {
				return fmt.Errorf("current context %s not found", config.CurrentContext)
			}
			cluster, ok := config.Clusters[context.Cluster]
			if !ok {
				return fmt.Errorf("cluster %s not found", context.Cluster)
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

func AssertKlusterletNamespace(clusterName, name, namespace string) {
	ginkgo.By(fmt.Sprintf("Klusterlet %s should be deployed in the namespace %s", name, namespace), func() {
		gomega.Eventually(func() error {
			var err error

			klusterlet, err := hubOperatorClient.OperatorV1().Klusterlets().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if klusterlet.Spec.Namespace != namespace {
				return fmt.Errorf("klusterlet namespace is not correct, expect %s but got %s", namespace, klusterlet.Spec.Namespace)
			}

			if klusterlet.Name != name {
				return fmt.Errorf("klusterlet name is not correct, expect %s but got %s", name, klusterlet.Name)
			}

			_, err = hubKubeClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
			if err != nil {
				return err
			}

			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertAppliedManifestWorkEvictionGracePeriod(evictionGracePeriod *metav1.Duration) {
	ginkgo.By("Klusterlet should have expected AppliedManifestWorkEvictionGracePeriod", func() {
		gomega.Eventually(func() error {
			deploy, err := hubKubeClient.AppsV1().Deployments("open-cluster-management-agent").Get(context.TODO(), "klusterlet-agent", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if len(deploy.Spec.Template.Spec.Containers) != 1 {
				return fmt.Errorf("Unexpected number of contianers found for klusterlet-agent: %v", len(deploy.Spec.Template.Spec.Containers))
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
				return fmt.Errorf("Expected nil evictionGracePeriod but got %v", argValue)
			case evictionGracePeriod != nil && found:
				if evictionGracePeriod.Duration.String() == argValue {
					return nil
				}
				return fmt.Errorf("Expected evictionGracePeriod %q but got %v", evictionGracePeriod.Duration.String(), argValue)
			case evictionGracePeriod != nil && !found:
				return fmt.Errorf("Expected evictionGracePeriod %q but found no argument", evictionGracePeriod.Duration.String())
			default:
				return fmt.Errorf("Should not step into this branch")
			}
		}, 60*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}

func assertFeatureGate(name string, regsitrationFeatureGates, workFeatureGates []operatorv1.FeatureGate) {
	ginkgo.By(fmt.Sprintf("Klusterlet %s should have desired feature gate", name), func() {
		gomega.Eventually(func() error {
			var err error

			klusterlet, err := hubOperatorClient.OperatorV1().Klusterlets().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if len(regsitrationFeatureGates) > 0 {
				if klusterlet.Spec.RegistrationConfiguration == nil {
					return fmt.Errorf("klusterlet %v has no registration configuration", klusterlet.Name)
				}
				if !equality.Semantic.DeepEqual(regsitrationFeatureGates, klusterlet.Spec.RegistrationConfiguration.FeatureGates) {
					return fmt.Errorf("feature gate is not correct set, get %v, desired %v",
						klusterlet.Spec.RegistrationConfiguration.FeatureGates, regsitrationFeatureGates)
				}
			} else {
				if klusterlet.Spec.RegistrationConfiguration != nil && len(klusterlet.Spec.RegistrationConfiguration.FeatureGates) > 0 {
					return fmt.Errorf("klusterlet %v has no registration configuration", klusterlet.Name)
				}
			}

			if len(workFeatureGates) > 0 {
				if klusterlet.Spec.WorkConfiguration == nil {
					return fmt.Errorf("klusterlet %v has no work configuration", klusterlet.Name)
				}
				if !equality.Semantic.DeepEqual(workFeatureGates, klusterlet.Spec.WorkConfiguration.FeatureGates) {
					return fmt.Errorf("feature gate is not correct set, get %v, desired %v",
						klusterlet.Spec.WorkConfiguration.FeatureGates, workFeatureGates)
				}
			} else {
				if klusterlet.Spec.WorkConfiguration != nil && len(klusterlet.Spec.WorkConfiguration.FeatureGates) > 0 {
					return fmt.Errorf("klusterlet %v has no work configuration", klusterlet.Name)
				}
			}

			return nil
		}, 5*time.Minute, 1*time.Second).Should(gomega.Succeed())
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

func assertManifestworkFinalizer(namespace, workName, expected string) {
	ginkgo.By(fmt.Sprintf("Manifestwork %s/%s should have expected finalizer: %s", namespace, workName, expected), func() {
		gomega.Eventually(func() error {
			work, err := hubWorkClient.WorkV1().ManifestWorks(namespace).Get(context.TODO(), workName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for _, finalizer := range work.Finalizers {
				if finalizer == expected {
					return nil
				}
			}
			return fmt.Errorf("Manifestwork %s/%s does not have expected finalizer %s", namespace, workName, expected)
		}, 3*time.Minute, 10*time.Second).Should(gomega.Succeed())
	})
}

func assertAgentLeaderElection() {
	start := time.Now()
	ginkgo.By("Check if klusterlet agent is leader", func() {
		gomega.Eventually(func() error {
			namespace := "open-cluster-management-agent"
			agentSelector := "app=klusterlet-agent"
			leaseName := "klusterlet-agent-lock"
			// agent pod
			pods, err := hubKubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: agentSelector,
			})
			if err != nil {
				return fmt.Errorf("could not get agent pod: %v", err)
			}
			if len(pods.Items) != 1 {
				return fmt.Errorf("should be only one agent pod but get %d", len(pods.Items))
			}

			// agent lease
			lease, err := hubKubeClient.CoordinationV1().Leases(namespace).Get(context.TODO(), leaseName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("could not get Lease: %v", err)
			}

			// Check if the HolderIdentity field is present and if it has the prefix of the podName
			if lease.Spec.HolderIdentity != nil && strings.HasPrefix(*lease.Spec.HolderIdentity, pods.Items[0].Name) {
				return nil
			}

			return fmt.Errorf("klusterlet agent leader is still %s not %s", *lease.Spec.HolderIdentity, pods.Items[0].Name)
		}, 180*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
	util.Logf("spending time: %.2f seconds", time.Since(start).Seconds())
}

func assertClusterImportConfigSecret(managedClusterName string) {
	start := time.Now()
	defer func() {
		util.Logf("assert managed cluster import secret spending time: %.2f seconds", time.Since(start).Seconds())
	}()
	ginkgo.By("Should create the cluster import config secret", func() {
		gomega.Eventually(func() error {
			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), constants.ClusterImportConfigSecretName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if err := helpers.ValidateClusterImportConfigSecret(secret); err != nil {
				return fmt.Errorf("invalidated cluster import config secret:%v", err)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}
