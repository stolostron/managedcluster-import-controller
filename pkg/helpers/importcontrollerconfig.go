package helpers

import (
	"time"

	"github.com/go-logr/logr"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type ImportControllerConfig struct {
	componentNamespace string
	configMapLister    corev1listers.ConfigMapLister
	log                logr.Logger
}

func NewImportControllerConfig(componentNamespace string, configMapLister corev1listers.ConfigMapLister,
	log logr.Logger) *ImportControllerConfig {
	return &ImportControllerConfig{
		componentNamespace: componentNamespace,
		configMapLister:    configMapLister,
		log:                log,
	}
}

func (c *ImportControllerConfig) GetAutoImportStrategy() (string, error) {
	cm, err := c.configMapLister.ConfigMaps(c.componentNamespace).Get(constants.ControllerConfigConfigMapName)
	if errors.IsNotFound(err) {
		return constants.DefaultAutoImportStrategy, nil
	}
	if err != nil {
		return "", err
	}

	strategy := cm.Data[constants.AutoImportStrategyKey]
	switch strategy {
	case apiconstants.AutoImportStrategyImportAndSync, apiconstants.AutoImportStrategyImportOnly:
		return strategy, nil
	case "":
		return constants.DefaultAutoImportStrategy, nil
	default:
		c.log.Info("Invalid config value found and use default instead.",
			"configmap", constants.ControllerConfigConfigMapName,
			constants.AutoImportStrategyKey, strategy,
			"default", constants.DefaultAutoImportStrategy)
		return constants.DefaultAutoImportStrategy, nil
	}
}

func (c *ImportControllerConfig) GenerateImportConfig() (bool, error) {
	cm, err := c.configMapLister.ConfigMaps(c.componentNamespace).Get(constants.ControllerConfigConfigMapName)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// TODO: move the "clusterImportConfig" definition to cluster-lifecycle-api repo
	if cm.Data["clusterImportConfig"] == "true" {
		return true, nil
	}
	return false, nil
}

func FakeNewImportControllerConfig(componentNamespace string,
	autoImportStrategy, clusterImportConfig string) *ImportControllerConfig {
	if len(autoImportStrategy) == 0 {
		autoImportStrategy = constants.DefaultAutoImportStrategy
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "import-controller-config",
			Namespace: componentNamespace,
		},
		Data: map[string]string{
			"autoImportStrategy":  autoImportStrategy,
			"clusterImportConfig": clusterImportConfig,
		},
	}

	kubeClient := kubefake.NewSimpleClientset(configMap)
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	configmapInformer := kubeInformerFactory.Core().V1().ConfigMaps().Informer()
	_ = configmapInformer.GetStore().Add(configMap)
	return NewImportControllerConfig(componentNamespace,
		kubeInformerFactory.Core().V1().ConfigMaps().Lister(), logf.Log.WithName("fake-import-controller-config"))
}
