// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"reflect"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_getKubeAPIServerAddress(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})
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
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{}...),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "no error",
			args: args{
				client: fake.NewFakeClientWithScheme(s, infraConfig),
			},
			want:    "http://127.0.0.1:6443",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKubeAPIServerAddress(tt.args.client)
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
func Test_getKubeAPIServerSecretName(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.APIServer{})
	apiserverConfig := &ocinfrav1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.APIServerSpec{
			ServingCerts: ocinfrav1.APIServerServingCerts{
				NamedCertificates: []ocinfrav1.APIServerNamedServingCert{
					ocinfrav1.APIServerNamedServingCert{
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
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{}...),
				name:   "my-secret-name",
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "no name matches",
			args: args{
				client: fake.NewFakeClientWithScheme(s, apiserverConfig),
				name:   "fake-name",
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "success",
			args: args{
				client: fake.NewFakeClientWithScheme(s, apiserverConfig),
				name:   "my-dns-name.com",
			},
			want:    "my-secret-name",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKubeAPIServerSecretName(tt.args.client, tt.args.name)
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

func Test_getKubeAPIServerCertificate(t *testing.T) {
	s := scheme.Scheme
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
		client client.Client
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
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{}...),
				name:   "test-secret",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "wrong type",
			args: args{
				client: fake.NewFakeClientWithScheme(s, secretWrongType),
				name:   "test-secret",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty data",
			args: args{
				client: fake.NewFakeClientWithScheme(s, secretNoData),
				name:   "test-secret",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client: fake.NewFakeClientWithScheme(s, secretCorrect),
				name:   "test-secret",
			},
			want:    []byte("fake-cert-data"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKubeAPIServerCertificate(tt.args.client, tt.args.name)
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

func Test_checkIsIBMCloud(t *testing.T) {
	s := scheme.Scheme
	nodeIBM := &corev1.Node{
		Spec: corev1.NodeSpec{
			ProviderID: "ibm",
		},
	}
	nodeOther := &corev1.Node{}

	type args struct {
		client client.Client
		name   string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "is normal ocp",
			args: args{
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{nodeOther}...),
				name:   "test-secret",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "is ibm",
			args: args{
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{nodeIBM}...),
				name:   "test-secret",
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkIsIBMCloud(tt.args.client)
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
