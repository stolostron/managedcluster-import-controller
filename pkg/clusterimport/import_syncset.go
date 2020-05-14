// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterimport ...
package clusterimport

import (
	"context"
	"encoding/json"
	"reflect"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

const syncsetNamePostfix = "-klusterlet"

func syncSetNsN(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig, clusterDeployment *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      klusterletConfig.Spec.ClusterName + syncsetNamePostfix,
		Namespace: clusterDeployment.Namespace,
	}
}

func newSyncSet(
	client client.Client,
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
	clusterDeployment *hivev1.ClusterDeployment,
) (*hivev1.SyncSet, error) {
	runtimeObjects, err := GenerateImportObjects(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	runtimeRawExtensions := []runtime.RawExtension{}

	for _, obj := range runtimeObjects {
		runtimeRawExtensions = append(runtimeRawExtensions, runtime.RawExtension{Object: obj})
	}

	ssNsN := syncSetNsN(klusterletConfig, clusterDeployment)

	return &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssNsN.Name,
			Namespace: ssNsN.Namespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				Resources: runtimeRawExtensions,
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: klusterletConfig.Name,
				},
			},
		},
	}, nil
}

// GetSyncSet get the syncset use for installing klusterlet
func GetSyncSet(
	client client.Client,
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
	clusterDeployment *hivev1.ClusterDeployment,
) (*hivev1.SyncSet, error) {
	ssNsN := syncSetNsN(klusterletConfig, clusterDeployment)
	ss := &hivev1.SyncSet{}

	if err := client.Get(context.TODO(), ssNsN, ss); err != nil {
		return nil, err
	}

	return ss, nil
}

// CreateSyncSet create the syncset use for installing klusterlet
func CreateSyncSet(
	client client.Client,
	scheme *runtime.Scheme,
	cluster *clusterregistryv1alpha1.Cluster,
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
	clusterDeployment *hivev1.ClusterDeployment,
) (*hivev1.SyncSet, error) {
	ss, err := newSyncSet(client, klusterletConfig, clusterDeployment)
	if err != nil {
		return nil, err
	}
	// set ownerReference to klusterletconfig
	if err := controllerutil.SetControllerReference(cluster, ss, scheme); err != nil {
		return nil, err
	}

	if err := client.Create(context.TODO(), ss); err != nil {
		return nil, err
	}

	return ss, nil
}

// equalRawExtensions compares two rawExtensions and return true if they have same values
func equalRawExtensions(a, b *runtime.RawExtension) (bool, error) {
	aJSON, err := a.MarshalJSON()
	if err != nil {
		return false, err
	}
	bJSON, err := b.MarshalJSON()
	if err != nil {
		return false, err
	}
	var obj1, obj2 interface{}
	err = json.Unmarshal(aJSON, &obj1)
	if err != nil {
		return false, err
	}
	err = json.Unmarshal(bJSON, &obj2)
	if err != nil {
		return false, err
	}

	return reflect.DeepEqual(obj1, obj2), nil
}

// UpdateSyncSet updates the syncset base on klusterletConfig
func UpdateSyncSet(
	client client.Client,
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
	clusterDeployment *hivev1.ClusterDeployment,
	oldSyncSet *hivev1.SyncSet,
) (*hivev1.SyncSet, error) {
	runtimeObjects, err := GenerateImportObjects(client, klusterletConfig)
	if err != nil {
		return nil, err
	}
	isSame := len(oldSyncSet.Spec.SyncSetCommonSpec.Resources) == len(runtimeObjects)
	runtimeRawExtensions := []runtime.RawExtension{}
	for i, obj := range runtimeObjects {
		rawObj := runtime.RawExtension{Object: obj}
		runtimeRawExtensions = append(runtimeRawExtensions, rawObj)

		if isSame {
			if ok, _ := equalRawExtensions(&rawObj, &oldSyncSet.Spec.SyncSetCommonSpec.Resources[i]); !ok {
				isSame = false
			}
		}
	}

	if !isSame {
		oldSyncSet.Spec.SyncSetCommonSpec.Resources = runtimeRawExtensions
		if err := client.Update(context.TODO(), oldSyncSet); err != nil {
			return nil, err
		}
	}

	return oldSyncSet, nil
}
