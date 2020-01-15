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
package clusterregistry

import (
	"context"

	"github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/clusterimport"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"

	"github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/utils"
	mcmv1alpha1 "github.ibm.com/IBMPrivateCloud/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
)

// constants for delete work and finalizer
const (
	EndpointDeleteWork = "delete-multicluster-endpoint"
	ClusterFinalizer   = "rcm-api.cluster"
)

func getDeleteWork(r *ReconcileCluster, cluster *clusterregistryv1alpha1.Cluster) (*mcmv1alpha1.Work, error) {
	clusterNamespace := types.NamespacedName{
		Name:      EndpointDeleteWork,
		Namespace: cluster.Namespace,
	}
	deleteWork := &mcmv1alpha1.Work{}
	if err := r.client.Get(context.TODO(), clusterNamespace, deleteWork); err != nil {
		return nil, err
	}

	return deleteWork, nil
}

func isClusterOnline(cluster *clusterregistryv1alpha1.Cluster) bool {
	for _, condition := range cluster.Status.Conditions {
		if condition.Type == clusterregistryv1alpha1.ClusterOK {
			return true
		}
	}

	return false
}

func newDeleteJob(r *ReconcileCluster, cluster *clusterregistryv1alpha1.Cluster) (*batchv1.Job, error) {
	endpointConfig, err := utils.GetEndpointConfig(r.client, cluster.Name, cluster.Namespace)
	if err != nil {
		return nil, err
	}

	operatorImageName := endpointConfig.Spec.ImageRegistry +
		"/" + clusterimport.EndpointOperatorImageName +
		endpointConfig.Spec.ImageNamePostfix +
		":" + endpointConfig.Spec.Version

	jobBackoff := int32(0) // 0 = no retries before the job fails
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: batchv1.SchemeGroupVersion.String(),
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      EndpointDeleteWork,
			Namespace: clusterimport.EndpointNamespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &jobBackoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: clusterimport.EndpointOperatorName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "self-destruct",
							Image:   operatorImageName,
							Command: []string{"self-destruct.sh"},
						},
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: endpointConfig.Spec.ImagePullSecret,
						},
					},
				},
			},
		},
	}, nil
}

func createDeleteWork(r *ReconcileCluster, cluster *clusterregistryv1alpha1.Cluster) error {
	job, err := newDeleteJob(r, cluster)
	if err != nil {
		return err
	}

	work := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      EndpointDeleteWork,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterregistryv1alpha1.SchemeGroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
		},
		Spec: mcmv1alpha1.WorkSpec{
			Cluster: corev1.LocalObjectReference{
				Name: cluster.Name,
			},
			ActionType: mcmv1alpha1.CreateActionType,
			Type:       mcmv1alpha1.ActionWorkType,
			KubeWork: &mcmv1alpha1.KubeWorkSpec{
				Resource:  "job",
				Name:      EndpointDeleteWork,
				Namespace: clusterimport.EndpointNamespace,
				ObjectTemplate: runtime.RawExtension{
					Object: job,
				},
			},
		},
	}

	if err := r.client.Create(context.TODO(), work); err != nil {
		return err
	}
	return nil
}
