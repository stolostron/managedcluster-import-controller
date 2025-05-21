// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusternamespacedeletion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	siteconfigv1alpha1 "github.com/stolostron/siteconfig/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.
var (
	cfg              *rest.Config
	testEnv          *envtest.Environment
	k8sClient        kubernetes.Interface
	hubDynamicClient dynamic.Interface
	runtimeClient    client.Client
	setupLog         = ctrl.Log.WithName("test")
)

func TestAPIs(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Controller Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	ginkgo.By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../../../", "test", "e2e", "resources", "hive"),
			filepath.Join("../../../", "test", "e2e", "resources", "assisted-service"),
			filepath.Join("../../../", "test", "e2e", "resources", "hypershift"),
			filepath.Join("../../../", "test", "e2e", "resources", "ocm"),
			filepath.Join("../../../", "test", "e2e", "resources", "capi"),
			filepath.Join("../../../", "test", "e2e", "resources", "siteconfig"),
		},
	}

	var err error
	cfg, err = testEnv.Start()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(cfg).ToNot(gomega.BeNil())

	err = asv1beta1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = hivev1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = hyperv1beta1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = corev1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = clusterv1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = addonv1alpha1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = capiv1beta1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	err = siteconfigv1alpha1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	opts := ctrl.Options{
		Scheme: scheme.Scheme,
	}

	k8sClient, err = kubernetes.NewForConfig(cfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	hubDynamicClient, err = dynamic.NewForConfig(cfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	mgr, err := ctrl.NewManager(cfg, opts)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	runtimeClient = mgr.GetClient()

	clientHolder := &helpers.ClientHolder{
		RuntimeClient:    runtimeClient,
		RuntimeAPIReader: mgr.GetAPIReader(),
		KubeClient:       k8sClient,
	}

	err = Add(context.TODO(), mgr, clientHolder)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	go func() {
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			setupLog.Error(err, "problem running controllers")
			os.Exit(1)
		}
		fmt.Printf("failed to start manager, %v\n", err)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}()
})

var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("tearing down the test environment")
	if err := testEnv.Stop(); err != nil {
		setupLog.Error(err, "problem tearing down the test environment")
	}
})
