package basecontroller

import (
	"context"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"open-cluster-management.io/sdk-go/pkg/basecontroller/factory"
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
	labelKey           = "test"
)

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "baseController integration suite")
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
	kubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(kubeClient, 1*time.Second,
		informers.WithTweakListOptions(func(listOptions *metav1.ListOptions) {
			selector := &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      labelKey,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			}
			listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
		}))

	testController := newTestController(kubeClient, kubeInformerFactory.Core().V1().ConfigMaps())

	go testController.Run(ctx, 1)
	go kubeInformerFactory.Start(ctx.Done())

	close(done)
}, 300)

var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("tearing down the test environment")

	cancel()
	err := testEnv.Stop()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
})

type testController struct {
	kubeClient      kubernetes.Interface
	configMapLister corev1lister.ConfigMapLister
}

func newTestController(
	kubeClient kubernetes.Interface,
	configmapInformers corev1informers.ConfigMapInformer) factory.Controller {
	c := &testController{
		kubeClient:      kubeClient,
		configMapLister: configmapInformers.Lister(),
	}

	return factory.New().WithInformersQueueKeysFunc(func(obj runtime.Object) []string {
		key, _ := cache.MetaNamespaceKeyFunc(obj)
		return []string{key}
	}, configmapInformers.Informer()).WithSync(c.sync).ToController("test-controller")
}

func (c *testController) sync(ctx context.Context, syncCtx factory.SyncContext, key string) error {
	klog.Infof("Reconciling test controller sync %q", key)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil
	}

	cm, err := c.configMapLister.ConfigMaps(namespace).Get(name)
	switch {
	case errors.IsNotFound(err):
		return nil
	case err != nil:
		return err
	}

	labels := cm.GetLabels()
	if len(labels) == 0 {
		return nil
	}

	cm.Data = labels

	_, err = c.kubeClient.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{})

	return err
}
