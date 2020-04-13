// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package controller

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"

	"github.com/open-cluster-management/rcm-controller/pkg/controller/clusterdeployment"
)

func init() {
	// AddToManagerFuncs is a list of functions and manadatory GVs to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, addToManager{
		function: clusterdeployment.Add,
		MandatoryGroupVersions: []schema.GroupVersion{
			hivev1.SchemeGroupVersion,
			clusterregistryv1alpha1.SchemeGroupVersion,
		},
	})
}
