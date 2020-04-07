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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func endpointConfigNsN(clusterDeployment *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      clusterDeployment.Spec.ClusterName,
		Namespace: clusterDeployment.Spec.ClusterName,
	}
}

func getEndpointConfig(client client.Client, clusterDeployment *hivev1.ClusterDeployment) (*multicloudv1alpha1.EndpointConfig, error) {
	ncNsN := endpointConfigNsN(clusterDeployment)
	nc := &multicloudv1alpha1.EndpointConfig{}

	if err := client.Get(context.TODO(), ncNsN, nc); err != nil {
		return nil, err
	}

	return nc, nil
}
