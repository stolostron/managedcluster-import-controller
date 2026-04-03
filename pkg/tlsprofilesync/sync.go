// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tlsprofilesync

import (
	"context"
	"fmt"
	"os"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// ConfigMapName is the name of the ConfigMap that holds the TLS profile configuration.
	// Upstream OCM components watch this ConfigMap to get TLS settings without
	// depending on OpenShift APIs.
	ConfigMapName = "ocm-tls-profile"
)

// Run starts the tls-profile-sync sidecar using the controller-runtime pattern.
// It watches the OpenShift APIServer CR for TLS profile changes and syncs the
// configuration to a ConfigMap in the sidecar's namespace. This allows upstream
// OCM components (klusterlet-operator) to consume TLS settings via standard
// Kubernetes APIs.
func Run(ctx context.Context) error {
	namespace := getNamespace()
	if namespace == "" {
		return fmt.Errorf("failed to determine namespace: set POD_NAMESPACE env or run in-cluster")
	}

	klog.Infof("Starting tls-profile-sync sidecar in namespace %q", namespace)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Detect if running on OpenShift by checking for the config.openshift.io API group
	_, err = kubeClient.Discovery().ServerResourcesForGroupVersion("config.openshift.io/v1")
	if err != nil {
		klog.Infof("config.openshift.io API not found, not an OpenShift cluster, "+
			"TLS profile sync is not needed, sleeping indefinitely: %v", err)
		<-ctx.Done()
		return nil
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(cfg, manager.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable metrics server for sidecar
		},
		LeaderElection:          true,
		LeaderElectionID:        "tls-profile-sync.open-cluster-management.io",
		LeaderElectionNamespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	reconciler := &tlsProfileSyncReconciler{
		client:     mgr.GetClient(),
		kubeClient: kubeClient,
		namespace:  namespace,
	}

	c, err := controller.New("tls-profile-sync", mgr, controller.Options{
		Reconciler: reconciler,
	})
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Watch the APIServer CR (cluster singleton)
	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		&configv1.APIServer{},
		&handler.TypedEnqueueRequestForObject[*configv1.APIServer]{},
	)); err != nil {
		return fmt.Errorf("failed to watch APIServer: %w", err)
	}

	// Watch the ocm-tls-profile ConfigMap so it gets recreated if accidentally deleted
	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		&corev1.ConfigMap{},
		handler.TypedEnqueueRequestsFromMapFunc(
			func(_ context.Context, cm *corev1.ConfigMap) []reconcile.Request {
				if cm.Name == ConfigMapName && cm.Namespace == namespace {
					return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "cluster"}}}
				}
				return nil
			},
		),
	)); err != nil {
		return fmt.Errorf("failed to watch ConfigMap: %w", err)
	}

	klog.Info("Starting manager for tls-profile-sync")
	return mgr.Start(ctx)
}

// tlsProfileSyncReconciler reconciles APIServer CR changes by syncing the TLS
// profile to a ConfigMap.
type tlsProfileSyncReconciler struct {
	client     client.Client
	kubeClient kubernetes.Interface
	namespace  string
}

func (r *tlsProfileSyncReconciler) Reconcile(
	ctx context.Context, req reconcile.Request,
) (reconcile.Result, error) {
	// Only care about the "cluster" singleton
	if req.Name != "cluster" {
		return reconcile.Result{}, nil
	}

	apiServer := &configv1.APIServer{}
	if err := r.client.Get(ctx, req.NamespacedName, apiServer); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Info("APIServer CR not found, skipping")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	data := buildConfigMapData(apiServer.Spec.TLSSecurityProfile)

	if err := r.syncConfigMap(ctx, data); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync ConfigMap: %w", err)
	}

	klog.Infof("Synced ConfigMap %s/%s: minTLSVersion=%s, profileType=%s",
		r.namespace, ConfigMapName,
		data["minTLSVersion"], data["profileType"])

	return reconcile.Result{}, nil
}

func (r *tlsProfileSyncReconciler) syncConfigMap(
	ctx context.Context, data map[string]string,
) error {
	cm, err := r.kubeClient.CoreV1().ConfigMaps(r.namespace).Get(
		ctx, ConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = r.kubeClient.CoreV1().ConfigMaps(r.namespace).Create(
			ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapName,
					Namespace: r.namespace,
				},
				Data: data,
			}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	cm.Data = data
	_, err = r.kubeClient.CoreV1().ConfigMaps(r.namespace).Update(
		ctx, cm, metav1.UpdateOptions{})
	return err
}

// buildConfigMapData converts an OpenShift TLSSecurityProfile into the ConfigMap
// data format expected by upstream OCM components. Cipher suites are converted
// from OpenSSL format to IANA format so upstream code doesn't need OpenSSL knowledge.
func buildConfigMapData(profile *configv1.TLSSecurityProfile) map[string]string {
	if profile == nil {
		// Default to Intermediate when no profile is set
		profile = &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileIntermediateType,
		}
	}

	var profileSpec *configv1.TLSProfileSpec
	var profileType string

	switch profile.Type {
	case configv1.TLSProfileOldType:
		profileSpec = configv1.TLSProfiles[configv1.TLSProfileOldType]
		profileType = string(configv1.TLSProfileOldType)
	case configv1.TLSProfileModernType:
		profileSpec = configv1.TLSProfiles[configv1.TLSProfileModernType]
		profileType = string(configv1.TLSProfileModernType)
	case configv1.TLSProfileCustomType:
		if profile.Custom != nil {
			spec := profile.Custom.TLSProfileSpec
			profileSpec = &spec
		} else {
			profileSpec = configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		}
		profileType = string(configv1.TLSProfileCustomType)
	default:
		// Intermediate is the default
		profileSpec = configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		profileType = string(configv1.TLSProfileIntermediateType)
	}

	minTLSVersion := string(profileSpec.MinTLSVersion)
	cipherSuites := libgocrypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers)

	return map[string]string{
		"minTLSVersion": minTLSVersion,
		"cipherSuites":  strings.Join(cipherSuites, ","),
		"profileType":   profileType,
	}
}

// getNamespace returns the namespace the sidecar is running in.
func getNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	// Fall back to in-cluster namespace detection
	if data, err := os.ReadFile(
		"/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}

	return ""
}
