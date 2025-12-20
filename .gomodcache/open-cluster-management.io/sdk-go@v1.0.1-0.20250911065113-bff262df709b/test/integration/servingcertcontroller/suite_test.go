package servingcertcontroller

import (
	"context"
	"fmt"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/sdk-go/pkg/servingcert"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
	"time"
)

var (
	testEnv    *envtest.Environment
	kubeClient kubernetes.Interface
	ctx        context.Context
	cancel     context.CancelFunc
)

const (
	eventuallyTimeout  = 30 // seconds
	eventuallyInterval = 1  // seconds
	testNamespace      = "test-serving-cert-controller"
	testTargetService1 = "test-target-service-1"
	testTargetService2 = "test-target-service-2"
)

var servingCertController = &servingcert.ServingCertController{}

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "servingCertController integration suite")
}

var _ = ginkgo.BeforeSuite(func(done ginkgo.Done) {
	ctx, cancel = context.WithCancel(context.TODO())

	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	cfg.QPS = 100
	cfg.Burst = 200

	kubeClient, err = kubernetes.NewForConfig(cfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = kubeClient.CoreV1().Namespaces().Create(ctx,
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	servingCertController = servingcert.NewServingCertController(testNamespace, kubeClient).
		WithTargetServingCerts([]servingcert.TargetServingCertOptions{
			{
				Name:      testTargetService1,
				HostNames: []string{fmt.Sprintf("%s.%s.svc", testTargetService1, testNamespace)},
				LoadDir:   "./",
			},
			{
				Name:      testTargetService2,
				HostNames: []string{fmt.Sprintf("%s.%s.svc", testTargetService2, testNamespace)},
			},
		}).WithResyncInterval(time.Second * 1)

	servingCertController.Start(ctx)
	close(done)
}, 300)

var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("tearing down the test environment")

	cancel()
	err := testEnv.Stop()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
})
