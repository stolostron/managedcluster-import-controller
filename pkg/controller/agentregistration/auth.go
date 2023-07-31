package agentregistration

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/authenticatorfactory"
	"k8s.io/apiserver/pkg/authentication/group"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/request/websocket"
	"k8s.io/apiserver/pkg/authentication/request/x509"
	"k8s.io/apiserver/pkg/authentication/token/cache"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	webhooktoken "k8s.io/apiserver/plugin/pkg/authenticator/token/webhook"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	authenticationConfigMapNamespace = metav1.NamespaceSystem
	// authenticationConfigMapName is the name of ConfigMap in the kube-system namespace holding the root certificate
	// bundle to use to verify client certificates on incoming requests before trusting usernames in headers specified
	// by --requestheader-username-headers. This is created in the cluster by the kube-apiserver.
	// "WARNING: generally do not depend on authorization being already done for incoming requests.")
	authenticationConfigMapName = "extension-apiserver-authentication"
)

type DelegatingAuthenticatorConfig struct {
	// TokenAccessReviewTimeout specifies a time limit for requests made by the authorization webhook client.
	TokenAccessReviewTimeout time.Duration

	// WebhookRetryBackoff specifies the backoff parameters for the authentication webhook retry logic.
	// This allows us to configure the sleep time at each iteration and the maximum number of retries allowed
	// before we fail the webhook call in order to limit the fan out that ensues when the system is degraded.
	WebhookRetryBackoff *wait.Backoff

	// CacheTTL is the length of time that a token authentication answer will be cached.
	CacheTTL time.Duration

	APIAudiences authenticator.Audiences

	clientCA dynamiccertificates.ControllerRunner
}

func (d *DelegatingAuthenticatorConfig) New() (authenticator.Request, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Warningf("Current runtime environment is not in a cluster, ignore --delegating-authentication flag.")
		return nil, nil
	}

	config.QPS = 200
	config.Burst = 400

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get delegated authentication kubeconfig: %v", err)
	}

	clientCAProvider, err := dynamiccertificates.NewDynamicCAFromConfigMapController(
		"client-ca",
		authenticationConfigMapNamespace,
		authenticationConfigMapName,
		"client-ca-file",
		client,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load configmap based client CA file: %v", err)
	}

	d.clientCA = clientCAProvider

	tokenAuth, err := webhooktoken.NewFromInterface(
		client.AuthenticationV1(),
		d.APIAudiences,
		*d.WebhookRetryBackoff,
		d.TokenAccessReviewTimeout,
		webhooktoken.AuthenticatorMetrics{
			RecordRequestTotal:   authenticatorfactory.RecordRequestTotal,
			RecordRequestLatency: authenticatorfactory.RecordRequestLatency,
		})
	if err != nil {
		return nil, err
	}

	cachingTokenAuth := cache.New(tokenAuth, false, d.CacheTTL, d.CacheTTL)

	return group.NewAuthenticatedGroupAdder(union.New(
		x509.NewDynamic(clientCAProvider.VerifyOptions, x509.CommonNameUserConversion),
		bearertoken.New(cachingTokenAuth),
		websocket.NewProtocolAuthenticator(cachingTokenAuth),
	)), nil
}

func (d *DelegatingAuthenticatorConfig) Start(ctx context.Context) {
	if d.clientCA == nil {
		return
	}

	if err := d.clientCA.RunOnce(ctx); err != nil {
		runtime.HandleError(err)
	}

	go d.clientCA.Run(ctx, 1)
}
