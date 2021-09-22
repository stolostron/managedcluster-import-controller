// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/controller"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	imgregistryv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/imageregistry/v1alpha1"

	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/component-base/logs"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Change below variables to serve metrics on different host or port.
var metricsPort = 8383

var (
	scheme   = k8sruntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(k8sscheme.AddToScheme(scheme))
	utilruntime.Must(ocinfrav1.AddToScheme(scheme))
	utilruntime.Must(hivev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(workv1.AddToScheme(scheme))
	utilruntime.Must(imgregistryv1alpha1.AddToScheme(scheme))
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Get a config to talk to the kube-apiserver
	cfg, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "failed to get kube config")
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create kube client")
		os.Exit(1)
	}

	apiExtensionsClient, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create api extensions client")
		os.Exit(1)
	}

	operatorClient, err := operatorclient.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create api extensions client")
		os.Exit(1)
	}

	// Create controller-runtime manager
	mgr, err := ctrl.NewManager(cfg, manager.Options{
		Scheme:             scheme,
		MetricsBindAddress: fmt.Sprintf(":%d", metricsPort),
		LeaderElection:     true,
		LeaderElectionID:   "managedcluster-import-controller.open-cluster-management.io",
	})
	if err != nil {
		setupLog.Error(err, "failed to create manager")
		os.Exit(1)
	}

	setupLog.Info("Registering Controllers")
	if err := controller.AddToManager(mgr, &helpers.ClientHolder{
		KubeClient:          kubeClient,
		APIExtensionsClient: apiExtensionsClient,
		OperatorClient:      operatorClient,
		RuntimeClient:       mgr.GetClient(),
	}); err != nil {
		setupLog.Error(err, "failed to register controller")
		os.Exit(1)
	}

	setupLog.Info("Starting Controller Manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "failed to start manager")
		os.Exit(1)
	}
}
