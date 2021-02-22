// Copyright (c) 2020 Red Hat, Inc.

//Package managedcluster ...
package managedcluster

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_createKubeconfigData(t *testing.T) {

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
			Name:      "test-sa-token",
			Namespace: "test-namespace",
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
					ocinfrav1.APIServerNamedServingCert{
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

	s := scheme.Scheme

	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	type args struct {
		client client.Client
		secret *corev1.Secret
	}
	type wantData struct {
		serverURL   string
		useInsecure bool
		certData    []byte
		token       string
	}
	tests := []struct {
		name    string
		args    args
		want    wantData
		wantErr bool
	}{
		{
			name: "use default certificate",
			args: args{
				client: fake.NewFakeClientWithScheme(s, testInfraConfigIP),
				secret: testTokenSecret,
			},
			want: wantData{
				serverURL:   "http://127.0.0.1:6443",
				useInsecure: false,
				certData:    []byte("default-cert-data"),
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name: "use named certificate",
			args: args{
				client: fake.NewFakeClientWithScheme(s, testInfraConfigDNS, apiserverConfig, secretCorrect),
				secret: testTokenSecret,
			},
			want: wantData{
				serverURL:   "https://my-dns-name.com:6443",
				useInsecure: false,
				certData:    []byte("custom-cert-data"),
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name: "use default when cert not found",
			args: args{
				client: fake.NewFakeClientWithScheme(s, testInfraConfigDNS, apiserverConfig),
				secret: testTokenSecret,
			},
			want: wantData{
				serverURL:   "https://my-dns-name.com:6443",
				useInsecure: false,
				certData:    []byte("default-cert-data"),
				token:       "fake-token",
			},
			wantErr: false,
		},
		{
			name: "return error cert malformat",
			args: args{
				client: fake.NewFakeClientWithScheme(s, testInfraConfigDNS, apiserverConfig, secretWrong),
				secret: testTokenSecret,
			},
			want: wantData{
				serverURL:   "",
				useInsecure: false,
				certData:    nil,
				token:       "",
			},
			wantErr: true,
		},
		{
			name: "roks failed to connect return error",
			args: args{
				client: fake.NewFakeClientWithScheme(s, testInfraServerStopped, apiserverConfig, &corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "ibm",
					},
				}),
				secret: testTokenSecret,
			},
			want: wantData{
				serverURL:   serverStopped.URL,
				useInsecure: false,
				certData:    []byte("default-cert-data"),
				token:       "fake-token",
			},
			wantErr: true,
		},
		{
			name: "roks with no valid cert use default",
			args: args{
				client: fake.NewFakeClientWithScheme(s, testInfraServerTLS, apiserverConfig, &corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "ibm",
					},
				}),
				secret: testTokenSecret,
			},
			want: wantData{
				serverURL:   serverTLS.URL,
				useInsecure: false,
				certData:    []byte("default-cert-data"),
				token:       "fake-token",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			kubeconfigData, err := createKubeconfigData(tt.args.client, tt.args.secret)

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
					tt.want.certData,
					clusterConfig.CertificateAuthorityData,
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
func Test_getValidCertificatesFromURL(t *testing.T) {
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
