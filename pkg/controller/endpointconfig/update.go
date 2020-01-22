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

	mcmv1alpha1 "github.ibm.com/IBMPrivateCloud/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	multicloudv1alpha1 "github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/apis/multicloud/v1alpha1"
	multicloudv1beta1 "github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/apis/multicloud/v1beta1"
)

// EndpointUpdateWork ...
const EndpointUpdateWork = "update-multicluster-endpoint"

// getEndpointUpdateWork - fetch the endpoint update work
func getEndpointUpdateWork(r *ReconcileEndpointConfig, endpointc *multicloudv1alpha1.EndpointConfig) (*mcmv1alpha1.Work, error) {
	ncNsN := types.NamespacedName{
		Name:      endpointc.Name + "-update-endpoint",
		Namespace: endpointc.Namespace,
	}
	endpointWork := &mcmv1alpha1.Work{}
	if err := r.client.Get(context.TODO(), ncNsN, endpointWork); err != nil {
		return nil, err
	}

	return endpointWork, nil
}

// createEndpointUpdateWork - creates work to update endpoint on the managed cluster
func createEndpointUpdateWork(r *ReconcileEndpointConfig, endpointc *multicloudv1alpha1.EndpointConfig, endpoint *multicloudv1beta1.Endpoint) error {
	endpoint.Spec = endpointc.Spec
	work := &mcmv1alpha1.Work{
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointc.Name + "-update-endpoint",
			Namespace: endpointc.Namespace,
		},
		Spec: mcmv1alpha1.WorkSpec{
			Cluster: corev1.LocalObjectReference{
				Name: endpointc.Name,
			},
			ActionType: mcmv1alpha1.UpdateActionType,
			Type:       mcmv1alpha1.ActionWorkType,
			KubeWork: &mcmv1alpha1.KubeWorkSpec{
				Resource:  "endpoints.multicloud.ibm.com",
				Name:      EndpointUpdateWork,
				Namespace: "multicluster-endpoint",
				ObjectTemplate: runtime.RawExtension{
					Object: endpoint,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(endpointc, work, r.scheme); err != nil {
		return err
	}

	if err := r.client.Create(context.TODO(), work); err != nil {
		return err
	}

	return nil
}

// deleteEndpointUpdateWork - delete endpoint update work
func deleteEndpointUpdateWork(r *ReconcileEndpointConfig, work *mcmv1alpha1.Work) error {
	if err := r.client.Delete(context.TODO(), work); err != nil {
		return err
	}
	return nil
}
