// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
			got, err := getKubeAPIServerAddress(context.Background(), tt.args.client)
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

func TestGetKubeAPIServerCertificate(t *testing.T) {
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
			got, err := getKubeAPIServerCertificate(context.Background(), tt.args.client, tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("getKubeAPIServerCertificate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getKubeAPIServerCertificate() = %v, want %v", got, tt.want)
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

func TestGetBootstrapKubeConfigData(t *testing.T) {
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
			"ca.crt": "fake-root-ca",
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
			"ca.crt": []byte("default-cert-data"),
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
			"tls.crt": []byte("custom-cert-data"),
			"tls.key": []byte("custom-key-data"),
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
	}
	tests := []struct {
		name        string
		clientObjs  []client.Object
		runtimeObjs []runtime.Object
		want        wantData
		wantErr     bool
	}{
		{
			name:        "use default certificate",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{testTokenSecret, cm},
			want: wantData{
				serverURL:   "http://127.0.0.1:6443",
				useInsecure: false,
				certData:    []byte("fake-root-ca"),
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
				certData:    []byte("custom-cert-data"),
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
				certData:    []byte("fake-root-ca"),
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
				certData:    []byte("default-cert-data"),
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
				certData:    []byte("fake-root-ca"),
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
				certData:    []byte("fake-root-ca"),
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name:        "token is valid",
			clientObjs:  []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{secretCorrect, mockImportSecret(t, time.Now().Add(8640*time.Hour), "https://my-dns-name.com:6443", "custom-cert-data")},
			want: wantData{
				serverURL:   "https://my-dns-name.com:6443",
				useInsecure: false,
				certData:    []byte("custom-cert-data"),
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name:        "token is expired",
			clientObjs:  []client.Object{testInfraConfigIP},
			runtimeObjs: []runtime.Object{sa, mockImportSecret(t, time.Now().Add(-1*time.Hour)), cm},
			want: wantData{
				serverURL:   "http://127.0.0.1:6443",
				useInsecure: false,
				certData:    []byte("fake-root-ca"),
				token:       "fake-token",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
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
				RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(tt.clientObjs...).Build(),
				KubeClient:    fakeKubeClinet,
			}

			kubeconfigData, _, err := getBootstrapKubeConfigData(context.Background(), clientHolder, cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("createKubeconfigData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
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

func TestGetImagePullSecret(t *testing.T) {
	cases := []struct {
		name           string
		clientObjs     []client.Object
		secret         *corev1.Secret
		managedCluster *clusterv1.ManagedCluster
		expectedSecret *corev1.Secret
	}{
		{
			name:       "no registry",
			clientObjs: []client.Object{},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
					Namespace: os.Getenv("POD_NAMESPACE"),
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("fake-token"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			expectedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
					Namespace: os.Getenv("POD_NAMESPACE"),
				},
			},
		},
		{
			name:       "has registry",
			clientObjs: []client.Object{},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test1",
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("fake-token"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"open-cluster-management.io/image-registry": "test1.test2",
					},
					Annotations: map[string]string{
						"open-cluster-management.io/image-registries": `{"pullSecret":"test1.test"}`,
					},
				},
			},
			expectedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test1",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(c.secret)
			clientHolder := &helpers.ClientHolder{
				KubeClient:          kubeClient,
				ImageRegistryClient: imageregistry.NewClient(kubeClient),
			}

			secret, err := getImagePullSecret(context.Background(), clientHolder, c.managedCluster)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if secret.Name != c.expectedSecret.Name {
				t.Errorf("expected secret %v, but got %v", c.expectedSecret.Name, secret.Name)
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
			if c.expectedSAName != getBootstrapSAName(c.clusterName) {
				t.Errorf("expected sa %v, but got %v", c.expectedSAName, getBootstrapSAName(c.clusterName))
			}
		})
	}
}

func TestValidateKubeAPIServerAddress(t *testing.T) {
	cases := []struct {
		name               string
		kubeAPIServer      string
		infraKubeAPIServer string
		valid              bool
	}{
		{
			name: "kube apiserver address is empty",
		},
		{
			name:               "address changed",
			kubeAPIServer:      "https://api.my-cluster.example.com:6443",
			infraKubeAPIServer: "https://api-int.my-cluster.example.com:6443",
		},
		{
			name:               "no change",
			kubeAPIServer:      "https://api.my-cluster.example.com:6443",
			infraKubeAPIServer: "https://api.my-cluster.example.com:6443",
			valid:              true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Logf("Test name: %s", c.name)

			clientHolder := &helpers.ClientHolder{
				RuntimeClient: fake.NewClientBuilder().WithScheme(testscheme).WithObjects(&ocinfrav1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Spec: ocinfrav1.InfrastructureSpec{},
					Status: ocinfrav1.InfrastructureStatus{
						APIServerURL: c.infraKubeAPIServer,
					},
				}).Build(),
			}

			valid, err := validateKubeAPIServerAddress(context.TODO(), c.kubeAPIServer, clientHolder)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if valid != c.valid {
				t.Errorf("expected %v, but got %v", c.valid, valid)
			}
		})
	}
}

func TestValidateCAData(t *testing.T) {
	cases := []struct {
		name            string
		clusterName     string
		bootstrapCAData []byte
		currentCAData   []byte
		valid           bool
	}{
		{
			name: "CA data is empty",
		},
		{
			name:            "cert changes",
			bootstrapCAData: []byte("my-ca-bundle"),
			currentCAData:   []byte("my-new-ca-bundle"),
		},
		{
			name:            "no cert change",
			bootstrapCAData: []byte("my-ca-bundle"),
			currentCAData:   []byte("my-ca-bundle"),
			valid:           true,
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
					"tls.crt": c.currentCAData,
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

			valid, err := validateCAData(context.TODO(), c.bootstrapCAData, kubeAPIServer, clientHolder, &clusterv1.ManagedCluster{})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if valid != c.valid {
				t.Errorf("expected %v, but got %v", c.valid, valid)
			}
		})
	}
}

func mockImportSecret(t *testing.T, expirationTime time.Time, args ...string) *corev1.Secret {
	server := "http://fake-server:6443"
	if len(args) > 0 {
		server = args[0]
	}
	caData := []byte("fake-ca")
	if len(args) > 1 {
		caData = []byte(args[1])
	}
	token := "fake-token"
	if len(args) > 2 {
		token = args[2]
	}
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

	template, err := manifestFiles.ReadFile("manifests/klusterlet/bootstrap_secret.yaml")
	if err != nil {
		t.Fatal(err)
		return nil
	}
	config := KlusterletRenderConfig{
		KlusterletNamespace: "test",
		BootstrapKubeconfig: base64.StdEncoding.EncodeToString(boostrapConfigData),
		InstallMode:         string(operatorv1.InstallModeDefault),
	}
	raw := helpers.MustCreateAssetFromTemplate("bootstrap_secret", template, config)

	importYAML := new(bytes.Buffer)
	importYAML.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(raw)))

	expiration, err := expirationTime.MarshalText()
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
			"import.yaml": importYAML.Bytes(),
			"expiration":  expiration,
		},
	}
}
