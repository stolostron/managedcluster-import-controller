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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const selectorSyncsetName = "multicluster-endpoint"
const selectorSyncsetLabelName = "cluster-managed"
const selectorSyncsetLabelValue = "true"

func selectorSyncsetNsN() types.NamespacedName {
	return types.NamespacedName{
		Name:      selectorSyncsetName,
		Namespace: "",
	}
}

// HasClusterManagedLabels checks whether a clusterdeployment contains labels used by the selectorSyncset
func HasClusterManagedLabels(clusterDeployment *hivev1.ClusterDeployment) bool {
	if clusterDeployment.ObjectMeta.Labels == nil {
		return false
	}
	if val, ok := clusterDeployment.ObjectMeta.Labels[selectorSyncsetLabelName]; ok && val == selectorSyncsetLabelValue {
		return true
	}
	return false
}

// AddClusterManagedLabels adds labels and returns a clusterDeployment with new labels
func AddClusterManagedLabels(clusterDeployment *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	// shallow copy
	res := *clusterDeployment
	// ready to clone labels
	res.ObjectMeta.Labels = make(map[string]string)
	for k, v := range clusterDeployment.ObjectMeta.Labels {
		res.ObjectMeta.Labels[k] = v
	}

	res.ObjectMeta.Labels[selectorSyncsetLabelName] = selectorSyncsetLabelValue
	return &res
}

// RemoveClusterManagedLabels removes labels and returns a clusterDeployment with new labels
func RemoveClusterManagedLabels(clusterDeployment *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	if clusterDeployment.ObjectMeta.Labels == nil {
		return clusterDeployment
	}

	// shallow copy
	res := *clusterDeployment
	// ready to clone labels
	res.ObjectMeta.Labels = make(map[string]string)
	for k, v := range clusterDeployment.ObjectMeta.Labels {
		if k != selectorSyncsetLabelName {
			res.ObjectMeta.Labels[k] = v
		}
	}
	return &res
}

// newSelectorSyncset generate the SelectorSyncset for installing multicluster-endpoint
func newSelectorSyncset() (*hivev1.SelectorSyncSet, error) {
	runtimeObjects, err := generateCommonImportObjects()
	if err != nil {
		return nil, err
	}

	sssNsN := selectorSyncsetNsN()

	runtimeRawExtensions := []runtime.RawExtension{}
	for _, obj := range runtimeObjects {
		runtimeRawExtensions = append(runtimeRawExtensions, runtime.RawExtension{Object: obj})
	}

	sss := &hivev1.SelectorSyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SelectorSyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sssNsN.Name,
			Namespace: sssNsN.Namespace,
		},
		Spec: hivev1.SelectorSyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				Resources: runtimeRawExtensions,
			},
		},
	}
	metav1.AddLabelToSelector(
		&sss.Spec.ClusterDeploymentSelector,
		selectorSyncsetLabelName,
		selectorSyncsetLabelValue,
	)
	return sss, nil
}

// GetSelectorSyncset get the selector syncset use for installing multicluster-endpoint
func GetSelectorSyncset(client client.Client) (*hivev1.SelectorSyncSet, error) {
	sss := &hivev1.SelectorSyncSet{}

	if err := client.Get(context.TODO(), selectorSyncsetNsN(), sss); err != nil {
		return nil, err
	}

	return sss, nil
}

// CreateSelectorSyncset create the selector syncset use for installing multicluster-endpoint
func CreateSelectorSyncset(client client.Client) (*hivev1.SelectorSyncSet, error) {
	sss, err := newSelectorSyncset()
	if err != nil {
		return nil, err
	}

	if err := client.Create(context.TODO(), sss); err != nil {
		return nil, err
	}

	return sss, nil
}
