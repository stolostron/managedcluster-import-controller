//Package clusterdeployment ...
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
package clusterdeployment

import (
	"context"
	"fmt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func clusterRegistryNsN(clusterDeployment *hivev1.ClusterDeployment) (types.NamespacedName, error) {
	if clusterDeployment == nil {
		return types.NamespacedName{}, fmt.Errorf("func clusterRegistryNsN received nil pointer *hivev1.ClusterDeployment")
	}
	if clusterDeployment.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("func clusterRegistryNsN received empty string clusterDeployment.Spec.ClusterName")
	}
	return types.NamespacedName{
		Name:      clusterDeployment.Spec.ClusterName,
		Namespace: clusterDeployment.Spec.ClusterName,
	}, nil
}

func getClusterRegistryCluster(client client.Client, clusterDeployment *hivev1.ClusterDeployment) (*clusterregistryv1alpha1.Cluster, error) {
	crNsN, err := clusterRegistryNsN(clusterDeployment)
	if err != nil {
		return nil, fmt.Errorf("error from call to func clusterRegistryNsn")
	}
	cr := &clusterregistryv1alpha1.Cluster{}

	if err := client.Get(context.TODO(), crNsN, cr); err != nil {
		return nil, err
	}

	return cr, nil
}
