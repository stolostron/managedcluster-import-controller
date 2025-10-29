// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	ocinfrav1 "github.com/openshift/api/config/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &ocinfrav1.APIServer{})
}

func TestGetKubeAPIServerConfig(t *testing.T) {

	rootCACertData, rootCAKeyData, err := testinghelpers.NewRootCA("test root ca")
	if err != nil {
		t.Errorf("failed to create root ca: %v", err)
	}

	customServerCertData, customServerKeyData, err := testinghelpers.NewServerCertificate("custom kube-apiserver", rootCACertData, rootCAKeyData)
	if err != nil {
		t.Errorf("failed to create custom server cert: %v", err)
	}

	proxyServerCertData, _, err := testinghelpers.NewServerCertificate("proxy server", rootCACertData, rootCAKeyData)
	if err != nil {
		t.Errorf("failed to create proxy server cert: %v", err)
	}

	mergedCAData, err := mergeCertificateData(rootCACertData, proxyServerCertData)
	if err != nil {
		t.Errorf("failed to merge ca cert data: %v", err)
	}

	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testcluster",
		},
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-root-ca.crt",
			Namespace: "testcluster",
		},
		Data: map[string]string{
			"ca.crt": string(rootCACertData),
		},
	}

	proxyCAcm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxy-ca.crt",
			Namespace: "testcluster",
		},
		Data: map[string]string{
			"ca.crt": string(proxyServerCertData),
		},
	}

	testInfraConfigIP := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	testInfraConfigDNS := &ocinfrav1.Infrastructure{
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

	secretCorrect := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			"tls.crt": customServerCertData,
			"tls.key": customServerKeyData,
		},
		Type: corev1.SecretTypeTLS,
	}

	secretWrong := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{},
		Type: corev1.SecretTypeTLS,
	}

	node := &corev1.Node{
		Spec: corev1.NodeSpec{
			ProviderID: "ibm",
		},
	}

	serverStopped := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))

	serverTLS := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))

	testInfraServerTLS := testInfraConfigDNS.DeepCopy()
	testInfraServerTLS.Status.APIServerURL = serverTLS.URL
	testInfraServerStopped := testInfraConfigDNS.DeepCopy()
	testInfraServerStopped.Status.APIServerURL = serverStopped.URL

	type wantData struct {
		serverURL string
		ca        string
		certData  []byte
		proxyURL  string
	}
	testcases := []struct {
		name             string
		clientObjs       []client.Object
		runtimeObjs      []runtime.Object
		klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
		selfManaged      bool
		want             wantData
		wantErr          bool
	}{
		{
			name:        "use default certificate",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm},
			want: wantData{
				serverURL: "http://127.0.0.1:6443",
				certData:  rootCACertData,
			},
			wantErr: false,
		},
		{
			name:        "use named certificate",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{secretCorrect},
			want: wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  customServerCertData,
			},
			wantErr: false,
		},
		{
			name:        "use default when cert not found",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{cm},
			want: wantData{
				serverURL: "https://my-dns-name.com:6443",
				certData:  rootCACertData,
			},
			wantErr: false,
		},
		{
			name:        "return error cert malformat",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{secretWrong},
			wantErr:     true,
		},
		{
			name:       "roks failed to connect return error",
			clientObjs: []client.Object{testInfraServerStopped, apiserverConfig, node},
			wantErr:    true,
		},
		{
			name:        "roks with no valid cert use default",
			clientObjs:  []client.Object{testInfraServerTLS, apiserverConfig, node},
			runtimeObjs: []runtime.Object{cm},
			want: wantData{
				serverURL: serverTLS.URL,
				certData:  rootCACertData,
			},
			wantErr: false,
		},
		{
			name:        "no token secrets",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{secretWrong, cm},
			want: wantData{
				serverURL: "http://127.0.0.1:6443",
				certData:  rootCACertData,
			},
			wantErr: false,
		},
		{
			name:        "with proxy config",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm},
			klusterletConfig: newKlusterletConfig(&klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
				HTTPSProxy: "https://127.0.0.1:3129",
				CABundle:   proxyServerCertData,
			}),
			want: wantData{
				serverURL: "http://127.0.0.1:6443",
				certData:  mergedCAData,
				proxyURL:  "https://127.0.0.1:3129",
			},
			wantErr: false,
		},
		{
			name:        "with proxy config but HubKubeAPIServerConfig exists",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore,
					},
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://127.0.0.1:3129",
						CABundle:   proxyServerCertData,
					},
				},
			},
			want: wantData{
				serverURL: "http://127.0.0.1:6443",
			},
			wantErr: false,
		},
		{
			name:        "with proxy config by klusterletconfig HubKubeAPIServerConfig, empty strategy",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ProxyURL: "https://127.0.0.1:3129",
						TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
							{
								Name: "proxy-server-cert",
								CABundle: klusterletconfigv1alpha1.ConfigMapReference{
									Namespace: "testcluster",
									Name:      "proxy-ca.crt",
								},
							},
						},
					},
				},
			},
			want: wantData{
				serverURL: "http://127.0.0.1:6443",
				certData:  mergedCAData,
				proxyURL:  "https://127.0.0.1:3129",
			},
			wantErr: false,
		},
		{
			name:        "with proxy config by klusterletconfig HubKubeAPIServerConfig from configmap",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseAutoDetectedCABundle,
						ProxyURL:                   "https://127.0.0.1:3129",
						TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
							{
								Name: "proxy-server-cert",
								CABundle: klusterletconfigv1alpha1.ConfigMapReference{
									Namespace: "testcluster",
									Name:      "proxy-ca.crt",
								},
							},
						},
					},
				},
			},
			want: wantData{
				serverURL: "http://127.0.0.1:6443",
				certData:  mergedCAData,
				proxyURL:  "https://127.0.0.1:3129",
			},
			wantErr: false,
		},
		{
			name:        "with custom ca",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseCustomCABundles,
						URL:                        "http://internal.com",
						TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
							{
								Name: "proxy-server-cert",
								CABundle: klusterletconfigv1alpha1.ConfigMapReference{
									Namespace: "testcluster",
									Name:      "proxy-ca.crt",
								},
							},
						},
					},
				},
			},
			want: wantData{
				serverURL: "http://internal.com",
				certData:  proxyServerCertData,
			},
			wantErr: false,
		},
		{
			name:        "with system trust stroe",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore,
						URL:                        "http://internal.com",
						TrustedCABundles: []klusterletconfigv1alpha1.CABundle{
							{
								Name: "proxy-server-cert",
								CABundle: klusterletconfigv1alpha1.ConfigMapReference{
									Namespace: "testcluster",
									Name:      "proxy-ca.crt",
								},
							},
						},
					},
				},
			},
			want: wantData{
				serverURL: "http://internal.com",
			},
			wantErr: false,
		},
		{
			name:        "self managed cluster",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			selfManaged: true,
			want: wantData{
				serverURL: apiServerInternalEndpoint,
				ca:        apiServerInternalEndpointCA,
			},
			wantErr: false,
		},
		{
			name:        "self managed cluster with proxy settings",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ProxyURL: "https://127.0.0.1:3129",
					},
				},
			},
			selfManaged: true,
			want: wantData{
				serverURL: apiServerInternalEndpoint,
				ca:        apiServerInternalEndpointCA,
			},
			wantErr: false,
		},
		{
			name:        "self managed cluster with custom server address",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						URL: "http://internal.com",
					},
				},
			},
			selfManaged: true,
			want: wantData{
				serverURL: "http://internal.com",
				certData:  rootCACertData,
			},
			wantErr: false,
		},
		{
			name:        "self managed cluster with custom strategy",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{cm, proxyCAcm},
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore,
					},
				},
			},
			selfManaged: true,
			want: wantData{
				serverURL: "http://127.0.0.1:6443",
			},
			wantErr: false,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)

			fakeKubeClinet := kubefake.NewSimpleClientset(tt.runtimeObjs...)

			clientHolder := &helpers.ClientHolder{
				RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).
					WithObjects(tt.clientObjs...).
					WithStatusSubresource(tt.clientObjs...).
					Build(),
				KubeClient: fakeKubeClinet,
			}

			kubeAPIServer, proxyURL, ca, caData, err := GetKubeAPIServerConfig(
				context.Background(), clientHolder, cluster.Name, tt.klusterletConfig, tt.selfManaged)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetKubeAPIServerConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				// it's safe to return here, because the last step, if err is not nil, and we don't expect err, it will fail the test
				return
			}

			if kubeAPIServer != tt.want.serverURL {
				t.Errorf(
					"GetKubeAPIServerConfig() returns wrong server. want %v, got %v",
					tt.want.serverURL,
					kubeAPIServer,
				)
			}

			if ca != tt.want.ca {
				t.Errorf(
					"GetKubeAPIServerConfig() returns wrong ca. want %v, got %v",
					tt.want.ca,
					ca,
				)
			}

			if !reflect.DeepEqual(caData, tt.want.certData) {
				t.Errorf(
					"GetKubeAPIServerConfig() returns wrong cert. want %v, got %v",
					string(tt.want.certData),
					string(caData),
				)
			}

			if proxyURL != tt.want.proxyURL {
				t.Errorf(
					"GetKubeAPIServerConfig() returns wrong proxyRUL. want %v, got %v",
					tt.want.proxyURL,
					proxyURL,
				)
			}
		})
	}
}

func TestCreateBootstrapKubeConfig(t *testing.T) {

	rootCACertData, _, err := testinghelpers.NewRootCA("test root ca")
	if err != nil {
		t.Errorf("failed to create root ca: %v", err)
	}

	type wantData struct {
		serverURL      string
		ca             string
		certData       []byte
		token          string
		proxyURL       string
		ctxClusterName string
	}
	testcases := []struct {
		name string

		serverURL      string
		ca             string
		certData       []byte
		token          string
		proxyURL       string
		ctxClusterName string

		want wantData
	}{
		{
			name:           "with CA data",
			serverURL:      "http://127.0.0.1:6443",
			certData:       rootCACertData,
			ctxClusterName: "cluster1",
			token:          "fake-token",
			want: wantData{
				serverURL:      "http://127.0.0.1:6443",
				certData:       rootCACertData,
				ctxClusterName: "cluster1",
				token:          "fake-token",
			},
		},
		{
			name:           "with CA file",
			serverURL:      "http://127.0.0.1:6443",
			ca:             "/etc/ca.crt",
			ctxClusterName: "cluster1",
			token:          "fake-token",
			want: wantData{
				serverURL:      "http://127.0.0.1:6443",
				ca:             "/etc/ca.crt",
				ctxClusterName: "cluster1",
				token:          "fake-token",
			},
		},
		{
			name:           "with both CA file and CA data",
			serverURL:      "http://127.0.0.1:6443",
			ca:             "/etc/ca.crt",
			certData:       rootCACertData,
			ctxClusterName: "cluster1",
			token:          "fake-token",
			want: wantData{
				serverURL:      "http://127.0.0.1:6443",
				certData:       rootCACertData,
				ctxClusterName: "cluster1",
				token:          "fake-token",
			},
		},
	}
	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)

			kubeconfigData, err := CreateBootstrapKubeConfig(tt.ctxClusterName, tt.serverURL, tt.proxyURL, tt.ca, tt.certData, []byte(tt.token))
			if err != nil {
				t.Errorf("CreateBootstrapKubeConfig() error = %v", err)
				return
			}

			bootstrapConfig := &clientcmdapi.Config{}
			if err := runtime.DecodeInto(clientcmdlatest.Codec, kubeconfigData, bootstrapConfig); err != nil {
				t.Errorf("createKubeconfigData() failed to decode return data")
				return
			}
			currentContext := bootstrapConfig.Contexts[bootstrapConfig.CurrentContext]
			if currentContext == nil {
				t.Errorf("createKubeconfigData() failed to get current context")
				return
			}
			clusterConfig, ok := bootstrapConfig.Clusters[currentContext.Cluster]
			if !ok {
				t.Errorf("createKubeconfigData() failed to get %s", currentContext.Cluster)
				return
			}
			authInfo, ok := bootstrapConfig.AuthInfos["default-auth"]
			if !ok {
				t.Errorf("createKubeconfigData() failed to get default-auth")
				return
			}

			if clusterConfig.Server != tt.want.serverURL {
				t.Errorf(
					"createKubeconfigData() returns wrong server. want %v, got %v",
					tt.want.serverURL,
					clusterConfig.Server,
				)
			}

			if clusterConfig.CertificateAuthority != tt.want.ca {
				t.Errorf(
					"createKubeconfigData() returns wrong ca. want %v, got %v",
					tt.want.ca,
					clusterConfig.CertificateAuthority,
				)
			}

			if !reflect.DeepEqual(clusterConfig.CertificateAuthorityData, tt.want.certData) {
				t.Errorf(
					"createKubeconfigData() returns wrong cert. want %v, got %v",
					string(tt.want.certData),
					string(clusterConfig.CertificateAuthorityData),
				)
			}

			if clusterConfig.ProxyURL != tt.want.proxyURL {
				t.Errorf(
					"createKubeconfigData() returns wrong proxyRUL. want %v, got %v",
					tt.want.proxyURL,
					clusterConfig.ProxyURL,
				)
			}

			if authInfo.Token != tt.want.token {
				t.Errorf(
					"createKubeconfigData() returns wrong token. want %v, got %v",
					tt.want.token,
					authInfo.Token,
				)
			}
		})
	}
}

func TestGetKubeAPIServerAddress(t *testing.T) {
	infraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	type args struct {
		client           client.Client
		klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "no cluster",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).Build(),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "no error",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(infraConfig).Build(),
			},
			want:    "http://127.0.0.1:6443",
			wantErr: false,
		},
		{
			name: "use custom address",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(infraConfig).Build(),
				klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerURL: "https://api.acm.example.com:6443",
					},
				},
			},
			want:    "https://api.acm.example.com:6443",
			wantErr: false,
		},
		{
			name: "use custom address but HubKubeAPIServerConfig exists",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(infraConfig).Build(),
				klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerURL: "https://api.acm.example.com:6443",
						HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
							ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore,
						},
					},
				},
			},
			want:    "http://127.0.0.1:6443",
			wantErr: false,
		},
		{
			name: "use custom address from HubKubeAPIServerConfig",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(infraConfig).Build(),
				klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
					Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
						HubKubeAPIServerURL: "https://api.acm.example.com:6443",
						HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
							URL: "https://api.acm-new.example.com:6443",
						},
					},
				},
			},
			want:    "https://api.acm-new.example.com:6443",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetKubeAPIServerAddress(context.Background(), tt.args.client, tt.args.klusterletConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("getKubeAPIServerAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getKubeAPIServerAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetKubeconfigClusterName(t *testing.T) {
	infraConfigID := "ab3f5cbd-d2c8-4563-92d7-342b486a340f"
	infraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
			UID:  types.UID(infraConfigID),
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	tests := []struct {
		name   string
		client client.Client
		want   string
	}{
		{
			name:   "no infra config, get default cluster name",
			client: fake.NewClientBuilder().WithScheme(testscheme).Build(),
			want:   "default-cluster",
		},
		{
			name:   "get cluster name from infra config",
			client: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(infraConfig).Build(),
			want:   infraConfigID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetKubeconfigClusterName(context.Background(), tt.client)
			if err != nil {
				t.Errorf("GetKubeconfigClusterName() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("GetKubeconfigClusterName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetKubeAPIServerSecretName(t *testing.T) {
	apiserverConfig := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{
			ServingCerts: ocinfrav1.APIServerServingCerts{
				NamedCertificates: []ocinfrav1.APIServerNamedServingCert{
					{
						Names:              []string{"my-dns-name.com"},
						ServingCertificate: ocinfrav1.SecretNameReference{Name: "my-secret-name"},
					},
				},
			},
		},
	}

	type args struct {
		client client.Client
		name   string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "not found apiserver",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).Build(),
				name:   "my-secret-name",
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "no name matches",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(apiserverConfig).Build(),
				name:   "fake-name",
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "success",
			args: args{
				client: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(apiserverConfig).Build(),
				name:   "my-dns-name.com",
			},
			want:    "my-secret-name",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKubeAPIServerSecretName(context.Background(), tt.args.client, tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("getKubeAPIServerSecretName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getKubeAPIServerSecretName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCustomKubeAPIServerCertificate(t *testing.T) {
	secretCorrect := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			"tls.crt": []byte("fake-cert-data"),
			"tls.key": []byte("fake-key-data"),
		},
		Type: corev1.SecretTypeTLS,
	}
	secretWrongType := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			"token": []byte("fake-token"),
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}
	secretNoData := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{},
		Type: corev1.SecretTypeTLS,
	}

	type args struct {
		client kubernetes.Interface
		name   string
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "no secret",
			args: args{
				client: kubefake.NewSimpleClientset(),
				name:   "test-secret",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "wrong type",
			args: args{
				client: kubefake.NewSimpleClientset(secretWrongType),
				name:   "test-secret",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty data",
			args: args{
				client: kubefake.NewSimpleClientset(secretNoData),
				name:   "test-secret",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client: kubefake.NewSimpleClientset(secretCorrect),
				name:   "test-secret",
			},
			want:    []byte("fake-cert-data"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCustomKubeAPIServerCertificate(context.Background(), tt.args.client, tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCustomKubeAPIServerCertificate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getCustomKubeAPIServerCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckIsIBMCloud(t *testing.T) {
	nodeIBM := &corev1.Node{
		Spec: corev1.NodeSpec{
			ProviderID: "ibm://123///abc/def",
		},
	}
	nodeOtherWithIBMstring := &corev1.Node{
		Spec: corev1.NodeSpec{
			ProviderID: "baremetalhost:///openshift-machine-api/worker.test.ibm.com/123",
		},
	}
	nodeOther := &corev1.Node{}

	tests := []struct {
		name    string
		client  client.Client
		want    bool
		wantErr bool
	}{
		{
			name:    "is normal ocp",
			client:  fake.NewClientBuilder().WithScheme(testscheme).WithObjects(nodeOther).Build(),
			want:    false,
			wantErr: false,
		},
		{
			name:    "is ibm",
			client:  fake.NewClientBuilder().WithScheme(testscheme).WithObjects(nodeIBM).Build(),
			want:    true,
			wantErr: false,
		},
		{
			name:    "is other with ibm string",
			client:  fake.NewClientBuilder().WithScheme(testscheme).WithObjects(nodeOtherWithIBMstring).Build(),
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkIsIBMCloud(context.Background(), tt.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkIsROKS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("checkIsROKS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetValidCertificatesFromURL(t *testing.T) {
	serverStopped := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	serverTLS := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	rootTLS := x509.NewCertPool()
	rootTLS.AddCert(serverTLS.Certificate())
	tests := []struct {
		name    string
		url     string
		root    *x509.CertPool
		want    []*x509.Certificate
		wantErr bool
	}{
		{
			name:    "invalid url",
			url:     "abc:@@@@ /invalid:url/",
			root:    nil,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "tls connection failed",
			url:     serverStopped.URL,
			root:    nil,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "unauthorized certs",
			url:     serverTLS.URL,
			root:    nil,
			want:    nil,
			wantErr: false,
		},
		{
			name:    "valid certs",
			url:     serverTLS.URL,
			root:    rootTLS,
			want:    []*x509.Certificate{serverTLS.Certificate()},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getValidCertificatesFromURL(tt.url, tt.root)
			if (err != nil) != tt.wantErr {
				t.Errorf(
					"getValidCertificatesFromURL() returns wrong error. want %t, got %v",
					tt.wantErr,
					err,
				)
			} else if err == nil {
				if len(tt.want) != len(got) {
					t.Errorf("getValidCertificatesFromURL() returns wrong number of certificates. want %d, got %d\n",
						len(tt.want), len(got))
				}
				for i, gotCert := range got {
					wantCert := tt.want[i]
					if !wantCert.Equal(gotCert) {
						t.Errorf("getValidCertificatesFromURL() returns wrong number of certificates. want %v, got %v\n",
							tt.want, got)
					}
				}
			}
		})
	}
}

func TestGetProxySettings(t *testing.T) {
	tests := []struct {
		name             string
		klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
		proxyURL         string
		proxyCAData      []byte
	}{
		{
			name: "no proxy",
		},
		{
			name: "http proxy",
			klusterletConfig: newKlusterletConfig(&klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
				HTTPProxy: "http://127.0.0.1:3128",
				CABundle:  []byte("fake-ca-cert"),
			}),
			proxyURL: "http://127.0.0.1:3128",
		},
		{
			name: "https proxy",
			klusterletConfig: newKlusterletConfig(&klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
				HTTPSProxy: "https://127.0.0.1:3129",
				CABundle:   []byte("fake-ca-cert"),
			}),
			proxyURL:    "https://127.0.0.1:3129",
			proxyCAData: []byte("fake-ca-cert"),
		},
		{
			name: "both",
			klusterletConfig: newKlusterletConfig(&klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
				HTTPProxy:  "http://127.0.0.1:3128",
				HTTPSProxy: "https://127.0.0.1:3129",
				CABundle:   []byte("fake-ca-cert"),
			}),
			proxyURL:    "https://127.0.0.1:3129",
			proxyCAData: []byte("fake-ca-cert"),
		},
		{
			name: "merged config - ProxyURL equals HTTPSProxy with CA bundle",
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ProxyURL: "https://127.0.0.1:3129",
					},
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://127.0.0.1:3129",
						CABundle:   []byte("merged-ca-cert"),
					},
				},
			},
			proxyURL:    "https://127.0.0.1:3129",
			proxyCAData: []byte("merged-ca-cert"),
		},
		{
			name: "merged config - ProxyURL does not equal HTTPSProxy",
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ProxyURL: "https://127.0.0.1:3130",
					},
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://127.0.0.1:3129",
						CABundle:   []byte("merged-ca-cert"),
					},
				},
			},
			proxyURL:    "https://127.0.0.1:3130",
			proxyCAData: nil,
		},
		{
			name: "merged config - ProxyURL equals HTTPSProxy without CA bundle",
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ProxyURL: "https://127.0.0.1:3129",
					},
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://127.0.0.1:3129",
					},
				},
			},
			proxyURL:    "https://127.0.0.1:3129",
			proxyCAData: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyURL, caData := GetProxySettings(tt.klusterletConfig)
			if proxyURL != tt.proxyURL {
				t.Errorf("GetProxySettings() = %v, want %v", proxyURL, tt.proxyURL)
			}
			if !reflect.DeepEqual(caData, tt.proxyCAData) {
				t.Errorf("GetProxySettings() = %v, want %v", caData, tt.proxyCAData)
			}
		})
	}
}

func TestGetBootstrapCAData(t *testing.T) {
	certData1, _, _ := testinghelpers.NewRootCA("test ca1")
	certData2, _, _ := testinghelpers.NewRootCA("test ca2")
	mergedCAData, _ := mergeCertificateData(certData1, certData2)

	cases := []struct {
		name             string
		apiServerCAData  []byte
		klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
		expectedCAData   []byte
	}{
		{
			name:            "witout proxy ca",
			apiServerCAData: certData1,
			expectedCAData:  certData1,
		},
		{
			name:            "with blank line in api server certs",
			apiServerCAData: []byte(fmt.Sprintf("%s\n\n%s", string(certData1), string(certData2))),
			expectedCAData:  mergedCAData,
		},
		{
			name:            "with custom ca",
			apiServerCAData: certData1,
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerCABundle: certData2,
				},
			},
			expectedCAData: certData2,
		},
		{
			name:            "with custom ca but HubKubeAPIServerConfig exists",
			apiServerCAData: certData1,
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerCABundle: certData2,
					HubKubeAPIServerConfig: &klusterletconfigv1alpha1.KubeAPIServerConfig{
						ServerVerificationStrategy: klusterletconfigv1alpha1.ServerVerificationStrategyUseSystemTruststore,
					},
				},
			},
			expectedCAData: nil,
		},
		{
			name:            "with proxy ca",
			apiServerCAData: certData1,
			klusterletConfig: &klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://127.0.0.1:3128",
						CABundle:   certData2,
					},
				},
			},
			expectedCAData: mergedCAData,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Logf("Test name: %s", c.name)

			fqdn := "api.my-cluster.example.com"
			kubeAPIServer := fmt.Sprintf("https://%s:6443", fqdn)
			secretName := "my-secret-name"

			fakeKubeClient := kubefake.NewSimpleClientset(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: "openshift-config",
				},
				Data: map[string][]byte{
					"tls.crt": c.apiServerCAData,
				},
			})

			clientHolder := &helpers.ClientHolder{
				RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(&ocinfrav1.APIServer{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Spec: ocinfrav1.APIServerSpec{
						ServingCerts: ocinfrav1.APIServerServingCerts{
							NamedCertificates: []ocinfrav1.APIServerNamedServingCert{
								{
									Names:              []string{fqdn},
									ServingCertificate: ocinfrav1.SecretNameReference{Name: secretName},
								},
							},
						},
					},
				}).Build(),
				KubeClient: fakeKubeClient,
			}

			caData, err := GetBootstrapCAData(context.TODO(), clientHolder, kubeAPIServer, "cluster", c.klusterletConfig)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(caData, c.expectedCAData) {
				t.Errorf("expected %s, but got %s", c.expectedCAData, caData)
			}
		})
	}
}

func TestMergeCertificateData(t *testing.T) {
	certData, _, err := testinghelpers.NewRootCA("test ca")
	if err != nil {
		t.Errorf("failed to create root ca: %v", err)
	}

	tests := []struct {
		name      string
		caBundles [][]byte
		merged    []byte
		wantErr   bool
	}{
		{
			name: "no bundle",
		},
		{
			name: "invalid cert",
			caBundles: [][]byte{
				[]byte("invalid-cert"),
			},
			wantErr: true,
		},
		{
			name:      "one cert",
			caBundles: [][]byte{certData},
			merged:    certData,
		},
		{
			name:      "two same certs",
			caBundles: [][]byte{certData, certData},
			merged:    certData,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := mergeCertificateData(tt.caBundles...)
			if (err != nil) != tt.wantErr {
				t.Errorf(
					"mergeCertificateData() returns wrong error. want %t, got %v",
					tt.wantErr,
					err,
				)
			} else if err == nil {
				if !reflect.DeepEqual(merged, tt.merged) {
					t.Errorf("mergeCertificateData() = %v, want %v", merged, tt.merged)
				}
			}
		})
	}
}

func newKlusterletConfig(
	proxyConfig *klusterletconfigv1alpha1.KubeAPIServerProxyConfig) *klusterletconfigv1alpha1.KlusterletConfig {
	klusterletConfig := &klusterletconfigv1alpha1.KlusterletConfig{}
	if proxyConfig != nil {
		klusterletConfig.Spec.HubKubeAPIServerProxyConfig = *proxyConfig
	}
	return klusterletConfig
}

func TestValidateBootstrapKubeconfig(t *testing.T) {
	certData1, _, _ := testinghelpers.NewRootCA("test ca1")
	certData2, _, _ := testinghelpers.NewRootCA("test ca2")

	cases := []struct {
		name           string
		kubeAPIServer  string
		proxyURL       string
		ca             string
		caData         []byte
		ctxClusterName string

		requiredKubeAPIServer  string
		requiredProxyURL       string
		requiredCA             string
		requiredCAData         []byte
		requiredCtxClusterName string

		valid bool
	}{
		{
			name:                  "kube apiserver address is empty",
			requiredKubeAPIServer: "https://api-int.my-cluster.example.com:6443",
		},
		{
			name:                  "address changed",
			kubeAPIServer:         "https://api.my-cluster.example.com:6443",
			requiredKubeAPIServer: "https://api-int.my-cluster.example.com:6443",
		},
		{
			name:             "proxy is empty",
			requiredProxyURL: "https://127.0.0.1:3128",
		},
		{
			name:             "proxy changed",
			proxyURL:         "http://127.0.0.1:3128",
			requiredProxyURL: "https://127.0.0.1:3128",
		},
		{
			name:           "CA data is empty",
			requiredCAData: certData1,
		},
		{
			name:           "ca data changed",
			caData:         certData1,
			requiredCAData: certData2,
		},
		{
			name:       "replace ca data with ca file",
			caData:     certData1,
			requiredCA: "/etc/ca.crt",
		},
		{
			name:           "replace ca file with ca data",
			requiredCA:     "/etc/ca.crt",
			requiredCAData: certData1,
		},
		{
			name:       "ca file is empty",
			requiredCA: "/etc/ca.crt",
		},
		{
			name:       "ca file changed",
			ca:         "/etc/ca.crt",
			requiredCA: "/etc/new-ca.crt",
		},
		{
			name:                   "all valid",
			kubeAPIServer:          "https://api.my-cluster.example.com:6443",
			requiredKubeAPIServer:  "https://api.my-cluster.example.com:6443",
			caData:                 certData1,
			requiredCAData:         certData1,
			ctxClusterName:         "my-cluster",
			requiredCtxClusterName: "my-cluster",
			valid:                  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Logf("Test name: %s", c.name)
			valid := ValidateBootstrapKubeconfig("cluster1", c.kubeAPIServer, c.proxyURL, c.ca, c.caData, c.ctxClusterName,
				c.requiredKubeAPIServer, c.requiredProxyURL, c.requiredCA, c.requiredCAData, c.requiredCtxClusterName)
			if valid != c.valid {
				t.Errorf("expected %v, but got %v", c.valid, valid)
			}
		})
	}
}
