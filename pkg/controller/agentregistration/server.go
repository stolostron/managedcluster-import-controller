package agentregistration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/controller/importconfig"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/klog/v2"
)

func NewAgentRegistrationServer(port int, clientHolder *helpers.ClientHolder) *AgentRegistrationServer {
	return &AgentRegistrationServer{
		port:         port,
		clientHolder: clientHolder,
	}
}

// AgentRegistrationServer returns a bundle of manifests to the agent to apply for the registration process.
type AgentRegistrationServer struct {
	clientHolder *helpers.ClientHolder
	port         int
	auth         authenticator.Request
}

func (s *AgentRegistrationServer) Run(ctx context.Context) error {
	var err error

	if s.port == 0 {
		return fmt.Errorf("port is not set")
	}

	// client auth
	authConfig := &DelegatingAuthenticatorConfig{
		CacheTTL:                 10 * time.Second,
		WebhookRetryBackoff:      genericoptions.DefaultAuthWebhookRetryBackoff(),
		TokenAccessReviewTimeout: 10 * time.Second,
	}
	s.auth, err = authConfig.New()
	if err != nil {
		return err
	}
	authConfig.Start(ctx)

	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           s,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}

	klog.Infof("Starting AgentRegistrationServer on port %d", s.port)
	return server.ListenAndServeTLS("/server/tls.crt", "/server/tls.key")
}

func (s *AgentRegistrationServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	var content []byte

	klog.Infof("AgentRegistrationServer request: %s %s", r.Method, r.RequestURI)

	// Authenticate the request
	authResponse, pass, err := s.auth.AuthenticateRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !pass {
		http.Error(w, fmt.Sprintf("authentication failed, response:%v", authResponse), http.StatusUnauthorized)
		return
	}

	// Authorize the request
	userInfo := authResponse.User
	extra := make(map[string]authorizationv1.ExtraValue)
	for k, v := range userInfo.GetExtra() {
		extra[k] = authorizationv1.ExtraValue(v)
	}
	sarrequest := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   userInfo.GetName(),
			Groups: userInfo.GetGroups(),
			UID:    userInfo.GetUID(),
			Extra:  extra,
			NonResourceAttributes: &authorizationv1.NonResourceAttributes{
				Path: "/agent-registration/*",
				Verb: "get",
			},
		},
	}
	sarresult, err := s.clientHolder.KubeClient.AuthorizationV1().SubjectAccessReviews().Create(r.Context(), sarrequest, metav1.CreateOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("create SAR failed %v, user: %v", err.Error(), userInfo), http.StatusInternalServerError)
		return
	}
	if !sarresult.Status.Allowed {
		http.Error(w, fmt.Sprintf("authorization failed, response:%v, user:%v", sarresult.Status, userInfo), http.StatusUnauthorized)
		return
	}

	// Parse AgentRegistrationRequest from the request body
	urlparams := strings.Split(r.RequestURI, "/")
	if len(urlparams) != 3 {
		http.Error(w, "invalid request URI", http.StatusBadRequest)
		return
	}

	switch urlparams[1] {
	case "manifests":
		clusterID := urlparams[2]
		content, err = importconfig.GenerateAgentRegistrationManifests(r.Context(), s.clientHolder, clusterID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	case "crds":
		content, err = importconfig.GenerateKlusterletCRDs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}

	_, err = w.Write(content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
