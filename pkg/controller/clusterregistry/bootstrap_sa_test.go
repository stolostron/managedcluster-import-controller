//Package clusterregistry ...
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
package clusterregistry

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const BootstrapServiceAccountNamePostfix = "-bootstrap-sa"

func Test_bootstrapServiceAccountNsN(t *testing.T) {
	testClusterRegistry := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster",
			Namespace: "testdeploymentcluster",
		},
	}

	type args struct {
		cluster *clusterregistryv1alpha1.Cluster
	}
	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				cluster: nil,
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "empty cluster.Name",
			args: args{
				cluster: &clusterregistryv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "",
						Namespace: "",
					},
				},
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				cluster: testClusterRegistry,
			},
			want: types.NamespacedName{
				Name:      "testdeploymentcluster" + bootstrapServiceAccountNamePostfix,
				Namespace: "testdeploymentcluster",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bootstrapServiceAccountNsN(tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("bootstrapServiceAccountNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bootstrapServiceAccountNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_NewBootstrapServiceAccount(t *testing.T) {
	testClusterRegistry := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster",
			Namespace: "testdeploymentcluster",
		},
	}

	type args struct {
		cluster *clusterregistryv1alpha1.Cluster
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.ServiceAccount
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				cluster: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				cluster: testClusterRegistry,
			},
			want: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testdeploymentcluster" + BootstrapServiceAccountNamePostfix,
					Namespace: "testdeploymentcluster",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewBootstrapServiceAccount(tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBootstrapServiceAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("NewBootstrapServiceAccount() = %v, want %v", got, tt.want)
				}
			} else {
				if got.GetNamespace() != tt.want.GetNamespace() || got.GetName() != tt.want.GetName() {
					t.Errorf("NewBootstrapServiceAccount() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func Test_getBootstrapServiceAccount(t *testing.T) {

	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})

	testSA := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster" + BootstrapServiceAccountNamePostfix,
			Namespace: "testdeploymentcluster",
		},
	}

	testclient := fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
		testSA,
	}...)

	type args struct {
		r       *ReconcileCluster
		cluster *clusterregistryv1alpha1.Cluster
	}

	tests := []struct {
		name    string
		args    args
		want    *corev1.ServiceAccount
		wantErr bool
	}{
		{
			name: "empty cluster registry object",
			args: args{
				r: &ReconcileCluster{
					client: testclient,
					scheme: testscheme,
				},
				cluster: &clusterregistryv1alpha1.Cluster{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "SA does not exist",
			args: args{
				r: &ReconcileCluster{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{}...),
					scheme: testscheme,
				},
				cluster: &clusterregistryv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testdeploymentcluster",
						Namespace: "testdeploymentcluster",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				r: &ReconcileCluster{
					client: testclient,
					scheme: testscheme,
				},
				cluster: &clusterregistryv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testdeploymentcluster",
						Namespace: "testdeploymentcluster",
					},
				},
			},
			want: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testdeploymentcluster" + BootstrapServiceAccountNamePostfix,
					Namespace: "testdeploymentcluster",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getBootstrapServiceAccount(tt.args.r, tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("getBootstrapServiceAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("getBootstrapServiceAccount() = %v, want %v", got, tt.want)
				}
			} else {
				if got.Namespace != tt.want.GetNamespace() || got.Name != tt.want.GetName() {
					println(err.Error())
					t.Errorf("GetBootstrapServiceAccount() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func Test_createBootstrapServiceAccount(t *testing.T) {

	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})

	testscheme.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})

	testClusterRegistry := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster",
			Namespace: "testdeploymentcluster",
		},
	}

	testSA := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster" + BootstrapServiceAccountNamePostfix,
			Namespace: "testdeploymentcluster",
		},
	}

	type args struct {
		r       *ReconcileCluster
		cluster *clusterregistryv1alpha1.Cluster
	}

	tests := []struct {
		name    string
		args    args
		want    *corev1.ServiceAccount
		wantErr bool
	}{
		{
			name: "SA already exists",
			args: args{
				r: &ReconcileCluster{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testClusterRegistry,
						testSA,
					}...),
					scheme: testscheme,
				},
				cluster: testClusterRegistry,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty cluster registry object",
			args: args{
				r: &ReconcileCluster{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testClusterRegistry,
					}...),
					scheme: testscheme,
				},
				cluster: &clusterregistryv1alpha1.Cluster{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				r: &ReconcileCluster{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testClusterRegistry,
					}...),
					scheme: testscheme,
				},
				cluster: testClusterRegistry,
			},
			want: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testdeploymentcluster" + BootstrapServiceAccountNamePostfix,
					Namespace: "testdeploymentcluster",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createBootstrapServiceAccount(tt.args.r, tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("createBootstrapServiceAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("createBootstrapServiceAccount() = %v, want %v", got, tt.want)
				}
			} else {
				if got.Namespace != tt.want.GetNamespace() || got.Name != tt.want.GetName() {
					println(err.Error())
					t.Errorf("createBootstrapServiceAccount() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
