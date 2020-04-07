// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package endpointconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

type clusterReconcileMapper struct{}

func (mapper *clusterReconcileMapper) Map(obj handler.MapObject) []reconcile.Request {
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      obj.Meta.GetName(),
				Namespace: obj.Meta.GetNamespace(),
			},
		},
	}
}

func clusterRegistryNsN(endpointConfig *multicloudv1alpha1.EndpointConfig) (types.NamespacedName, error) {
	if endpointConfig == nil {
		return types.NamespacedName{}, fmt.Errorf("endpointConfig is nil")
	}

	if endpointConfig.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("endpointConfig.Spec.ClusterName is empty")
	}

	if endpointConfig.Spec.ClusterNamespace == "" {
		return types.NamespacedName{}, fmt.Errorf("endpointConfig.Spec.ClusterNamespace is empty")
	}

	return types.NamespacedName{
		Name:      endpointConfig.Spec.ClusterName,
		Namespace: endpointConfig.Spec.ClusterNamespace,
	}, nil
}

func getClusterRegistryCluster(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*clusterregistryv1alpha1.Cluster, error) {
	crNsN, err := clusterRegistryNsN(endpointConfig)
	if err != nil {
		return nil, err
	}

	cr := &clusterregistryv1alpha1.Cluster{}

	if err := client.Get(context.TODO(), crNsN, cr); err != nil {
		return nil, err
	}

	return cr, nil
}
