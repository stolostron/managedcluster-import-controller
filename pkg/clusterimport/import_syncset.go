//Package clusterimport ...
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package clusterimport

import (
	"context"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

const syncsetNamePostfix = "-multicluster-endpoint"

func syncSetNsN(endpointConfig *multicloudv1alpha1.EndpointConfig, clusterDeployment *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      endpointConfig.Spec.ClusterName + syncsetNamePostfix,
		Namespace: clusterDeployment.Namespace,
	}
}

func newSyncSet(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig, clusterDeployment *hivev1.ClusterDeployment) (*hivev1.SyncSet, error) {
	runtimeObjects, err := GenerateImportObjects(client, endpointConfig)
	if err != nil {
		return nil, err
	}

	runtimeRawExtensions := []runtime.RawExtension{}

	for _, obj := range runtimeObjects {
		runtimeRawExtensions = append(runtimeRawExtensions, runtime.RawExtension{Object: obj})
	}

	ssNsN := syncSetNsN(endpointConfig, clusterDeployment)

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
					Name: endpointConfig.Name,
				},
			},
		},
	}, nil
}

// GetSyncSet get the syncset use for installing multicluster-endpoint
func GetSyncSet(
	client client.Client,
	endpointConfig *multicloudv1alpha1.EndpointConfig,
	clusterDeployment *hivev1.ClusterDeployment,
) (*hivev1.SyncSet, error) {
	ssNsN := syncSetNsN(endpointConfig, clusterDeployment)
	ss := &hivev1.SyncSet{}

	if err := client.Get(context.TODO(), ssNsN, ss); err != nil {
		return nil, err
	}

	return ss, nil
}

// CreateSyncSet create the syncset use for installing multicluster-endpoint
func CreateSyncSet(
	client client.Client,
	endpointConfig *multicloudv1alpha1.EndpointConfig,
	clusterDeployment *hivev1.ClusterDeployment,
) (*hivev1.SyncSet, error) {
	ss, err := newSyncSet(client, endpointConfig, clusterDeployment)
	if err != nil {
		return nil, err
	}

	if err := client.Create(context.TODO(), ss); err != nil {
		return nil, err
	}

	return ss, nil
}
