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
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/flightctl"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/hosted"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/importconfig"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/importstatus"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/manifestwork"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/resourcecleanup"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/selfmanagedcluster"
	"github.com/stolostron/managedcluster-import-controller/pkg/features"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	certificatesv1 "k8s.io/api/certificates/v1"
	kevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManager adds all controllers to the manager
func AddToManager(ctx context.Context,
	manager manager.Manager,
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	componentNamespace string,
	flightctlManager *flightctl.FlightCtlManager,
	mcRecorder kevents.EventRecorder) error {

	extraCSRApprovalConditions := []func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error){
		func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error) {
			return flightctlManager.IsManagedClusterAFlightctlDevice(ctx, helpers.GetClusterName(csr))
		},
	}

	AddToManagerFuncs := []struct {
		ControllerName string
		Add            func() error
	}{
		{
			csr.ControllerName,
			func() error {
				return csr.Add(ctx, manager, clientHolder, extraCSRApprovalConditions)
			},
		},
		{
			managedcluster.ControllerName,
			func() error { return managedcluster.Add(ctx, manager, clientHolder, mcRecorder) },
		},
		{
			importconfig.ControllerName,
			func() error { return importconfig.Add(ctx, manager, clientHolder, informerHolder, componentNamespace) },
		},
		{
			manifestwork.ControllerName,
			func() error { return manifestwork.Add(ctx, manager, clientHolder, informerHolder, mcRecorder) },
		},
		{
			selfmanagedcluster.ControllerName,
			func() error {
				return selfmanagedcluster.Add(ctx, manager, clientHolder, informerHolder, mcRecorder, componentNamespace)
			},
		},
		{
			autoimport.ControllerName,
			func() error {
				return autoimport.Add(ctx, manager, clientHolder, informerHolder, mcRecorder, componentNamespace)
			},
		},
		{
			clusterdeployment.ControllerName,
			func() error {
				return clusterdeployment.Add(ctx, manager, clientHolder, informerHolder, mcRecorder, componentNamespace)
			},
		},
		{
			clusternamespacedeletion.ControllerName,
			func() error { return clusternamespacedeletion.Add(ctx, manager, clientHolder) },
		},
		{
			importstatus.ControllerName,
			func() error { return importstatus.Add(ctx, manager, clientHolder, informerHolder, mcRecorder) },
		},
		{
			hosted.ControllerName,
			func() error {
				if features.DefaultMutableFeatureGate.Enabled(features.KlusterletHostedMode) {
					return hosted.Add(ctx, manager, clientHolder, informerHolder, mcRecorder)
				}
				return nil
			},
		},
		{
			resourcecleanup.ControllerName,
			func() error {
				return resourcecleanup.Add(ctx, manager, clientHolder, mcRecorder)
			},
		},
		{
			flightctl.ManagedClusterControllerName,
			func() error {
				return flightctl.AddManagedClusterController(ctx, manager, flightctlManager, clientHolder)
			},
		},
	}

	for _, f := range AddToManagerFuncs {
		if err := f.Add(); err != nil {
			return fmt.Errorf("failed to add %s controller: %w", f.ControllerName, err)
		}
	}

	return nil
}
