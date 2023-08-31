package agentregistration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"

	listerklusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/listers/klusterletconfig/v1alpha1"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func RunAgentRegistrationServer(ctx context.Context, port int, clientHolder *helpers.ClientHolder,
	klusterletconfigLister listerklusterletconfigv1alpha1.KlusterletConfigLister) error {
	mux := http.NewServeMux()

	mux.Handle("/agent-registration/crds/v1", authMiddleware(clientHolder, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := bootstrap.GenerateKlusterletCRDsV1()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		_, err = w.Write(content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})))

	mux.Handle("/agent-registration/crds/v1beta1", authMiddleware(clientHolder, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := bootstrap.GenerateKlusterletCRDsV1Beta1()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		_, err = w.Write(content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})))

	// example URl: https://<route address>/agent-registration/manifests/cluster1?klusterletconfig=default
	mux.Handle("/agent-registration/manifests/", authMiddleware(clientHolder, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		urlparams := strings.Split(r.URL.Path, "/")
		clusterID := urlparams[len(urlparams)-1]

		// Get KlusterletConfig
		var kc *klusterletconfigv1alpha1.KlusterletConfig
		klusterletconfigName := r.URL.Query().Get("klusterletconfig")
		if klusterletconfigName != "" {
			kc, err = klusterletconfigLister.Get(klusterletconfigName)
			if err != nil && !apierrors.IsNotFound(err) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// In the agent-registration case, the bootstrap sa is not created in the managed cluster namespace, because managed cluster is not created yet.
		// Instead, it's in the pod namespace with the name "agent-registration-bootstrap".
		bootstrapkubeconfig, _, err := bootstrap.CreateBootstrapKubeConfig(ctx, clientHolder, AgentRegistrationDefaultBootstrapSAName,
			os.Getenv(constants.PodNamespaceEnvVarName),
			7*24*3600)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		klusterletClusterAnnotations := map[string]string{
			"agent.open-cluster-management.io/create-with-default-klusterletaddonconfig": "true",
		}
		if kc != nil {
			// This annotation will finanlly be added on the managedcluster which created by the agent side.
			// Then the reconciliation of importconfig-controller will render manifests with the same KlusterletConfig
			klusterletClusterAnnotations[apiconstants.AnnotationKlusterletConfig] = klusterletconfigName
		}

		content, err := bootstrap.NewKlusterletManifestsConfig(
			operatorv1.InstallModeDefault,
			clusterID,
			DefaultKlusterletNamespace,
			bootstrapkubeconfig).
			WithKlusterletClusterAnnotations(klusterletClusterAnnotations).
			WithKlusterletConfig(kc).
			Generate(r.Context(), clientHolder)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		_, err = w.Write(content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})))

	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Addr:              fmt.Sprintf(":%d", port),
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		Handler:           mux,
	}

	klog.Infof("Starting AgentRegistrationServer on port %d", port)
	return server.ListenAndServeTLS("/server/tls.crt", "/server/tls.key")
}

func authMiddleware(clientHolder *helpers.ClientHolder, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the Authorization header value
		authHeader := r.Header.Get("Authorization")

		// Check if the header value starts with "Bearer "
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
			return
		}

		// Extract the token from the header value
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Authentication
		trresult, err := clientHolder.KubeClient.AuthenticationV1().TokenReviews().Create(r.Context(), &authenticationv1.TokenReview{
			Spec: authenticationv1.TokenReviewSpec{
				Token: token,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			http.Error(w, fmt.Sprintf("create TR failed %v", err.Error()), http.StatusInternalServerError)
			return
		}
		if !trresult.Status.Authenticated {
			http.Error(w, fmt.Sprintf("authentication failed, response:%v, error:%v", trresult.Status, trresult.Status.Error), http.StatusUnauthorized)
			return
		}

		// Authorization
		userInfo := trresult.Status.User
		extra := make(map[string]authorizationv1.ExtraValue)
		for k, v := range userInfo.Extra {
			extra[k] = authorizationv1.ExtraValue(v)
		}
		sarrequest := &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User:   userInfo.Username,
				Groups: userInfo.Groups,
				UID:    userInfo.UID,
				Extra:  extra,
				NonResourceAttributes: &authorizationv1.NonResourceAttributes{
					Path: "/agent-registration/*",
					Verb: "get",
				},
			},
		}
		sarresult, err := clientHolder.KubeClient.AuthorizationV1().SubjectAccessReviews().Create(r.Context(), sarrequest, metav1.CreateOptions{})
		if err != nil {
			http.Error(w, fmt.Sprintf("create SAR failed %v, user: %v", err.Error(), userInfo), http.StatusInternalServerError)
			return
		}
		if !sarresult.Status.Allowed {
			http.Error(w, fmt.Sprintf("authorization failed, response:%v, user:%v", sarresult.Status, userInfo), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

const (
	AgentRegistrationDefaultBootstrapSAName = "agent-registration-bootstrap"
	DefaultKlusterletNamespace              = "open-cluster-management-agent"
)
