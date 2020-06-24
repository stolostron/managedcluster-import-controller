// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package controller

import (
	"github.com/open-cluster-management/rcm-controller/pkg/controller/csr"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	// AddToManagerFuncs is a list of functions and manadatory GVs to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, addToManager{
		function:               csr.Add,
		MandatoryGroupVersions: []schema.GroupVersion{},
	})
}
