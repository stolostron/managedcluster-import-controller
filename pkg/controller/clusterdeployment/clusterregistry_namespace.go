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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func clusterRegistryNamespaceNsN(clusterDeployment *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      clusterDeployment.Spec.ClusterName,
		Namespace: "",
	}
}

func getClusterRegistryNamespace(client client.Client, clusterDeployment *hivev1.ClusterDeployment) (*corev1.Namespace, error) {
	nsNsN := clusterRegistryNamespaceNsN(clusterDeployment)
	ns := &corev1.Namespace{}

	if err := client.Get(context.TODO(), nsNsN, ns); err != nil {
		return nil, err
	}

	return ns, nil
}
