package helpers

import (
	"context"
	"fmt"

	klusterletconfigclient "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/clientset/versioned"
	klusterletconfighelper "github.com/stolostron/cluster-lifecycle-api/helpers/klusterletconfig"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetMergedKlusterletConfigWithGlobal(
	klusterletconfigName string,
	kcClient klusterletconfigclient.Interface,
) (*klusterletconfigv1alpha1.KlusterletConfig, error) {
	var err error
	var kc *klusterletconfigv1alpha1.KlusterletConfig
	if klusterletconfigName != "" {
		kc, err = kcClient.ConfigV1alpha1().KlusterletConfigs().Get(context.TODO(), klusterletconfigName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get klusterletconfig %s: %v", klusterletconfigName, err)
		}
	}

	globalKlusterletConfig, err := kcClient.ConfigV1alpha1().KlusterletConfigs().Get(context.TODO(), constants.GlobalKlusterletConfigName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get global klusterletconfig: %v", err)
	}
	if globalKlusterletConfig != nil {
		fmt.Printf("global klusterletconfig: %v\n", globalKlusterletConfig)
		fmt.Printf("global klusterletconfig spec nodePlacement: %v\n", globalKlusterletConfig.Spec.NodePlacement)
	}

	return klusterletconfighelper.MergeKlusterletConfigs(globalKlusterletConfig, kc)
}
