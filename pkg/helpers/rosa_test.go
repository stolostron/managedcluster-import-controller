// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	clustersmgmttesting "github.com/openshift-online/ocm-sdk-go/testing"
)

func TestKubeConfig(t *testing.T) {
	gomega.RegisterTestingT(t)

	accessToken := clustersmgmttesting.MakeTokenString("Bearer", 5*time.Minute)
	refreshToken := clustersmgmttesting.MakeTokenString("Refresh", 10*time.Hour)

	oidServer := clustersmgmttesting.MakeTCPServer()
	oidServer.AppendHandlers(
		ghttp.CombineHandlers(
			clustersmgmttesting.RespondWithAccessAndRefreshTokens(accessToken, refreshToken),
		),
	)
	oauthServer := clustersmgmttesting.MakeTCPServer()
	apiServer := clustersmgmttesting.MakeTCPServer()
	defer func() {
		oidServer.Close()
		oauthServer.Close()
		apiServer.Close()
	}()

	cases := []struct {
		name           string
		clusterID      string
		handlers       []http.HandlerFunc
		expectedErrMsg string
	}{
		{
			name:      "cluster not found",
			clusterID: "0000",
			handlers: []http.HandlerFunc{
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/api/clusters_mgmt/v1/clusters/0000" {
							t.Fatalf("unexpected request %q", r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusNotFound, `{}`),
				),
			},
			expectedErrMsg: "status is 404",
		},
		{
			name:      "cluster api not found",
			clusterID: "0001",
			handlers: []http.HandlerFunc{
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/api/clusters_mgmt/v1/clusters/0001" {
							t.Fatalf("unexpected request %q", r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, `{}`),
				),
			},
			expectedErrMsg: "rosa cluster 0001 api url is not found",
		},
		{
			name:      "request a token",
			clusterID: "0002",
			handlers: []http.HandlerFunc{
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0002" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newRosaCluster(oauthServer.URL())),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0002/identity_providers" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0002/identity_providers" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusCreated, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0002/groups/cluster-admins/users/acm-import" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusNotFound, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0002/groups/cluster-admins/users" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusCreated, "{}"),
				),
			},
			expectedErrMsg: "kubeconfig for rosa cluster 0002 is not ready, retry after 30 seconds",
		},
		{
			name:      "there is only a htpasswd provider id",
			clusterID: "0003",
			handlers: []http.HandlerFunc{
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0003" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newRosaCluster(oauthServer.URL())),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0003/identity_providers" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newIdentityProviderList()),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0003/identity_providers/1234/htpasswd_users" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0003/identity_providers/1234/htpasswd_users" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusCreated, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0003/groups/cluster-admins/users/acm-import" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusNotFound, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0003/groups/cluster-admins/users" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusCreated, "{}"),
				),
			},
			expectedErrMsg: "kubeconfig for rosa cluster 0003 is not ready, retry after 30 seconds",
		},
		{
			name:      "there is an existed htpasswd user",
			clusterID: "0004",
			handlers: []http.HandlerFunc{
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0004" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newRosaCluster(oauthServer.URL())),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0004/identity_providers" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newIdentityProviderList()),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0004/identity_providers/1234/htpasswd_users" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newHTPasswdUserList()),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPatch || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0004/identity_providers/1234/htpasswd_users/4567" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0004/groups/cluster-admins/users/acm-import" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusNotFound, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0004/groups/cluster-admins/users" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusCreated, "{}"),
				),
			},
			expectedErrMsg: "kubeconfig for rosa cluster 0004 is not ready, retry after 30 seconds",
		},
		{
			name:      "the user is already in the admin group",
			clusterID: "0005",
			handlers: []http.HandlerFunc{
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0005" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newRosaCluster(oauthServer.URL())),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0005/identity_providers" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0005/identity_providers" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusCreated, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/0005/groups/cluster-admins/users/acm-import" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
			},
			expectedErrMsg: "kubeconfig for rosa cluster 0005 is not ready, retry after 30 seconds",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeConfigGetter := NewRosaKubeConfigGetter()
			kubeConfigGetter.SetAPIServerURL(apiServer.URL())
			kubeConfigGetter.SetTokenURL(oidServer.URL())
			kubeConfigGetter.SetToken(accessToken)
			kubeConfigGetter.SetClusterID(c.clusterID)

			oauthServer.AppendHandlers(
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// just ensure the token request is received
						w.WriteHeader(http.StatusNotImplemented)
					}),
				),
			)

			apiServer.AppendHandlers(c.handlers...)

			_, _, err := kubeConfigGetter.KubeConfig()
			if len(c.expectedErrMsg) == 0 {
				if err != nil {
					t.Errorf("unexected error %v", err)
				}

				return
			}

			if err == nil {
				t.Errorf("exected error %q, but no error", c.expectedErrMsg)
				return
			}

			if err.Error() != c.expectedErrMsg {
				t.Errorf("exected error %q, but get %v", c.expectedErrMsg, err)
			}
		})
	}
}

func TestStopRetry(t *testing.T) {
	gomega.RegisterTestingT(t)

	accessToken := clustersmgmttesting.MakeTokenString("Bearer", 5*time.Minute)
	refreshToken := clustersmgmttesting.MakeTokenString("Refresh", 10*time.Hour)

	oidServer := clustersmgmttesting.MakeTCPServer()
	oidServer.AppendHandlers(
		ghttp.CombineHandlers(
			clustersmgmttesting.RespondWithAccessAndRefreshTokens(accessToken, refreshToken),
		),
	)
	oauthServer := clustersmgmttesting.MakeTCPServer()
	oauthServer.AppendHandlers(
		ghttp.CombineHandlers(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// just ensure the token request is received
				w.WriteHeader(http.StatusNotImplemented)
			}),
		),
	)
	apiServer := clustersmgmttesting.MakeTCPServer()
	defer func() {
		oidServer.Close()
		oauthServer.Close()
		apiServer.Close()
	}()

	cases := []struct {
		name      string
		clusterID string
		handlers  []http.HandlerFunc
	}{
		{
			name:      "stop retry",
			clusterID: "test",
			handlers: []http.HandlerFunc{
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/test" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newRosaCluster(oauthServer.URL())),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/test/identity_providers" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, newIdentityProviderList()),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodDelete || r.URL.Path != "/api/clusters_mgmt/v1/clusters/test/identity_providers/1234" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/test/groups/cluster-admins/users/acm-import" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
				ghttp.CombineHandlers(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodDelete || r.URL.Path != "/api/clusters_mgmt/v1/clusters/test/groups/cluster-admins/users/acm-import" {
							t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
						}
					}),
					clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
				),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getter := &RosaKubeConfigGetter{
				apiServerURL:      apiServer.URL(),
				tokenURL:          oidServer.URL(),
				totalRetryTimes:   defaultRetryTimes,
				clusterID:         c.clusterID,
				currentRetryTimes: 20,
				importUserPasswd:  "1234",
			}
			apiServer.AppendHandlers(c.handlers...)

			if retry, _, _ := getter.KubeConfig(); retry {
				t.Errorf("expect stop to retry, but failed")
			}
		})
	}
}

func newRosaCluster(url string) string {
	return fmt.Sprintf("{\"kind\":\"Cluster\",\"api\":{\"url\": \"%s\"}}", url)
}

func newIdentityProviderList() string {
	return `
{
	"kind": "IdentityProviderList",
	"items": [
		{
			"kind": "IdentityProvider",
			"type": "HTPasswdIdentityProvider",
			"id": "1234",
			"name": "acm-import"
		}
	]
}
`
}

func newHTPasswdUserList() string {
	return `
{
	"kind": "IdentityProviderList",
	"items": [
		{
			"kind": "HTPasswdUser",
			"id": "4567",
			"username": "acm-import"
		}
	]
}
`
}
