// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/autoimport"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/clusterdeployment"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/csr"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/importconfig"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/managedcluster"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/manifestwork"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller/selfmanagedcluster"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var log = logf.Log.WithName("controllers")

type AddToManagerFunc func(manager.Manager, *helpers.ClientHolder) (string, error)

// AddToManagerFuncs is a list of functions to add all controllers to the manager
var AddToManagerFuncs = []AddToManagerFunc{
	csr.Add,
	managedcluster.Add,
	importconfig.Add,
	manifestwork.Add,
	selfmanagedcluster.Add,
	autoimport.Add,
	clusterdeployment.Add,
}

// AddToManager adds all controllers to the manager
func AddToManager(manager manager.Manager, clientHolder *helpers.ClientHolder) error {
	for _, addFunc := range AddToManagerFuncs {
		name, err := addFunc(manager, clientHolder)
		if err != nil {
			return err
		}

		log.Info(fmt.Sprintf("Add controller %s to manager", name))
	}

	return nil
}
