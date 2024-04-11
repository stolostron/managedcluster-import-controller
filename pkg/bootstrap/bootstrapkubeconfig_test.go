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

	configv1 "github.com/openshift/api/config/v1"
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

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &configv1.Infrastructure{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &configv1.APIServer{})
}

func TestCreateBootstrapKubeConfig(t *testing.T) {

	rootCACertData, rootCAKeyData, err := testinghelpers.NewRootCA("test root ca")
	if err != nil {
		t.Errorf("failed to create root ca: %v", err)
	}

	defaultServerCertData, _, err := testinghelpers.NewServerCertificate("default kube-apiserver", rootCACertData, rootCAKeyData)
	if err != nil {
		t.Errorf("failed to create default server cert: %v", err)
	}

	customServerCertData, customServerKeyData, err := testinghelpers.NewServerCertificate("custom kube-apiserver", rootCACertData, rootCAKeyData)
	if err != nil {
		t.Errorf("failed to create default server cert: %v", err)
	}

	proxyServerCertData, _, err := testinghelpers.NewServerCertificate("proxy server", rootCACertData, rootCAKeyData)
	if err != nil {
		t.Errorf("failed to create default server cert: %v", err)
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

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-bootstrap-sa",
			Namespace: "testcluster",
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

	testTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-bootstrap-sa-token-xxxx",
			Namespace: "testcluster",
		},
		Data: map[string][]byte{
			"token":  []byte("fake-token"),
			"ca.crt": defaultServerCertData,
		},
		Type: corev1.SecretTypeServiceAccountToken,
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
		serverURL   string
		useInsecure bool
		certData    []byte
		token       string
		proxyURL    string
	}
	testcases := []struct {
		name             string
		clientObjs       []client.Object
		runtimeObjs      []runtime.Object
		klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
		want             wantData
		wantErr          bool
	}{
		{
			name:        "use default certificate",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{testTokenSecret, cm},
			want: wantData{
				serverURL:   "http://127.0.0.1:6443",
				useInsecure: false,
				certData:    rootCACertData,
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name:        "use named certificate",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{testTokenSecret, secretCorrect},
			want: wantData{
				serverURL:   "https://my-dns-name.com:6443",
				useInsecure: false,
				certData:    customServerCertData,
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name:        "use default when cert not found",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{testTokenSecret, cm},
			want: wantData{
				serverURL:   "https://my-dns-name.com:6443",
				useInsecure: false,
				certData:    rootCACertData,
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name:        "return error cert malformat",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{testTokenSecret, secretWrong},
			want: wantData{
				serverURL:   "",
				useInsecure: false,
				certData:    nil,
				token:       "",
			},
			wantErr: true,
		},
		{
			name:        "roks failed to connect return error",
			clientObjs:  []client.Object{testInfraServerStopped, apiserverConfig, node},
			runtimeObjs: []runtime.Object{testTokenSecret},
			want: wantData{
				serverURL:   serverStopped.URL,
				useInsecure: false,
				certData:    defaultServerCertData,
				token:       "fake-token",
			},
			wantErr: true,
		},
		{
			name:        "roks with no valid cert use default",
			clientObjs:  []client.Object{testInfraServerTLS, apiserverConfig, node},
			runtimeObjs: []runtime.Object{testTokenSecret, cm},
			want: wantData{
				serverURL:   serverTLS.URL,
				useInsecure: false,
				certData:    rootCACertData,
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name:        "no token secrets",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{sa, cm},
			want: wantData{
				serverURL:   "http://127.0.0.1:6443",
				useInsecure: false,
				certData:    rootCACertData,
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name:        "with proxy config",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{testTokenSecret, cm},
			klusterletConfig: newKlusterletConfig(&klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
				HTTPSProxy: "https://127.0.0.1:3129",
				CABundle:   proxyServerCertData,
			}),
			want: wantData{
				serverURL:   "http://127.0.0.1:6443",
				useInsecure: false,
				certData:    mergedCAData,
				token:       "fake-token",
				proxyURL:    "https://127.0.0.1:3129",
			},
			wantErr: false,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)

			fakeKubeClinet := kubefake.NewSimpleClientset(tt.runtimeObjs...)

			fakeKubeClinet.PrependReactor(
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
				KubeClient: fakeKubeClinet,
			}

			kubeconfigData, _, err := CreateBootstrapKubeConfig(context.Background(), clientHolder, GetBootstrapSAName(cluster.Name), cluster.Name, 8640*3600, tt.klusterletConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("createKubeconfigData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				// it's safe to return here, because the last step, if err is not nil, and we don't expect err, it will fail the test
				return
			}

			bootstrapConfig := &clientcmdapi.Config{}
			if err := runtime.DecodeInto(clientcmdlatest.Codec, kubeconfigData, bootstrapConfig); err != nil {
				t.Errorf("createKubeconfigData() failed to decode return data")
				return
			}
			clusterConfig, ok := bootstrapConfig.Clusters["default-cluster"]
			if !ok {
				t.Errorf("createKubeconfigData() failed to get default-cluster")
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
			if clusterConfig.InsecureSkipTLSVerify != tt.want.useInsecure {
				t.Errorf(
					"createKubeconfigData() returns wrong insecure. want %v, got %v",
					tt.want.useInsecure,
					clusterConfig.InsecureSkipTLSVerify,
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
		client client.Client
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetKubeAPIServerAddress(context.Background(), tt.args.client)
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

func TestGetBootstrapSAName(t *testing.T) {
	cases := []struct {
		name           string
		clusterName    string
		expectedSAName string
		managedCluster *clusterv1.ManagedCluster
	}{
		{
			name:           "short name",
			clusterName:    "123456789",
			expectedSAName: "123456789-bootstrap-sa",
		},
		{
			name:           "long name",
			clusterName:    "123456789-123456789-123456789-123456789-123456789-123456789",
			expectedSAName: "123456789-123456789-123456789-123456789-123456789--bootstrap-sa",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.expectedSAName != GetBootstrapSAName(c.clusterName) {
				t.Errorf("expected sa %v, but got %v", c.expectedSAName, GetBootstrapSAName(c.clusterName))
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
				t.Errorf("expected %v, but got %v", c.expectedCAData, caData)
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

func newKlusterletConfig(proxyConfig *klusterletconfigv1alpha1.KubeAPIServerProxyConfig) *klusterletconfigv1alpha1.KlusterletConfig {
	klusterletConfig := &klusterletconfigv1alpha1.KlusterletConfig{}
	if proxyConfig != nil {
		klusterletConfig.Spec.HubKubeAPIServerProxyConfig = *proxyConfig
	}
	return klusterletConfig
}
