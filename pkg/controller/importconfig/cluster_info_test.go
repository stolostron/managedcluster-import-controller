// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"reflect"
	"testing"
	"time"

	ocinfrav1 "github.com/openshift/api/config/v1"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/ocm/pkg/operator/helpers/chart"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildBootstrapKubeconfigData(t *testing.T) {
	certData1, _, _ := testinghelpers.NewRootCA("test ca1")
	certData2, keyData2, _ := testinghelpers.NewRootCA("test ca2")

	testInfraConfigDNS := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
			UID:  "default-cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "https://my-dns-name.com:6443",
		},
	}
	testInfraConfigWithoutUID := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "https://my-dns-name.com:6443",
		},
	}

	apiserverConfig := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}

	apiserverConfigWithCustomCA := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{
			ServingCerts: ocinfrav1.APIServerServingCerts{
				NamedCertificates: []ocinfrav1.APIServerNamedServingCert{
					{
						Names:              []string{"my-dns-name.com"},
						ServingCertificate: ocinfrav1.SecretNameReference{Name: "test-secret"},
					},
				},
			},
		},
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-root-ca.crt",
			Namespace: "testcluster",
		},
		Data: map[string]string{
			"ca.crt": string(certData1),
		},
	}

	secretCorrect := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			"tls.crt": certData2,
			"tls.key": keyData2,
		},
		Type: corev1.SecretTypeTLS,
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-bootstrap-sa",
			Namespace: "testcluster",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name:      "testcluster-bootstrap-sa-token-5pw5c",
				Namespace: "testcluster",
			},
		},
	}

	saSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-bootstrap-sa-token-5pw5c",
			Namespace: "testcluster",
		},
		Data: map[string][]byte{
			"token": []byte("sa-token"),
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	type wantData struct {
		serverURL string
		ca        string
		certData  []byte
		token     string
	}
	tests := []struct {
		name             string
		clientObjs       []client.Object
		runtimeObjs      []runtime.Object
		selfManaged      bool
		klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
		want             *wantData
		wantErr          bool
	}{
		{
			name:        "import secret not exist",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{cm, sa, saSecret},
			wantErr:     false,
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData1,
				token:     "sa-token",
			},
		},
		{
			name:        "import secret don't have the desired content",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{cm, mockImportSecretWithoutContent()},
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData1,
				token:     "fake-token",
			},
			wantErr: false,
		},
		{
			name:       "token is expired",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfigWithCustomCA},
			runtimeObjs: []runtime.Object{cm, secretCorrect,
				mockImportSecret(t, time.Now().Add(-1*time.Hour),
					"https://my-dns-name.com:6443",
					certData2,
					"mock-token"),
			},
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData2,
				token:     "fake-token",
			},
			wantErr: false,
		},
		{
			name:       "caData not validate",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfigWithCustomCA},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					[]byte("wrong"),
					"mock-token"),
			},
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData2,
				token:     "mock-token",
			},
			wantErr: false,
		},
		{
			name:       "kubeAPIServer not validate",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfigWithCustomCA},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://wrong.com:6443",
					certData2,
					"mock-token"),
			},
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData2,
				token:     "mock-token",
			},
			wantErr: false,
		},
		{
			name:       "kubeconfig current context cluster name not match",
			clientObjs: []client.Object{testInfraConfigWithoutUID, apiserverConfigWithCustomCA},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					certData2,
					"mock-token"),
			},
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData2,
				token:     "mock-token",
			},
			wantErr: false,
		},
		{
			name:       "all fileds are valid",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfigWithCustomCA},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					certData2,
					"mock-token"),
			},
			wantErr: false,
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData2,
				token:     "mock-token",
			},
		},
		{
			name:       "all fileds are valid, failed to get the ca from ocp, fallback to the kube-root-ca.crt configmap from the pod namespace.",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfigWithCustomCA},
			runtimeObjs: []runtime.Object{cm,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					certData1,
					"mock-token"),
			},
			wantErr: false,
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData1,
				token:     "mock-token",
			},
		},
		{
			name:        "self managed cluster without import secret",
			selfManaged: true,
			wantErr:     false,
			want: &wantData{
				serverURL: "https://kubernetes.default.svc:443",
				ca:        "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
				token:     "fake-token",
			},
		},
		{
			name:       "legacy token exists but serviceaccount secret is missing",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfigWithCustomCA},
			runtimeObjs: []runtime.Object{cm, secretCorrect,
				mockLegacyImportSecret(t, "https://my-dns-name.com:6443", certData2, "legacy-token"),
				// Note: no serviceaccount secret included - simulates it being deleted
			},
			want: &wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  certData2,
				token:     "fake-token", // Should generate new token since legacy validation fails
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)

			fakeKubeClient := kubefake.NewSimpleClientset(tt.runtimeObjs...)

			fakeKubeClient.PrependReactor(
				"create",
				"serviceaccounts/token",
				func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					return true,
						&authv1.TokenRequest{
							Status: authv1.TokenRequestStatus{Token: "fake-token", ExpirationTimestamp: metav1.Now()},
						}, nil
				},
			)

			clientHolder := &helpers.ClientHolder{
				RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).
					WithObjects(tt.clientObjs...).
					WithStatusSubresource(tt.clientObjs...).
					Build(),
				KubeClient: fakeKubeClient,
			}

			cluster := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "testcluster",
				},
			}

			if tt.selfManaged {
				cluster.Labels = map[string]string{
					constants.SelfManagedLabel: "true",
				}
			}

			kubeconfigData, _, _, err := buildBootstrapKubeconfigData(context.Background(), clientHolder, cluster, tt.klusterletConfig) // cluster.Name = testcluster
			if err != nil {
				t.Errorf("buildBootstrapKubeconfigData() error = %v", err)
				return
			}

			if tt.want == nil {
				if kubeconfigData == nil {
					return
				} else {
					t.Errorf("buildBootstrapKubeconfigData() returns wrong data. want nil, got %v", kubeconfigData)
					return
				}
			}
			if tt.want != nil && kubeconfigData == nil {
				t.Errorf("buildBootstrapKubeconfigData() returns wrong data. want %v, got nil", tt.want)
				return
			}

			bootstrapConfig := &clientcmdapi.Config{}
			if err := runtime.DecodeInto(clientcmdlatest.Codec, kubeconfigData, bootstrapConfig); err != nil {
				t.Errorf("buildBootstrapKubeconfigData() failed to decode return data")
				return
			}

			configContext := bootstrapConfig.Contexts[bootstrapConfig.CurrentContext]
			clusterConfig, ok := bootstrapConfig.Clusters[configContext.Cluster]
			if !ok {
				t.Errorf("buildBootstrapKubeconfigData() failed to get %s", configContext.Cluster)
				return
			}
			authInfo, ok := bootstrapConfig.AuthInfos["default-auth"]
			if !ok {
				t.Errorf("buildBootstrapKubeconfigData() failed to get default-auth")
				return
			}

			if clusterConfig.Server != tt.want.serverURL {
				t.Errorf(
					"buildBootstrapKubeconfigData() returns wrong server. want %v, got %v",
					tt.want.serverURL,
					clusterConfig.Server,
				)
			}

			if !reflect.DeepEqual(clusterConfig.CertificateAuthorityData, tt.want.certData) {
				t.Errorf(
					"buildBootstrapKubeconfigData() returns wrong cert. want %v, got %v",
					string(tt.want.certData),
					string(clusterConfig.CertificateAuthorityData),
				)
			}

			if authInfo.Token != tt.want.token {
				t.Errorf(
					"buildBootstrapKubeconfigData() returns wrong token. want %v, got %v",
					tt.want.token,
					authInfo.Token,
				)
			}
		})
	}
}

func mockImportSecret(t *testing.T, expirationTime time.Time, server string, caData []byte, token string) *corev1.Secret {
	bootstrapConfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   server,
			InsecureSkipTLSVerify:    false,
			CertificateAuthorityData: caData,
		}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			Token: token,
		}},
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	boostrapConfigData, err := runtime.Encode(clientcmdlatest.Codec, &bootstrapConfig)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	config := &chart.KlusterletChartConfig{
		CreateNamespace:        false,
		Klusterlet:             chart.KlusterletConfig{Namespace: "test", Name: "klusterlet"},
		BootstrapHubKubeConfig: string(boostrapConfigData),
	}
	_, objects, err := chart.RenderKlusterletChart(config, "test")
	if err != nil {
		t.Fatal(err)
	}

	importYAML := bootstrap.AggregateObjects(objects)

	expiration, err := expirationTime.MarshalText()
	if err != nil {
		t.Fatal(err)
		return nil
	}

	creation, err := time.Now().MarshalText()
	if err != nil {
		t.Fatal(err)
		return nil
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-import",
			Namespace: "testcluster",
		},
		Data: map[string][]byte{
			"import.yaml": importYAML,
			"expiration":  expiration,
			"creation":    creation,
		},
	}
}

func mockLegacyImportSecret(t *testing.T, server string, caData []byte, token string) *corev1.Secret {
	bootstrapConfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   server,
			InsecureSkipTLSVerify:    false,
			CertificateAuthorityData: caData,
		}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			Token: token,
		}},
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	boostrapConfigData, err := runtime.Encode(clientcmdlatest.Codec, &bootstrapConfig)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	config := &chart.KlusterletChartConfig{
		CreateNamespace:        false,
		Klusterlet:             chart.KlusterletConfig{Namespace: "test", Name: "klusterlet"},
		BootstrapHubKubeConfig: string(boostrapConfigData),
	}
	_, objects, err := chart.RenderKlusterletChart(config, "test")
	if err != nil {
		t.Fatal(err)
	}

	importYAML := bootstrap.AggregateObjects(objects)

	// Legacy token - no expiration or creation data
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-import",
			Namespace: "testcluster",
		},
		Data: map[string][]byte{
			"import.yaml": importYAML,
			// Note: no "expiration" or "creation" keys - this makes it a legacy token
		},
	}
}

func mockImportSecretWithoutContent() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-import",
			Namespace: "testcluster",
		},
		Data: map[string][]byte{},
	}
}

func timeToString(time time.Time) []byte {
	timeStr, _ := time.MarshalText()
	return timeStr
}

func TestValidateTokenExpiration(t *testing.T) {
	tests := []struct {
		name                 string
		token                string
		creation, expiration []byte
		expectedResult       bool
	}{
		{
			name:           "token is empty",
			token:          "",
			expectedResult: false,
		},
		{
			name:           "expiration is empty",
			token:          "abc",
			expectedResult: true,
		},
		{
			name:           "creation is empty",
			token:          "abc",
			expiration:     timeToString(time.Now()),
			expectedResult: false,
		},
		{
			name:           "creation is empty",
			token:          "abc",
			expiration:     timeToString(time.Now()),
			expectedResult: false,
		},
		{
			name:           "not expired",
			token:          "abc",
			expiration:     timeToString(time.Now().Add(1 * time.Hour)),
			creation:       timeToString(time.Now().Add(-239 * time.Minute)),
			expectedResult: true,
		},
		{
			name:           "expired",
			token:          "abc",
			expiration:     timeToString(time.Now().Add(1 * time.Hour)),
			creation:       timeToString(time.Now().Add(-241 * time.Minute)),
			expectedResult: false,
		},
		{
			name:           "invalid creation",
			token:          "abc",
			expiration:     timeToString(time.Now().Add(1 * time.Hour)),
			creation:       []byte("abc"),
			expectedResult: false,
		},
		{
			name:           "invalid expiration",
			token:          "abc",
			expiration:     []byte("abc"),
			creation:       timeToString(time.Now().Add(-5 * time.Hour)),
			expectedResult: false,
		},
		{
			name:           "empty creation, not expired",
			token:          "abc",
			expiration:     timeToString(time.Now().Add(73 * time.Hour * 24)),
			expectedResult: true,
		},
		{
			name:           "empty creation, expired",
			token:          "abc",
			expiration:     timeToString(time.Now().Add(71 * time.Hour * 24)),
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedResult != validateTokenExpiration(tt.token, tt.creation, tt.expiration) {
				t.Errorf("validateTokenExpiration() expected %v, got %v", tt.expectedResult, tt.expectedResult)
			}
		})
	}
}

func TestValidateLegacyServiceAccountToken(t *testing.T) {
	tests := []struct {
		name            string
		saName          string
		secretNamespace string
		expectedToken   string
		secrets         []runtime.Object
		expectedResult  bool
	}{
		{
			name:            "token matches existing serviceaccount secret",
			saName:          "test-bootstrap-sa",
			secretNamespace: "test-cluster",
			expectedToken:   "valid-token-123",
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-abc123",
						Namespace: "test-cluster",
					},
					Type: corev1.SecretTypeServiceAccountToken,
					Data: map[string][]byte{
						"token": []byte("valid-token-123"),
					},
				},
			},
			expectedResult: true,
		},
		{
			name:            "token doesn't match existing serviceaccount secret",
			saName:          "test-bootstrap-sa",
			secretNamespace: "test-cluster",
			expectedToken:   "different-token-456",
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-abc123",
						Namespace: "test-cluster",
					},
					Type: corev1.SecretTypeServiceAccountToken,
					Data: map[string][]byte{
						"token": []byte("valid-token-123"),
					},
				},
			},
			expectedResult: false,
		},
		{
			name:            "no serviceaccount secret exists",
			saName:          "test-bootstrap-sa",
			secretNamespace: "test-cluster",
			expectedToken:   "any-token",
			secrets:         []runtime.Object{},
			expectedResult:  false,
		},
		{
			name:            "serviceaccount secret exists but has no token data",
			saName:          "test-bootstrap-sa",
			secretNamespace: "test-cluster",
			expectedToken:   "any-token",
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-abc123",
						Namespace: "test-cluster",
					},
					Type: corev1.SecretTypeServiceAccountToken,
					Data: map[string][]byte{},
				},
			},
			expectedResult: false,
		},
		{
			name:            "wrong secret type",
			saName:          "test-bootstrap-sa",
			secretNamespace: "test-cluster",
			expectedToken:   "any-token",
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-abc123",
						Namespace: "test-cluster",
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"token": []byte("any-token"),
					},
				},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeKubeClient := kubefake.NewSimpleClientset(tt.secrets...)

			result := validateLegacyServiceAccountToken(ctx, fakeKubeClient, tt.saName, tt.secretNamespace, tt.expectedToken)

			if result != tt.expectedResult {
				t.Errorf("validateLegacyServiceAccountToken() = %v, expected %v", result, tt.expectedResult)
			}
		})
	}
}
