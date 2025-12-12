package flightctl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	flightctlclient "github.com/flightctl/flightctl/lib/api/client"
	flightctlapiv1 "github.com/flightctl/flightctl/lib/apipublic/v1alpha1"
	flightctlcli "github.com/flightctl/flightctl/lib/cli"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
)

const (
	FlightCtlDiscoveryConfigMap = "flightctl-discovery"
)

//go:embed manifests
var FlightCtlManifestFiles embed.FS

// flightctl-agent-registration <-> flightctl-agent-registration: used in the flightctl Repository,
// will be delivered to the flightctl-agent in managed cluster side.
var files = []string{
	"manifests/clusterrolebinding_agentregistration.yml",
	"manifests/serviceaccount.yml",
}

func NewFlightCtlManager(clientHolder *helpers.ClientHolder,
	clusterIngressDomain string) *FlightCtlManager {
	fcm := &FlightCtlManager{
		agentRegistrationServer: "https://agent-registration-multicluster-engine." + clusterIngressDomain,
		clientHolder:            clientHolder,
		recorder:                helpers.NewEventRecorder(clientHolder.KubeClient, "FlightCtl"),
	}
	return fcm
}

type FlightCtlManager struct {
	clientHolder *helpers.ClientHolder
	recorder     events.Recorder

	flightctlClient flightctlClient
	flightctlServer string

	agentRegistrationServer string
}

// StartReconcileFlightCtlResources starts a loop to reconcile FlightCtl resources
// 1. ensure the flightctl-api service is running on the hub side.
// 2. apply the kubernetes resources.
// 3. apply the Repository resources.
// 4. keep reconcile the Repository resource every day to keep agent registration token fresh.
func (f *FlightCtlManager) StartReconcileFlightCtlResources(ctx context.Context) {
	// Helper function to apply resources and record errors
	applyFunc := func(ctx context.Context) (bool, error) {
		// First, check if flightctl is enabled and healthy, if not, skip the reconciliation.
		if err := f.isFlightCtlEnabledAndHealthy(); err != nil {
			f.recorder.Event("FlightCtlDegraded",
				fmt.Sprintf("FlightCtl is not enabled or healthy, skipping resource reconciliation: %v", err))
			return false, nil
		}

		if err := f.ensureFlightCtlServer(); err != nil {
			f.recorder.Event("FlightCtlServerFailed",
				fmt.Sprintf("Failed to ensure FlightCtl server: %v", err))
			return false, nil
		}

		// Apply kubernetes resources
		if err := f.applyKuberentesResources(ctx); err != nil {
			f.recorder.Event("KubernetesResourcesFailed",
				fmt.Sprintf("Failed to apply Kubernetes resources: %v", err))
			return false, nil
		}

		// Create Repository resources
		if err := f.applyRepository(ctx); err != nil {
			f.recorder.Event("RepositoryFailed",
				fmt.Sprintf("Failed to apply Repository resources: %v", err))
			return false, nil
		}

		// Record successful sync
		f.recorder.Event("ResourcesSynced", "Successfully synced FlightCtl resources")
		return true, nil
	}

	// Poll every 5 minutes until success
	if err := wait.PollUntilContextCancel(ctx, 5*time.Minute, true, applyFunc); err != nil {
		f.recorder.Event("ResourcesSynced", "Failed to sync FlightCtl resources")
	}

	// Keep reconcile the Repository resource every day to keep agent registration token fresh.
	wait.Until(func() {
		if err := f.applyRepository(ctx); err != nil {
			f.recorder.Event("RepositoryFailed", fmt.Sprintf("Failed to reconcile Repository resources: %v", err))
		}
	}, 24*time.Hour, ctx.Done())
}

// ensureFlightCtlServer sets the flightctl server address if not already set.
// It gets the apiEndpoint from the flightctl-discovery ConfigMap.
func (f *FlightCtlManager) ensureFlightCtlServer() error {
	if f.flightctlServer != "" {
		return nil
	}

	namespace := os.Getenv("POD_NAMESPACE")
	cm, err := f.clientHolder.KubeClient.CoreV1().ConfigMaps(namespace).Get(
		context.Background(),
		FlightCtlDiscoveryConfigMap,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to get %s configmap: %v", FlightCtlDiscoveryConfigMap, err)
	}

	apiEndpoint, ok := cm.Data["apiEndpoint"]
	if !ok || apiEndpoint == "" {
		return fmt.Errorf("apiEndpoint not found or empty in %s configmap", FlightCtlDiscoveryConfigMap)
	}

	f.flightctlServer = apiEndpoint
	f.flightctlClient = &flightctlClientImpl{flightctlServer: f.flightctlServer}
	return nil
}

func (f *FlightCtlManager) applyKuberentesResources(_ context.Context) error {
	var err error

	// Create rbac resources and set owner reference to the ns.
	objects, err := helpers.FilesToObjects(files, struct {
		Namespace string
	}{
		Namespace: os.Getenv("POD_NAMESPACE"),
	}, &FlightCtlManifestFiles)
	if err != nil {
		return err
	}
	if _, err := helpers.ApplyResources(
		f.clientHolder, f.recorder, nil, nil, objects...); err != nil {
		return err
	}

	return nil
}

func (f *FlightCtlManager) applyRepository(ctx context.Context) error {
	flightctlClientToken, err := f.getFlightCtlClientToken()
	if err != nil {
		return err
	}

	agentRegistrationToken, err := f.getFlightCtlAgentRegistrationServiceAccountToken(ctx)
	if err != nil {
		return err
	}

	ca, err := f.getAgentRegistrationCA()
	if err != nil {
		return err
	}

	expectedRepository := &flightctlapiv1.Repository{
		ApiVersion: "v1alpha1",
		Kind:       "Repository",
		Metadata: flightctlapiv1.ObjectMeta{
			// Note: In the fleets' `httpRef.repository` field, the name is `acm-registration`.
			// See details in: https://github.com/flightctl/flightctl/blob/main/docs/user/registering-microshift-devices-acm.md
			Name: ptr.To("acm-registration"),
		},
		Spec: flightctlapiv1.RepositorySpec{},
	}
	err = expectedRepository.Spec.MergeHttpRepoSpec(flightctlapiv1.HttpRepoSpec{
		Type: flightctlapiv1.Http,
		Url:  f.agentRegistrationServer,
		HttpConfig: flightctlapiv1.HttpConfig{
			Token: &agentRegistrationToken,
			CaCrt: &ca,
		},
		ValidationSuffix: ptr.To("/agent-registration"),
	})
	if err != nil {
		return err
	}

	return f.flightctlClient.ApplyRepository(ctx, flightctlClientToken, expectedRepository)
}

func (f *FlightCtlManager) IsManagedClusterAFlightctlDevice(ctx context.Context, managedClusterName string) (bool, error) {
	// First, check if flightctl is enabled and healthy.
	if err := f.isFlightCtlEnabledAndHealthy(); err != nil {
		return false, nil
	}

	flightctlClientToken, err := f.getFlightCtlClientToken()
	if err != nil {
		return false, err
	}

	response, err := f.flightctlClient.GetDevice(ctx, flightctlClientToken, managedClusterName)
	if err != nil {
		return false, err
	}

	if response.HTTPResponse.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if response.HTTPResponse.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to get device %s, status code: %d", managedClusterName, response.HTTPResponse.StatusCode)
	}

	return true, nil
}

// isFlightCtlEnabledAndHealthy checks if FlightCtl is enabled and healthy by:
// 1. Verifying the existence of the flightctl-discovery ConfigMap in the current namespace.
// 2. Performing a health check against the healthEndpoint specified in the ConfigMap.
// Returns nil if FlightCtl is enabled and healthy, otherwise returns an error.
func (f *FlightCtlManager) isFlightCtlEnabledAndHealthy() error {
	namespace := os.Getenv("POD_NAMESPACE")
	cm, err := f.clientHolder.KubeClient.CoreV1().ConfigMaps(namespace).Get(
		context.Background(),
		FlightCtlDiscoveryConfigMap,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("flightctl is not enabled: %s configmap not found", FlightCtlDiscoveryConfigMap)
		}
		return fmt.Errorf("failed to get flightctl-discovery configmap: %v", err)
	}

	// Get the health endpoint from the ConfigMap
	healthEndpoint, ok := cm.Data["healthEndpoint"]
	if !ok || healthEndpoint == "" {
		return fmt.Errorf("healthEndpoint not found in flightctl-discovery configmap")
	}

	// Get the flightctl namespace from the ConfigMap
	flightctlNamespace, ok := cm.Data["namespace"]
	if !ok || flightctlNamespace == "" {
		return fmt.Errorf("namespace not found in flightctl-discovery configmap")
	}

	// Get the CA certificate from flightctl-ca-bundle Secret
	caSecret, err := f.clientHolder.KubeClient.CoreV1().Secrets(flightctlNamespace).Get(
		context.Background(),
		"flightctl-ca-bundle",
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to get flightctl-ca-bundle secret: %v", err)
	}

	caCert, ok := caSecret.Data["ca.crt"]
	if !ok {
		return fmt.Errorf("ca.crt not found in flightctl-ca-bundle secret")
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return fmt.Errorf("failed to parse CA certificate from flightctl-ca-bundle")
	}

	// Perform health check with custom CA
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}
	resp, err := client.Get(healthEndpoint)
	if err != nil {
		return fmt.Errorf("failed to perform health check: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("flightctl health check failed with status code: %d", resp.StatusCode)
	}

	return nil
}

// getFlightCtlAgentRegistrationServiceAccountToken creates a token for the flightctl-agent-registration service account.
// The token duration is set to 10 days to prevent flightctl-agent from holding a long-term credential.
func (f *FlightCtlManager) getFlightCtlAgentRegistrationServiceAccountToken(ctx context.Context) (string, error) {
	// Create token request for the service account
	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: ptr.To[int64](10 * 24 * 60 * 60), // Give 10 days for the token to expire
		},
	}

	// Get the token using TokenRequest API
	tokenResponse, err := f.clientHolder.KubeClient.CoreV1().ServiceAccounts(os.Getenv("POD_NAMESPACE")).
		CreateToken(ctx, "flightctl-agent-registration", tokenRequest, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create token: %v", err)
	}

	return tokenResponse.Status.Token, nil
}

// The token is mounted from the service account in the pod.
func (f *FlightCtlManager) getFlightCtlClientToken() (string, error) {
	tokenData, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return "", fmt.Errorf("failed to read service account token: %v", err)
	}
	return string(tokenData), nil
}

// TODO: @xuezhaojun need to consider cases like: https proxy in the middle, route using system CA cert instead of OCP self-signed cert, etc.
// Note: the CA cert will also rotate, but the expiration time is long enough.
func (f *FlightCtlManager) getAgentRegistrationCA() (string, error) {
	caData, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return "", fmt.Errorf("failed to read service account CA: %v", err)
	}

	return base64.StdEncoding.EncodeToString(caData), nil
}

type flightctlClient interface {
	ApplyRepository(ctx context.Context, token string, expectedRepository *flightctlapiv1.Repository) error
	GetDevice(ctx context.Context, token string, managedClusterName string) (*flightctlclient.ReadDeviceResponse, error)
}

var _ flightctlClient = &flightctlClientImpl{}

type flightctlClientImpl struct {
	flightctlServer string
}

func (f *flightctlClientImpl) ApplyRepository(ctx context.Context, token string, expectedRepository *flightctlapiv1.Repository) error {
	return flightctlcli.ApplyRepository(ctx, token, f.flightctlServer, expectedRepository)
}

func (f *flightctlClientImpl) GetDevice(ctx context.Context, token string, managedClusterName string) (*flightctlclient.ReadDeviceResponse, error) {
	return flightctlcli.GetDevice(ctx, token, f.flightctlServer, managedClusterName)
}
