// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package utils contains common utility functions that gets call by many differerent packages
package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func endpointConfigNsN(name string, namespace string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
}

// GetEndpointConfig - Get the endpoint config
func GetEndpointConfig(client client.Client, name string, namespace string) (*multicloudv1alpha1.EndpointConfig, error) {
	ncNsN := endpointConfigNsN(name, namespace)
	nc := &multicloudv1alpha1.EndpointConfig{}

	if err := client.Get(context.TODO(), ncNsN, nc); err != nil {
		return nil, err
	}

	return nc, nil
}
