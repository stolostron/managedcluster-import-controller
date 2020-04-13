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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func clusterRegistryNamespaceNsN(clusterDeployment *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      clusterDeployment.Spec.ClusterName,
		Namespace: "",
	}
}

func getClusterRegistryNamespace(client client.Client, clusterDeployment *hivev1.ClusterDeployment) (*corev1.Namespace, error) {
	nsNsN := clusterRegistryNamespaceNsN(clusterDeployment)
	ns := &corev1.Namespace{}

	if err := client.Get(context.TODO(), nsNsN, ns); err != nil {
		return nil, err
	}

	return ns, nil
}
