// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/managedcluster"
	"k8s.io/apimachinery/pkg/runtime/schema"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

func init() {
	// AddToManagerFuncs is a list of functions and manadatory GVs to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, addToManager{
		function: managedcluster.Add,
		MandatoryGroupVersions: []schema.GroupVersion{
			clusterv1.SchemeGroupVersion,
			workv1.SchemeGroupVersion,
			hivev1.SchemeGroupVersion,
		},
	})
}
