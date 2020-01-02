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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	multicloudv1alpha1 "github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/apis/multicloud/v1alpha1"
	multicloudv1beta1 "github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/apis/multicloud/v1beta1"
)

func Test_clusterRegistryNsN(t *testing.T) {
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
				Name:      "cluster-name",
				Namespace: "cluster-namespace",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := clusterRegistryNsN(tt.args.endpointConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("clusterRegistryNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clusterRegistryNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getClusterRegistryCluster(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})
	s.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})

	type args struct {
		client         client.Client
		endpointConfig *multicloudv1alpha1.EndpointConfig
	}

	endpointConfig := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
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

	tests := []struct {
		name    string
		args    args
		want    *clusterregistryv1alpha1.Cluster
		wantErr bool
	}{
		{name: "nil EndpointConfig",
			args: args{
				client:         fake.NewFakeClient([]runtime.Object{}...),
				endpointConfig: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cluster does not exist",
			args: args{
				client:         fake.NewFakeClient([]runtime.Object{}...),
				endpointConfig: endpointConfig,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cluster exist",
			args: args{
				client:         fake.NewFakeClientWithScheme(s, []runtime.Object{cluster}...),
				endpointConfig: endpointConfig,
			},
			want:    cluster,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getClusterRegistryCluster(tt.args.client, tt.args.endpointConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("getClusterRegistryCluster() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getClusterRegistryCluster() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clusterReconcileMapper_Map(t *testing.T) {
	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
	}

	type args struct {
		obj handler.MapObject
	}

	tests := []struct {
		name   string
		mapper *clusterReconcileMapper
		args   args
		want   []reconcile.Request
	}{
		{
			name:   "green",
			mapper: &clusterReconcileMapper{},
			args: args{
				obj: handler.MapObject{
					Meta: cluster,
				},
			},
			want: []reconcile.Request{
				{types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := &clusterReconcileMapper{}
			if got := mapper.Map(tt.args.obj); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clusterReconcileMapper.Map() = %v, want %v", got, tt.want)
			}
		})
	}
}
