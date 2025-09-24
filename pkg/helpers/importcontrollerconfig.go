package helpers

import (
	"github.com/go-logr/logr"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"k8s.io/apimachinery/pkg/api/errors"
	corev1listers "k8s.io/client-go/listers/core/v1"
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

// GenerateImportConfig to check whether to generate import config secret.
func (c *ImportControllerConfig) GenerateImportConfig() (bool, error) {
	cm, err := c.configMapLister.ConfigMaps(c.componentNamespace).Get(constants.ControllerConfigConfigMapName)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if cm.Data[constants.ClusterImportConfig] == "true" {
		return true, nil
	}
	return false, nil
}
