//Package clusterregistry ...
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
package clusterregistry

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const bootstrapServiceAccountNamePostfix = "-bootstrap-sa"

func bootstrapServiceAccountNsN(cluster *clusterregistryv1alpha1.Cluster) (types.NamespacedName, error) {
	if cluster == nil {
		return types.NamespacedName{}, fmt.Errorf("nil Cluster")
	}

	if cluster.Name == "" {
		return types.NamespacedName{}, fmt.Errorf("empty Cluster.Name")
	}

	return types.NamespacedName{
		Name:      cluster.Name + bootstrapServiceAccountNamePostfix,
		Namespace: cluster.Namespace,
	}, nil
}

func newBootstrapServiceAccount(cluster *clusterregistryv1alpha1.Cluster) (*corev1.ServiceAccount, error) {
	saNsN, err := bootstrapServiceAccountNsN(cluster)
	if err != nil {
		return nil, err
	}

	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saNsN.Name,
			Namespace: saNsN.Namespace,
		},
	}, nil
}

// getBootstrapServiceAccount get the service account use for multicluster-endpoint bootstrap
func getBootstrapServiceAccount(r *ReconcileCluster, cluster *clusterregistryv1alpha1.Cluster) (*corev1.ServiceAccount, error) {
	saNsN, err := bootstrapServiceAccountNsN(cluster)
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{}

	if err := r.client.Get(context.TODO(), saNsN, sa); err != nil {
		return nil, err
	}

	return sa, nil
}

// createBootstrapServiceAccount create the service account use for multicluster-endpoint bootstrap
func createBootstrapServiceAccount(r *ReconcileCluster, cluster *clusterregistryv1alpha1.Cluster) (*corev1.ServiceAccount, error) {
	sa, err := newBootstrapServiceAccount(cluster)
	if err != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(cluster, sa, r.scheme); err != nil {
		return nil, err
	}

	if err := r.client.Create(context.TODO(), sa); err != nil {
		return nil, err
	}

	return sa, nil
}
