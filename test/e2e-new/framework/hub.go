// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package framework

import (
	"fmt"
	"os"
	"os/user"
	"path"

	"github.com/onsi/ginkgo/v2"
	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	addonclient "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	klusterletconfigclient "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/clientset/versioned"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
)

var scheme = k8sruntime.NewScheme()

func init() {
	utilruntime.Must(k8sscheme.AddToScheme(scheme))
	utilruntime.Must(ocinfrav1.AddToScheme(scheme))
}

// Hub represents the hub cluster and provides all clients needed for e2e testing.
// In loopback mode (self-managed), hub and spoke are the same cluster.
type Hub struct {
	KubeClient          kubernetes.Interface
	DynamicClient       dynamic.Interface
	CRDClient           apiextensionsclient.Interface
	ClusterClient       clusterclient.Interface
	WorkClient          workclient.Interface
	OperatorClient      operatorclient.Interface
	AddonClient         addonclient.Interface
	KlusterletCfgClient klusterletconfigclient.Interface
	RuntimeClient       crclient.Client
	Recorder            events.Recorder
	Mapper              meta.RESTMapper
	RestConfig          *rest.Config
}

// NewHub creates a new Hub from a kubeconfig file path.
// If kubeconfigPath is empty, it uses KUBECONFIG env or ~/.kube/config.
func NewHub(kubeconfigPath string) (*Hub, error) {
	if kubeconfigPath == "" {
		var err error
		kubeconfigPath, err = GetKubeConfigFile()
		if err != nil {
			return nil, err
		}
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return NewHubFromConfig(cfg)
}

// NewHubFromConfig creates a new Hub from a rest.Config.
func NewHubFromConfig(cfg *rest.Config) (*Hub, error) {
	h := &Hub{RestConfig: cfg}
	var err error

	if h.KubeClient, err = kubernetes.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating kube client: %w", err)
	}
	if h.DynamicClient, err = dynamic.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}
	if h.CRDClient, err = apiextensionsclient.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating CRD client: %w", err)
	}
	if h.ClusterClient, err = clusterclient.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating cluster client: %w", err)
	}
	if h.WorkClient, err = workclient.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating work client: %w", err)
	}
	if h.OperatorClient, err = operatorclient.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating operator client: %w", err)
	}
	if h.AddonClient, err = addonclient.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating addon client: %w", err)
	}
	if h.KlusterletCfgClient, err = klusterletconfigclient.NewForConfig(cfg); err != nil {
		return nil, fmt.Errorf("creating klusterletconfig client: %w", err)
	}
	if h.RuntimeClient, err = crclient.New(cfg, crclient.Options{Scheme: scheme}); err != nil {
		return nil, fmt.Errorf("creating runtime client: %w", err)
	}

	h.Recorder = helpers.NewEventRecorder(h.KubeClient, "e2e-test")

	httpclient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP client: %w", err)
	}
	if h.Mapper, err = apiutil.NewDynamicRESTMapper(cfg, httpclient); err != nil {
		return nil, fmt.Errorf("creating REST mapper: %w", err)
	}

	return h, nil
}

// GetKubeConfigFile returns the kubeconfig file path from KUBECONFIG env or default location.
func GetKubeConfigFile() (string, error) {
	kubeConfigFile := os.Getenv("KUBECONFIG")
	if kubeConfigFile == "" {
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		kubeConfigFile = path.Join(u.HomeDir, ".kube", "config")
	}
	return kubeConfigFile, nil
}

// Logf writes debug output to GinkgoWriter.
func Logf(format string, args ...interface{}) {
	fmt.Fprintf(ginkgo.GinkgoWriter, "DEBUG: "+format+"\n", args...)
}
