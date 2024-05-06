package helpers

import (
	"fmt"

	listerklusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/listers/klusterletconfig/v1alpha1"
	klusterletconfighelper "github.com/stolostron/cluster-lifecycle-api/helpers/klusterletconfig"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func GetMergedKlusterletConfigWithGlobal(
	klusterletconfigName string,
	kcLister listerklusterletconfigv1alpha1.KlusterletConfigLister,
) (*klusterletconfigv1alpha1.KlusterletConfig, error) {
	var err error
	var kc *klusterletconfigv1alpha1.KlusterletConfig
	if klusterletconfigName != "" {
		kc, err = kcLister.Get(klusterletconfigName)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get klusterletconfig %s: %v", klusterletconfigName, err)
		}
	}

	globalKlusterletConfig, err := kcLister.Get(constants.GlobalKlusterletConfigName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get global klusterletconfig: %v", err)
	}

	// The object get from a lister should be be modified directly.
	return klusterletconfighelper.MergeKlusterletConfigs(globalKlusterletConfig.DeepCopy(), kc.DeepCopy())
}
