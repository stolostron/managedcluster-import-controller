//Package endpointconfig ...
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package endpointconfig

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
	"github.com/open-cluster-management/rcm-controller/pkg/controller/clusterregistry"
)

func init() {
	os.Setenv("ENDPOINT_CRD_FILE", "../../../build/resources/multicloud_v1beta1_endpoint_crd.yaml")
}

func Test_importSecretNsN(t *testing.T) {
	type args struct {
		endpointConfig *multicloudv1alpha1.EndpointConfig
	}

	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name:    "nil EndpointConfig",
			args:    args{},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "empty EndpointConfig.Spec.ClusterName",
			args: args{
				endpointConfig: &multicloudv1alpha1.EndpointConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "empty EndpointConfig.Spec.ClusterNamespace",
			args: args{
				endpointConfig: &multicloudv1alpha1.EndpointConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: multicloudv1beta1.EndpointSpec{
						ClusterName: "cluster-name",
					},
				},
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "no error",
			args: args{
				endpointConfig: &multicloudv1alpha1.EndpointConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: multicloudv1beta1.EndpointSpec{
						ClusterName:      "cluster-name",
						ClusterNamespace: "cluster-namespace",
					},
				},
			},
			want: types.NamespacedName{
				Name:      "cluster-name" + importSecretNamePostfix,
				Namespace: "cluster-namespace",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := importSecretNsN(tt.args.endpointConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("importSecretNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("importSecretNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getImportSecret(t *testing.T) {
	type args struct {
		client         client.Client
		endpointConfig *multicloudv1alpha1.EndpointConfig
	}

	testEndpointConfig := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName:      "cluster-name",
			ClusterNamespace: "cluster-namespace",
		},
	}

	testSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name" + importSecretNamePostfix,
			Namespace: "cluster-namespace",
		},
	}

	tests := []struct {
		name    string
		args    args
		want    *corev1.Secret
		wantErr bool
	}{
		{
			name: "nil EndpointConfig",
			args: args{
				client:         fake.NewFakeClient([]runtime.Object{}...),
				endpointConfig: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "secret does not exist",
			args: args{
				client:         fake.NewFakeClient([]runtime.Object{}...),
				endpointConfig: testEndpointConfig,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "secret does exist",
			args: args{
				client:         fake.NewFakeClient([]runtime.Object{testSecret}...),
				endpointConfig: testEndpointConfig,
			},
			want:    testSecret,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getImportSecret(tt.args.client, tt.args.endpointConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("getImportSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getImportSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newImportSecret(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})
	s.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})

	endpointConfig := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName:      "cluster-name",
			ClusterNamespace: "cluster-namespace",
		},
	}

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
	}

	infrastructConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "https://cluster-name.com:6443",
		},
	}

	serviceAccount, err := clusterregistry.NewBootstrapServiceAccount(cluster)
	if err != nil {
		t.Errorf("fail to initialize bootstrap serviceaccount, error = %v", err)
	}

	tokenSecret, err := serviceAccountTokenSecret(serviceAccount)
	if err != nil {
		t.Errorf("fail to initialize serviceaccount token secret, error = %v", err)
	}

	serviceAccount.Secrets = append(serviceAccount.Secrets, corev1.ObjectReference{
		Name: tokenSecret.Name,
	})

	type args struct {
		client         client.Client
		scheme         *runtime.Scheme
		endpointConfig *multicloudv1alpha1.EndpointConfig
	}

	tests := []struct {
		name    string
		args    args
		wantNil bool
		wantErr bool
	}{
		{
			name: "nil scheme",
			args: args{
				client:         fake.NewFakeClient([]runtime.Object{}...),
				scheme:         nil,
				endpointConfig: nil,
			},
			wantNil: true,
			wantErr: true,
		},
		{
			name: "nil endpointConfig",
			args: args{
				client:         fake.NewFakeClientWithScheme(s, []runtime.Object{}...),
				scheme:         s,
				endpointConfig: nil,
			},
			wantNil: true,
			wantErr: true,
		},
		{
			name: "empty endpointConfig",
			args: args{
				client:         fake.NewFakeClientWithScheme(s, []runtime.Object{}...),
				scheme:         s,
				endpointConfig: &multicloudv1alpha1.EndpointConfig{},
			},
			wantNil: true,
			wantErr: true,
		},
		{
			name: "no error",
			args: args{
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{
					endpointConfig,
					cluster,
					infrastructConfig,
					serviceAccount,
					tokenSecret,
					clusterInfoConfigMap(),
				}...),
				scheme:         s,
				endpointConfig: endpointConfig,
			},
			wantNil: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newImportSecret(tt.args.client, tt.args.scheme, tt.args.endpointConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("newImportSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if (got == nil) != tt.wantNil {
				t.Errorf("newImportSecret() = %v, want %v", got, tt.wantNil)
				return
			}
		})
	}
}

func Test_createImportSecret(t *testing.T) {
	endpointConfig := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName:      "cluster-name",
			ClusterNamespace: "cluster-namespace",
		},
	}

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
	}

	infrastructConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "https://cluster-name.com:6443",
		},
	}
	serviceAccount, err := clusterregistry.NewBootstrapServiceAccount(cluster)
	if err != nil {
		t.Errorf("fail to initialize bootstrap serviceaccount, error = %v", err)
	}

	tokenSecret, err := serviceAccountTokenSecret(serviceAccount)
	if err != nil {
		t.Errorf("fail to initialize serviceaccount token secret, error = %v", err)
	}

	serviceAccount.Secrets = append(serviceAccount.Secrets, corev1.ObjectReference{
		Name: tokenSecret.Name,
	})

	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})
	s.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})

	fakeClient := fake.NewFakeClientWithScheme(s,
		endpointConfig,
		cluster,
		infrastructConfig,
		serviceAccount,
		tokenSecret,
		clusterInfoConfigMap(),
	)

	importSecret, err := newImportSecret(fakeClient, s, endpointConfig)
	if err != nil {
		t.Errorf("fail to initialize import secret, error = %v", err)
	}

	type args struct {
		client         client.Client
		scheme         *runtime.Scheme
		endpointConfig *multicloudv1alpha1.EndpointConfig
	}

	tests := []struct {
		name    string
		args    args
		want    *corev1.Secret
		wantErr bool
	}{
		{
			name: "no error",
			args: args{
				client:         fakeClient,
				scheme:         s,
				endpointConfig: endpointConfig,
			},
			want:    importSecret,
			wantErr: false,
		},
		{
			name: "secret already exist",
			args: args{
				client: fake.NewFakeClientWithScheme(s,
					endpointConfig,
					cluster,
					serviceAccount,
					tokenSecret,
					clusterInfoConfigMap(),
					importSecret,
				),
				scheme:         s,
				endpointConfig: endpointConfig,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createImportSecret(tt.args.client, tt.args.scheme, tt.args.endpointConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("createImportSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && got != nil {
				tt.want.ObjectMeta.ResourceVersion = got.ObjectMeta.ResourceVersion
				tt.want.ObjectMeta.OwnerReferences[0].Controller = got.ObjectMeta.OwnerReferences[0].Controller
				tt.want.ObjectMeta.OwnerReferences[0].BlockOwnerDeletion = got.ObjectMeta.OwnerReferences[0].BlockOwnerDeletion
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createImportSecret() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func Test_toYAML(t *testing.T) {
	testCases := []struct {
		Name    string
		Objects []runtime.Object
		Output  []byte
	}{
		{
			Name:    "no objects",
			Objects: []runtime.Object{},
			Output:  nil,
		},
		{
			Name: "configmap",
			Objects: []runtime.Object{
				&corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						APIVersion: corev1.SchemeGroupVersion.String(),
						Kind:       "ConfigMap",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cm-test",
						Namespace: "test",
					},
				},
				&corev1.ServiceAccount{
					TypeMeta: metav1.TypeMeta{
						APIVersion: corev1.SchemeGroupVersion.String(),
						Kind:       "ServiceAccount",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sa-test",
						Namespace: "test",
					},
				},
			},
			Output: []byte(`
---
apiVersion: v1
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: cm-test
  namespace: test

---
apiVersion: v1
kind: ServiceAccount
metadata:
  creationTimestamp: null
  name: sa-test
  namespace: test
`),
		},
	}
	for _, testCase := range testCases {
		yaml, err := toYAML(testCase.Objects)
		assert.NoError(t, err)
		assert.Equal(t, testCase.Output, yaml)
	}
}

func serviceAccountTokenSecret(serviceAccount *corev1.ServiceAccount) (*corev1.Secret, error) {
	if serviceAccount == nil {
		return nil, fmt.Errorf("serviceAccount can not be nil")
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount.GetName(),
			Namespace: serviceAccount.GetNamespace(),
		},
		Data: map[string][]byte{
			"token": []byte("fake-token"),
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}, nil
}

func clusterInfoConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibmcloud-cluster-info",
			Namespace: "kube-public",
		},
		Data: map[string]string{
			"cluster_kube_apiserver_host": "api.test.com",
			"cluster_kube_apiserver_port": "6443",
		},
	}
}
