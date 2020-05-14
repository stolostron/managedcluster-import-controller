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

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

func klusterletConfigNsN(clusterDeployment *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      clusterDeployment.Spec.ClusterName,
		Namespace: clusterDeployment.Spec.ClusterName,
	}
}

func getKlusterletConfig(client client.Client, clusterDeployment *hivev1.ClusterDeployment) (*klusterletcfgv1beta1.KlusterletConfig, error) {
	ncNsN := klusterletConfigNsN(clusterDeployment)
	nc := &klusterletcfgv1beta1.KlusterletConfig{}

	if err := client.Get(context.TODO(), ncNsN, nc); err != nil {
		return nil, err
	}

	return nc, nil
}
