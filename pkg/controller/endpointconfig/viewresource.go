//Package clusterregistry contains common utility functions that gets call by many differerent packages
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
package endpointconfig

import (
	"context"
	"encoding/json"

	mcmv1alpha1 "github.ibm.com/IBMPrivateCloud/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	multicloudv1alpha1 "github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/apis/multicloud/v1alpha1"
	multicloudv1beta1 "github.ibm.com/IBMPrivateCloud/ibm-klusterlet-operator/pkg/apis/multicloud/v1beta1"
)

func endpointConfigNsN(name string, namespace string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
}

// GetResourceView - if resourceview is present it returns true otherwise it returns false
func getEndpointResourceView(client client.Client, cluster *clusterregistryv1alpha1.Cluster) (*mcmv1alpha1.ResourceView, error) {
	ncNsN := endpointConfigNsN(cluster.Name+"-get-endpoint", cluster.Namespace)
	resourceview := &mcmv1alpha1.ResourceView{}
	if err := client.Get(context.TODO(), ncNsN, resourceview); err != nil {
		return nil, err
	}
	return resourceview, nil
}

//IsEndpointResourceviewCompleted - check if the resourceview completed
func IsEndpointResourceviewCompleted(resourceview *mcmv1alpha1.ResourceView) bool {
	if len(resourceview.Status.Conditions) > 0 {
		for _, condition := range resourceview.Status.Conditions {
			if condition.Type == mcmv1alpha1.WorkCompleted {
				return true
			}
		}
	}
	return false
}

// createEndpointResourceview - Creates resourceview to fetch endpoint from managed cluster
func createEndpointResourceview(
	r *ReconcileEndpointConfig,
	cluster *clusterregistryv1alpha1.Cluster,
	endpointConf *multicloudv1alpha1.EndpointConfig) (*mcmv1alpha1.ResourceView, error) {

	resourceView := &mcmv1alpha1.ResourceView{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ResourceView",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-get-endpoint",
			Namespace: cluster.Namespace,
		},
		Spec: mcmv1alpha1.ResourceViewSpec{
			ClusterSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": cluster.Name,
				},
			},
			SummaryOnly: false,
			Mode:        "",
			//UpdateIntervalSeconds: 10,
			Scope: mcmv1alpha1.ViewFilter{
				Resource:     "endpoint.multicloud.ibm.com",
				ResourceName: "endpoint",
				NameSpace:    "multicluster-endpoint",
			},
		},
	}

	if err := controllerutil.SetControllerReference(endpointConf, resourceView, r.scheme); err != nil {
		return nil, err
	}

	err := r.client.Create(context.TODO(), resourceView)
	if err != nil {
		return nil, err
	}

	return resourceView, nil
}

// GetEndpoint - Fetch the endpoint from managed cluster
func GetEndpoint(
	r *ReconcileEndpointConfig,
	cluster *clusterregistryv1alpha1.Cluster,
	resourceView *mcmv1alpha1.ResourceView) (*multicloudv1beta1.Endpoint, error) {

	resourceNamespace := types.NamespacedName{
		Name:      resourceView.Name,
		Namespace: cluster.Namespace,
	}

	completedResourceView := &mcmv1alpha1.ResourceView{}
	if err := r.apireader.Get(context.TODO(), resourceNamespace, completedResourceView); err != nil {
		return nil, err
	}

	endpoint := &multicloudv1beta1.Endpoint{}
	err := json.Unmarshal(completedResourceView.Status.Results[cluster.Name].Raw, endpoint)
	if err != nil {
		return nil, err
	}

	return endpoint, nil
}
