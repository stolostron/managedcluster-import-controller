// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package klusterletconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
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

func clusterRegistryNsN(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (types.NamespacedName, error) {
	if klusterletConfig == nil {
		return types.NamespacedName{}, fmt.Errorf("klusterletConfig is nil")
	}

	if klusterletConfig.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("klusterletConfig.Spec.ClusterName is empty")
	}

	if klusterletConfig.Spec.ClusterNamespace == "" {
		return types.NamespacedName{}, fmt.Errorf("klusterletConfig.Spec.ClusterNamespace is empty")
	}

	return types.NamespacedName{
		Name:      klusterletConfig.Spec.ClusterName,
		Namespace: klusterletConfig.Spec.ClusterNamespace,
	}, nil
}

func getClusterRegistryCluster(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*clusterregistryv1alpha1.Cluster, error) {
	crNsN, err := clusterRegistryNsN(klusterletConfig)
	if err != nil {
		return nil, err
	}

	cr := &clusterregistryv1alpha1.Cluster{}

	if err := client.Get(context.TODO(), crNsN, cr); err != nil {
		return nil, err
	}

	return cr, nil
}
