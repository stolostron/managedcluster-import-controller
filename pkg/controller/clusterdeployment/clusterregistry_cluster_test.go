//Package clusterdeployment ...
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
package clusterdeployment

import (
	"reflect"
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_clusterRegistryNsN(t *testing.T) {
	type args struct {
		clusterDeployment *hivev1.ClusterDeployment
	}
	testClusterDeployment := &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ClusterDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster",
			Namespace: "namespace",
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "testdeploymentcluster",
		},
	}

	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name: "null cluster deployment",
			args: args{
				clusterDeployment: nil,
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "empty cluster deployment",
			args: args{
				clusterDeployment: &hivev1.ClusterDeployment{},
			},
			want:    types.NamespacedName{},
			wantErr: false,
		},
		{
			name: "success",
			args: args{
				clusterDeployment: testClusterDeployment,
			},
			want: types.NamespacedName{
				Name:      "testdeploymentcluster",
				Namespace: "testdeploymentcluster",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := clusterRegistryNsN(tt.args.clusterDeployment); !reflect.DeepEqual(got, tt.want) {
				if (err != nil) != tt.wantErr {
					t.Errorf("getClusterRegistryCluster() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				t.Error("got: ", got)
				t.Errorf("clusterRegistryNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getClusterRegistryCluster(t *testing.T) {
	type args struct {
		client            client.Client
		clusterDeployment *hivev1.ClusterDeployment
	}

	testClusterDeployment := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster",
			Namespace: "namespace",
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "testdeploymentcluster",
		},
	}

	testClusterRegistry := &clusterregistryv1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "clusterregistry.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploymentcluster",
			Namespace: "testdeploymentcluster",
		},
	}

	fscheme := scheme.Scheme

	fscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	fscheme.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})

	fclient := fake.NewFakeClientWithScheme(fscheme, []runtime.Object{
		testClusterDeployment,
		testClusterRegistry,
	}...)

	badclient := fake.NewFakeClient()

	tests := []struct {
		name    string
		args    args
		want    *clusterregistryv1alpha1.Cluster
		wantErr bool
	}{
		{
			name: "empty clusterDeployment",
			args: args{
				client:            fclient,
				clusterDeployment: &hivev1.ClusterDeployment{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "client error",
			args: args{
				client:            badclient,
				clusterDeployment: testClusterDeployment,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client:            fclient,
				clusterDeployment: testClusterDeployment,
			},
			want:    testClusterRegistry,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getClusterRegistryCluster(tt.args.client, tt.args.clusterDeployment)
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
