// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	"github.com/open-cluster-management/rcm-controller/pkg/controller"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	ocinfrav1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"

	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.
	"k8s.io/client-go/rest"
	rbacv1 "k8s.io/kubernetes/pkg/apis/rbac/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ctrl "sigs.k8s.io/controller-runtime"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost             = "0.0.0.0"
	metricsPort         int = 8383
	operatorMetricsPort int = 8686
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

var log = logf.Log.WithName("cmd")

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	klog.InitFlags(nil)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	// Get a config to talk to the apiserver

	cfg := ctrl.GetConfigOrDie()

	enableLeaderElection = false
	if _, err := rest.InClusterConfig(); err == nil {
		setupLog.Info("LeaderElection enabled as running in a cluster")
		enableLeaderElection = true
	} else {
		setupLog.Info("LeaderElection disabled as not running in a cluster")
	}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Namespace:          os.Getenv("WATCH_NAMESPACE"),
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		Port:               operatorMetricsPort,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "managedcluster-import-controller-leader.open-cluster-management.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources

	if err := ocinfrav1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := hivev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := corev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := clusterv1.Install(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := workv1.Install(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := rbacv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := certificatesv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Setup manager with controllers")
	missingGVS, err := controller.GetMissingGVS(cfg)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	//Channel to stop the manager
	stopMgrCh := make(chan struct{})

	if err := controller.AddToManager(mgr, missingGVS); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	nbOfMissingGVS := len(missingGVS)

	//If some CRD are not yet installled then we will monitor them
	if nbOfMissingGVS != 0 {
		go func() {
			change := false
			for !change {
				time.Sleep(time.Second * 10)
				currentMissingGVS, err := controller.GetMissingGVS(cfg)
				if err != nil {
					log.Error(err, "")
					os.Exit(1)
				}
				change = len(currentMissingGVS) != nbOfMissingGVS
				if change {
					log.Info(fmt.Sprintf("Old missing GVS: %v", missingGVS))
					log.Info(fmt.Sprintf("New missing GVS: %v", currentMissingGVS))
				}
			}
			//Close the manager
			log.Error(fmt.Errorf("new CRD discovered %s", ""),
				"This is an expected behavior, the operator stopped because a new CRD managed by this operator get discovered")
			close(stopMgrCh)
		}()
	}

	// Start the Cmd
	if err := mgr.Start(stopMgrCh); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}
