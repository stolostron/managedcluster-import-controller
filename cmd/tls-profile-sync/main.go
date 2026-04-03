// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/managedcluster-import-controller/pkg/tlsprofilesync"
)

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	ctx := ctrl.SetupSignalHandler()
	setupLog := ctrl.Log.WithName("setup")
	if err := tlsprofilesync.Run(ctx); err != nil {
		setupLog.Error(err, "failed to run tls-profile-sync")
		os.Exit(1)
	}
}
