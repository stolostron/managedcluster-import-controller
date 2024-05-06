// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"fmt"

	"github.com/stolostron/managedcluster-import-controller/pkg/controller/autoimport"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/clusterdeployment"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/clusternamespacedeletion"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/csr"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/hosted"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/importconfig"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/importstatus"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/manifestwork"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/selfmanagedcluster"
	"github.com/stolostron/managedcluster-import-controller/pkg/features"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	kevents "k8s.io/client-go/tools/events"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var log = logf.Log.WithName("controllers")

type AddToManagerFunc func(
	context.Context, manager.Manager, *helpers.ClientHolder, *source.InformerHolder, kevents.EventRecorder,
) (string, error)

// AddToManagerFuncs is a list of functions to add all controllers to the manager
var AddToManagerFuncs = []AddToManagerFunc{
	csr.Add,
	managedcluster.Add,
	importconfig.Add,
	manifestwork.Add,
	selfmanagedcluster.Add,
	autoimport.Add,
	clusterdeployment.Add,
	clusternamespacedeletion.Add,
	importstatus.Add,
}

// AddToManager adds all controllers to the manager
func AddToManager(ctx context.Context,
	manager manager.Manager,
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	mcRecorder kevents.EventRecorder) error {
	for _, addFunc := range AddToManagerFuncs {
		name, err := addFunc(ctx, manager, clientHolder, informerHolder, mcRecorder)
		if err != nil {
			return err
		}

		log.Info(fmt.Sprintf("Add controller %s to manager", name))
	}

	if features.DefaultMutableFeatureGate.Enabled(features.KlusterletHostedMode) {
		name, err := hosted.Add(ctx, manager, clientHolder, informerHolder, mcRecorder)
		if err != nil {
			return err
		}

		log.Info(fmt.Sprintf("Add controller %s to manager", name))
	}
	return nil
}
