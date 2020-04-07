// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterdeployment ...
package clusterdeployment

import (
	"context"
	"fmt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func clusterRegistryNsN(clusterDeployment *hivev1.ClusterDeployment) (types.NamespacedName, error) {
	if clusterDeployment == nil {
		return types.NamespacedName{}, fmt.Errorf("func clusterRegistryNsN received nil pointer *hivev1.ClusterDeployment")
	}
	if clusterDeployment.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("func clusterRegistryNsN received empty string clusterDeployment.Spec.ClusterName")
	}
	return types.NamespacedName{
		Name:      clusterDeployment.Spec.ClusterName,
		Namespace: clusterDeployment.Spec.ClusterName,
	}, nil
}

func getClusterRegistryCluster(client client.Client, clusterDeployment *hivev1.ClusterDeployment) (*clusterregistryv1alpha1.Cluster, error) {
	crNsN, err := clusterRegistryNsN(clusterDeployment)
	if err != nil {
		return nil, fmt.Errorf("error from call to func clusterRegistryNsn")
	}
	cr := &clusterregistryv1alpha1.Cluster{}

	if err := client.Get(context.TODO(), crNsN, cr); err != nil {
		return nil, err
	}

	return cr, nil
}
