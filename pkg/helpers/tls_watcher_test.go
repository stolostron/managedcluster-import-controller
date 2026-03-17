// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"net/http"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

func TestSetupTLSProfileWatcher_VanillaKubernetes(t *testing.T) {
	// Save original DeployOnOCP value and restore after test
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	// Simulate vanilla Kubernetes
	DeployOnOCP = false

	scheme := runtime.NewScheme()
	_ = configv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := &fakeManager{client: fakeClient}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should return nil on vanilla Kubernetes
	err := SetupTLSProfileWatcher(ctx, mgr)
	if err != nil {
		t.Errorf("SetupTLSProfileWatcher() on vanilla Kubernetes should return nil, got error: %v", err)
	}
}

func TestSetupTLSProfileWatcher_OpenShift_NoAPIServer(t *testing.T) {
	// Save original DeployOnOCP value and restore after test
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	// Simulate OpenShift
	DeployOnOCP = true

	scheme := runtime.NewScheme()
	_ = configv1.AddToScheme(scheme)

	// No APIServer object created
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := &fakeManager{client: fakeClient}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should return error when APIServer object doesn't exist
	err := SetupTLSProfileWatcher(ctx, mgr)
	if err == nil {
		t.Error("SetupTLSProfileWatcher() should return error when APIServer object doesn't exist")
	}
}

func TestSetupTLSProfileWatcher_OpenShift_WithAPIServer(t *testing.T) {
	// Save original DeployOnOCP value and restore after test
	originalDeployOnOCP := DeployOnOCP
	defer func() { DeployOnOCP = originalDeployOnOCP }()

	// Simulate OpenShift
	DeployOnOCP = true

	scheme := runtime.NewScheme()
	_ = configv1.AddToScheme(scheme)

	// Create APIServer with Intermediate TLS profile
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(apiServer).
		Build()

	mgr := &fakeManager{
		client: fakeClient,
		scheme: scheme,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should succeed when APIServer exists and scheme is provided
	err := SetupTLSProfileWatcher(ctx, mgr)
	if err != nil {
		t.Errorf("SetupTLSProfileWatcher() should succeed on OpenShift with APIServer, got error: %v", err)
	}
}

// fakeManager is a minimal implementation of manager.Manager for testing
type fakeManager struct {
	client client.Client
	scheme *runtime.Scheme
}

func (f *fakeManager) GetClient() client.Client {
	return f.client
}

func (f *fakeManager) GetScheme() *runtime.Scheme {
	if f.scheme != nil {
		return f.scheme
	}
	return runtime.NewScheme()
}
func (f *fakeManager) GetConfig() *rest.Config                              { return nil }
func (f *fakeManager) GetCache() cache.Cache                                { return nil }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer                 { return nil }
func (f *fakeManager) GetEventRecorder(name string) events.EventRecorder    { return nil }
func (f *fakeManager) GetEventRecorderFor(name string) record.EventRecorder { return nil }
func (f *fakeManager) GetRESTMapper() meta.RESTMapper                       { return nil }
func (f *fakeManager) GetAPIReader() client.Reader                 { return nil }
func (f *fakeManager) Start(ctx context.Context) error             { return nil }
func (f *fakeManager) Add(manager.Runnable) error                  { return nil }
func (f *fakeManager) Elected() <-chan struct{}                    { return nil }
func (f *fakeManager) AddMetricsServerExtraHandler(path string, handler http.Handler) error {
	return nil
}
func (f *fakeManager) AddHealthzCheck(name string, check healthz.Checker) error { return nil }
func (f *fakeManager) AddReadyzCheck(name string, check healthz.Checker) error  { return nil }
func (f *fakeManager) GetWebhookServer() webhook.Server           { return nil }
func (f *fakeManager) GetLogger() logr.Logger                      { return logr.Discard() }
func (f *fakeManager) GetControllerOptions() config.Controller {
	return config.Controller{}
}
func (f *fakeManager) GetHTTPClient() *http.Client              { return nil }
func (f *fakeManager) GetConverterRegistry() conversion.Registry { return nil }
func (f *fakeManager) GetResourceGroupIdentifier(schema.GroupResource) string {
	return ""
}
