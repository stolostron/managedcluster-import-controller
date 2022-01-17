// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/csr"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	// AddToManagerFuncs is a list of functions and manadatory GVs to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, addToManager{
		function:               csr.Add,
		MandatoryGroupVersions: []schema.GroupVersion{},
	})
}
