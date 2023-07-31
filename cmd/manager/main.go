// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/spf13/pflag"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/agentregistration"
	"github.com/stolostron/managedcluster-import-controller/pkg/features"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	informersworkv1 "open-cluster-management.io/api/client/work/informers/externalversions/work/v1"
	listersworkv1 "open-cluster-management.io/api/client/work/listers/work/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"
	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	informerscorev1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	utilflag "k8s.io/component-base/cli/flag"
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
	utilruntime.Must(asv1beta1.AddToScheme(scheme))
	utilruntime.Must(addonv1alpha1.AddToScheme(scheme))
}

func main() {
	var leaderElectionNamespace string = ""
	pflag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "", "required when the process is not running in cluster")
	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	features.DefaultMutableFeatureGate.AddFlag(pflag.CommandLine)

	logs.AddFlags(pflag.CommandLine)
	pflag.Parse()

	logs.InitLogs()
	defer logs.FlushLogs()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
		o.TimeEncoder = func(ts time.Time, encoder zapcore.PrimitiveArrayEncoder) {
			encoder.AppendString(ts.UTC().Format(time.RFC3339Nano))
		}
	}))

	ctx := ctrl.SetupSignalHandler()

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
		setupLog.Error(err, "failed to create registration operator client")
		os.Exit(1)
	}

	workClient, err := workclient.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create work client")
		os.Exit(1)
	}

	importSecretInformer := informerscorev1.NewFilteredSecretInformer(
		kubeClient,
		metav1.NamespaceAll,
		10*time.Minute,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(listOptions *metav1.ListOptions) {
			selector := &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.ClusterImportSecretLabel,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			}
			listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
		},
	)

	autoimportSecretInformer := informerscorev1.NewFilteredSecretInformer(
		kubeClient,
		metav1.NamespaceAll,
		10*time.Minute,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(listOptions *metav1.ListOptions) {
			listOptions.FieldSelector = fields.OneTermEqualSelector("metadata.name", constants.AutoImportSecretName).String()
		},
	)

	klusterletWorksInformer := informersworkv1.NewFilteredManifestWorkInformer(
		workClient,
		metav1.NamespaceAll,
		10*time.Minute,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(listOptions *metav1.ListOptions) {
			selector := &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.KlusterletWorksLabel,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			}
			listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
		},
	)

	hostedWorksInformer := informersworkv1.NewFilteredManifestWorkInformer(
		workClient,
		metav1.NamespaceAll,
		10*time.Minute,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(listOptions *metav1.ListOptions) {
			selector := &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.HostedClusterLabel,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			}
			listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
		},
	)

	// Create controller-runtime manager
	mgr, err := ctrl.NewManager(cfg, manager.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      fmt.Sprintf(":%d", metricsPort),
		LeaderElection:          true,
		LeaderElectionID:        "managedcluster-import-controller.open-cluster-management.io",
		LeaderElectionNamespace: leaderElectionNamespace,
	})
	if err != nil {
		setupLog.Error(err, "failed to create manager")
		os.Exit(1)
	}

	clientHolder := &helpers.ClientHolder{
		KubeClient:          kubeClient,
		APIExtensionsClient: apiExtensionsClient,
		OperatorClient:      operatorClient,
		RuntimeClient:       mgr.GetClient(),
		ImageRegistryClient: imageregistry.NewClient(kubeClient),
		WorkClient:          workClient,
	}

	setupLog.Info("Registering Controllers")
	if err := controller.AddToManager(
		mgr,
		clientHolder,
		&source.InformerHolder{
			ImportSecretInformer:     importSecretInformer,
			ImportSecretLister:       listerscorev1.NewSecretLister(importSecretInformer.GetIndexer()),
			AutoImportSecretInformer: autoimportSecretInformer,
			AutoImportSecretLister:   listerscorev1.NewSecretLister(autoimportSecretInformer.GetIndexer()),
			KlusterletWorkInformer:   klusterletWorksInformer,
			KlusterletWorkLister:     listersworkv1.NewManifestWorkLister(klusterletWorksInformer.GetIndexer()),
			HostedWorkInformer:       hostedWorksInformer,
			HostedWorkLister:         listersworkv1.NewManifestWorkLister(hostedWorksInformer.GetIndexer()),
		},
	); err != nil {
		setupLog.Error(err, "failed to register controller")
		os.Exit(1)
	}

	// Start the agent-registratioin server
	if features.DefaultMutableFeatureGate.Enabled(features.AgentRegistration) {
		agentRegistrationServer := agentregistration.NewAgentRegistrationServer(9091, clientHolder)
		go func() {
			if err := agentRegistrationServer.Run(ctx); err != nil {
				setupLog.Error(err, "failed to start agent registration server")
			}
		}()
	}

	go importSecretInformer.Run(ctx.Done())
	go autoimportSecretInformer.Run(ctx.Done())
	go klusterletWorksInformer.Run(ctx.Done())
	go hostedWorksInformer.Run(ctx.Done())

	setupLog.Info("Starting Controller Manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "failed to start manager")
		os.Exit(1)
	}
}
