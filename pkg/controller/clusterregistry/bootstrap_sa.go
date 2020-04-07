// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterregistry ...
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

// NewBootstrapServiceAccount initialize a new bootstrap serviceaccount
func NewBootstrapServiceAccount(cluster *clusterregistryv1alpha1.Cluster) (*corev1.ServiceAccount, error) {
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
	sa, err := NewBootstrapServiceAccount(cluster)
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
