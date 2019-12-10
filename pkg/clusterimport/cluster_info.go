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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const clusterInfoConfigMapName = "ibmcloud-cluster-info"
const clusterInfoConfigMapNamespace = "kube-public"

func clusterInfoConfigMapNsN() types.NamespacedName {
	return types.NamespacedName{
		Name:      clusterInfoConfigMapName,
		Namespace: clusterInfoConfigMapNamespace,
	}
}

func getKubeAPIServerAddress(client client.Client) (string, error) {
	configmap := &corev1.ConfigMap{}

	if err := client.Get(context.TODO(), clusterInfoConfigMapNsN(), configmap); err != nil {
		return "", err
	}

	apiServerHost, ok := configmap.Data["cluster_kube_apiserver_host"]
	if !ok {
		return "", fmt.Errorf("kube-public/ibmcloud-cluster-info does not contain cluster_kube_apiserver_host")
	}

	apiServerPort, ok := configmap.Data["cluster_kube_apiserver_port"]
	if !ok {
		return "https://" + apiServerHost, nil
	}

	return "https://" + apiServerHost + ":" + apiServerPort, nil
}
