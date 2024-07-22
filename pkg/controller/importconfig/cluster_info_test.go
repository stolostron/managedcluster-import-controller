// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"testing"
	"time"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetBootstrapKubeConfigDataFromImportSecret(t *testing.T) {
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

	type wantData struct {
		serverURL   string
		useInsecure bool
		certData    []byte
		token       string
	}
	tests := []struct {
		name             string
		clientObjs       []client.Object
		runtimeObjs      []runtime.Object
		klusterletConfig *klusterletconfigv1alpha1.KlusterletConfig
		want             *wantData
		wantErr          bool
	}{
		{
			name:    "import secret not exist",
			wantErr: false,
		},
		{
			name:        "import secret don't have the desired content",
			runtimeObjs: []runtime.Object{mockImportSecretWithoutContent()},
			wantErr:     false,
		},
		{
			name:       "token is expired",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(-1*time.Hour),
					"https://my-dns-name.com:6443",
					certData2,
					"mock-token"),
			},
			wantErr: false,
		},
		{
			name:       "caData not validate",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					[]byte("wrong"),
					"mock-token"),
			},
			wantErr: false,
		},
		{
			name:       "kubeAPIServer not validate",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://wrong.com:6443",
					certData2,
					"mock-token"),
			},
			wantErr: false,
		},
		{
			name:       "kubeconfig current context cluster name not match",
			clientObjs: []client.Object{testInfraConfigWithoutUID, apiserverConfig},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					certData2,
					"mock-token"),
			},
			wantErr: false,
		},
		{
			name:       "all fileds are valid",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{secretCorrect,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					certData2,
					"mock-token"),
			},
			wantErr: false,
			want: &wantData{
				serverURL:   "https://my-dns-name.com:6443",
				useInsecure: false,
				certData:    certData2,
				token:       "mock-token",
			},
		},
		{
			name:       "all fileds are valid, failed to get the ca from ocp, fallback to the kube-root-ca.crt configmap from the pod namespace.",
			clientObjs: []client.Object{testInfraConfigDNS, apiserverConfig},
			runtimeObjs: []runtime.Object{cm,
				mockImportSecret(t, time.Now().Add(8640*time.Hour),
					"https://my-dns-name.com:6443",
					certData1,
					"mock-token"),
			},
			wantErr: false,
			want: &wantData{
				serverURL:   "https://my-dns-name.com:6443",
				useInsecure: false,
				certData:    certData1,
				token:       "mock-token",
			},
		},
	}
	for _, tt := range tests {
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

			kubeconfigData, _, err := getBootstrapKubeConfigDataFromImportSecret(context.Background(), clientHolder, "testcluster", tt.klusterletConfig) // cluster.Name = testcluster
			if (err != nil) != tt.wantErr {
				t.Errorf("getBootstrapKubeConfigDataFromImportSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				// it's safe to return here, because the last step, if err is not nil, and we don't expect err, it will fail the test
				return
			}

			if tt.want == nil {
				if kubeconfigData == nil {
					return
				} else {
					t.Errorf("getBootstrapKubeConfigDataFromImportSecret() returns wrong data. want nil, got %v", kubeconfigData)
					return
				}
			}
			if tt.want != nil && kubeconfigData == nil {
				t.Errorf("getBootstrapKubeConfigDataFromImportSecret() returns wrong data. want %v, got nil", tt.want)
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

	template, err := bootstrap.ManifestFiles.ReadFile("manifests/klusterlet/bootstrap_secret.yaml")
	if err != nil {
		t.Fatal(err)
		return nil
	}
	config := bootstrap.KlusterletRenderConfig{
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

func mockImportSecretWithoutContent() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster-import",
			Namespace: "testcluster",
		},
		Data: map[string][]byte{},
	}
}
