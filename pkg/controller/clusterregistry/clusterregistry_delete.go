// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterregistry contains common utility functions that gets call by many differerent packages
package clusterregistry

import (
	"context"
	"os"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"

	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	"github.com/open-cluster-management/rcm-controller/pkg/clusterimport"
	"github.com/open-cluster-management/rcm-controller/pkg/utils"
)

// constants for delete work and finalizer
const (
	EndpointDeleteWork = "delete-multicluster-endpoint"
	ClusterFinalizer   = "rcm-controller.cluster"
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

// IsClusterOnline - if cluster is online returns true otherwise returns false
func IsClusterOnline(cluster *clusterregistryv1alpha1.Cluster) bool {
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

	// set up default values for endpointconfig
	if endpointConfig.Spec.ClusterNamespace == "" {
		endpointConfig.Spec.ClusterNamespace = endpointConfig.Namespace
	}

	if endpointConfig.Spec.ImagePullSecret == "" {
		endpointConfig.Spec.ImagePullSecret = os.Getenv("DEFAULT_IMAGE_PULL_SECRET")
	}

	if endpointConfig.Spec.ImageRegistry == "" {
		endpointConfig.Spec.ImageRegistry = os.Getenv("DEFAULT_IMAGE_REGISTRY")
	}

	operatorImageName, _, _ := clusterimport.GetEndpointOperatorImage(endpointConfig)

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
