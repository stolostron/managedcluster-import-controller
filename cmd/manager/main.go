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
	"strconv"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/agentregistration"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/flightctl"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/importconfig"
	"github.com/stolostron/managedcluster-import-controller/pkg/features"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	"k8s.io/client-go/informers"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/cache"

	ocinfrav1 "github.com/openshift/api/config/v1"
	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	klusterletconfigclient "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/clientset/versioned"
	klusterletconfiginformer "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/informers/externalversions"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	informerscluster "open-cluster-management.io/api/client/cluster/informers/externalversions"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	informerswork "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"

	"net/http"
	// #nosec G108
	_ "net/http/pprof"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Change below variables to serve metrics on different host or port.
var metricsPort = 8383

var (
	Burst int     = 100
	QPS   float32 = 50
)

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
	utilruntime.Must(klusterletconfigv1alpha1.AddToScheme(scheme))
	utilruntime.Must(hyperv1beta1.AddToScheme(scheme))
}

func main() {
	var leaderElectionNamespace string = ""
	var enablePprof bool = false
	if enablePprofEnv, exists := os.LookupEnv("ENABLE_PPROF"); exists {
		var err error
		enablePprof, err = strconv.ParseBool(enablePprofEnv)
		if err != nil {
			setupLog.Error(err, "failed to parse ENABLE_PPROF environment variable")
			os.Exit(1)
		}
	}

	var clusterIngressDomain string
	var enableFlightCtl bool = false

	pflag.StringVar(&clusterIngressDomain, "cluster-ingress-domain", "", "the ingress domain of the cluster")
	pflag.BoolVar(&enableFlightCtl, "enable-flightctl", false, "enable flightctl")

	pflag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "", "required when the process is not running in cluster")
	pflag.BoolVar(&helpers.DeployOnOCP, "deploy-on-ocp", true, "used to deploy the controller on OCP or not")
	pflag.Float32Var(&QPS, "kube-api-qps", 50, "QPS indicates the maximum QPS to the master from this client")
	pflag.IntVar(&Burst, "kube-api-burst", 100, "Burst indicates the maximum burst for throttle")
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

	fmt.Println("Starting Controller Manager")

	ctx := ctrl.SetupSignalHandler()

	// Get a config to talk to the kube-apiserver
	cfg, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "failed to get kube config")
		os.Exit(1)
	}

	cfg.QPS = QPS
	cfg.Burst = Burst

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

	klusterletconfigClient, err := klusterletconfigclient.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create klusterletconfig client")
		os.Exit(1)
	}

	managedclusterClient, err := clusterclient.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create managedcluster client")
		os.Exit(1)
	}

	importSecertInformerF := informers.NewFilteredSharedInformerFactory(
		kubeClient,
		10*time.Minute,
		metav1.NamespaceAll, func(listOptions *metav1.ListOptions) {
			selector := &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.ClusterImportSecretLabel,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			}
			listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
		})

	autoimportSecretInformerF := informers.NewFilteredSharedInformerFactory(
		kubeClient,
		10*time.Minute,
		metav1.NamespaceAll, func(listOptions *metav1.ListOptions) {
			listOptions.FieldSelector = fields.OneTermEqualSelector("metadata.name", constants.AutoImportSecretName).String()
		},
	)

	klusterletWorksInformerF := informerswork.NewFilteredSharedInformerFactory(
		workClient,
		10*time.Minute,
		metav1.NamespaceAll, func(listOptions *metav1.ListOptions) {
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

	hostedWorksInformerF := informerswork.NewFilteredSharedInformerFactory(
		workClient,
		10*time.Minute,
		metav1.NamespaceAll, func(listOptions *metav1.ListOptions) {
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

	klusterletconfigInformerF := klusterletconfiginformer.NewSharedInformerFactory(klusterletconfigClient, 10*time.Minute)
	klusterletconfigInformer := klusterletconfigInformerF.Config().V1alpha1().KlusterletConfigs().Informer()
	if err := klusterletconfigInformer.AddIndexers(
		cache.Indexers{
			importconfig.KlusterletConfigBootstrapKubeConfigSecretsIndexKey: importconfig.IndexKlusterletConfigByBootstrapKubeConfigSecrets(),
			importconfig.KlusterletConfigCustomizedCAConfigmapsIndexKey:     importconfig.IndexKlusterletConfigByCustomizedCAConfigmaps(),
		},
	); err != nil {
		setupLog.Error(err, "failed to add indexers to klusterletconfig informer")
		os.Exit(1)
	}
	klusterletconfigLister := klusterletconfigInformerF.Config().V1alpha1().KlusterletConfigs().Lister()

	// managedclusterInformer has an index on the klusterletconfig annotation, so we can get all managed clusters
	// affected by a klusterletconfig change.
	managedclusterInformerF := informerscluster.NewSharedInformerFactory(managedclusterClient, 10*time.Minute)
	managedclusterInformer := managedclusterInformerF.Cluster().V1().ManagedClusters().Informer()
	if err := managedclusterInformer.AddIndexers(
		cache.Indexers{
			importconfig.ManagedClusterKlusterletConfigAnnotationIndexKey: importconfig.IndexManagedClusterByKlusterletconfigAnnotation,
		},
	); err != nil {
		setupLog.Error(err, "failed to add indexers to managedcluster informer")
		os.Exit(1)
	}

	// Create controller-runtime manager
	mgr, err := ctrl.NewManager(cfg, manager.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: fmt.Sprintf(":%d", metricsPort),
		},
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
		RuntimeAPIReader:    mgr.GetAPIReader(),
		ImageRegistryClient: imageregistry.NewClient(kubeClient),
		WorkClient:          workClient,
	}

	// Create a new ManagedCluster EventRecorder which can be used to record events
	// with involvedObject set to the ManagedCluster.
	mcRecorder := helpers.NewManagedClusterEventRecorder(ctx, clientHolder.KubeClient)

	// Init flightctlManager
	flightctlManager := flightctl.NewFlightCtlManager(clientHolder, clusterIngressDomain)

	setupLog.Info("Registering Controllers")
	if err := controller.AddToManager(
		ctx,
		mgr,
		clientHolder,
		&source.InformerHolder{
			ImportSecretInformer:     importSecertInformerF.Core().V1().Secrets().Informer(),
			ImportSecretLister:       importSecertInformerF.Core().V1().Secrets().Lister(),
			AutoImportSecretInformer: autoimportSecretInformerF.Core().V1().Secrets().Informer(),
			AutoImportSecretLister:   autoimportSecretInformerF.Core().V1().Secrets().Lister(),
			KlusterletWorkInformer:   klusterletWorksInformerF.Work().V1().ManifestWorks().Informer(),
			KlusterletWorkLister:     klusterletWorksInformerF.Work().V1().ManifestWorks().Lister(),
			HostedWorkInformer:       hostedWorksInformerF.Work().V1().ManifestWorks().Informer(),
			HostedWorkLister:         hostedWorksInformerF.Work().V1().ManifestWorks().Lister(),
			KlusterletConfigInformer: klusterletconfigInformer,
			KlusterletConfigLister:   klusterletconfigLister,
			ManagedClusterInformer:   managedclusterInformer,
		},
		enableFlightCtl,
		flightctlManager,
		mcRecorder,
	); err != nil {
		setupLog.Error(err, "failed to register controller")
		os.Exit(1)
	}

	importSecertInformerF.Start(ctx.Done())
	autoimportSecretInformerF.Start(ctx.Done())
	klusterletWorksInformerF.Start(ctx.Done())
	hostedWorksInformerF.Start(ctx.Done())
	klusterletconfigInformerF.Start(ctx.Done())
	managedclusterInformerF.Start(ctx.Done())

	importSecertInformerF.WaitForCacheSync(ctx.Done())
	autoimportSecretInformerF.WaitForCacheSync(ctx.Done())
	klusterletWorksInformerF.WaitForCacheSync(ctx.Done())
	hostedWorksInformerF.WaitForCacheSync(ctx.Done())
	klusterletconfigInformerF.WaitForCacheSync(ctx.Done())
	managedclusterInformerF.WaitForCacheSync(ctx.Done())

	// Start the agent-registratioin server
	if features.DefaultMutableFeatureGate.Enabled(features.AgentRegistration) {
		go func() {
			if err := agentregistration.RunAgentRegistrationServer(ctx, 9091, clientHolder,
				klusterletconfigLister); err != nil {
				setupLog.Error(err, "failed to start agent registration server")
			}
		}()
	}

	if enableFlightCtl {
		err = flightctlManager.ApplyResources(ctx)
		if err != nil {
			setupLog.Error(err, "failed to install FlightCtl resources")
			os.Exit(1)
		}
	}

	if enablePprof {
		go func() {
			server := &http.Server{
				Addr:           "localhost:6060",
				Handler:        nil,
				ReadTimeout:    5 * time.Second,
				WriteTimeout:   10 * time.Second,
				IdleTimeout:    15 * time.Second,
				MaxHeaderBytes: 1 << 20, // 1MB
			}
			setupLog.Info("Starting pprof server on localhost:6060")
			if err := server.ListenAndServe(); err != nil {
				setupLog.Error(err, "failed to start pprof server")
			}
		}()
	}

	setupLog.Info("Starting Controller Manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "failed to start manager")
		os.Exit(1)
	}
}
