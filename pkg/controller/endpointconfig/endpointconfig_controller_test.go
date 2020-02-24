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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ocinfrav1 "github.com/openshift/api/config/v1"

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
	"github.com/open-cluster-management/rcm-controller/pkg/controller/clusterregistry"
)

func TestReconcileEndpointConfig_Reconcile(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})
	s.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})

	terminatingEndpointConfig := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cluster-name",
			Namespace:         "cluster-namespace",
			DeletionTimestamp: &metav1.Time{time.Now()},
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName:      "not-cluster-name",
			ClusterNamespace: "not-cluster-namespace",
		},
	}
	invalidEndpointConfig := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName:      "not-cluster-name",
			ClusterNamespace: "not-cluster-namespace",
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

	req := reconcile.Request{
		types.NamespacedName{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
	}

	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}

	type args struct {
		request reconcile.Request
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "endpointConfig do not exist",
			fields: fields{
				client: fake.NewFakeClient(),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: false,
		},
		{
			name: "terminating endpointConfig",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, terminatingEndpointConfig),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: false,
		},
		{
			name: "invalid endpointConfig",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, invalidEndpointConfig),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: true,
		},
		{
			name: "cluster does not exist",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, endpointConfig),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: false,
		},
		{
			name: "missing resource to generate secret",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, endpointConfig, cluster),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: true,
		},
		{
			name: "success",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s,
					endpointConfig,
					cluster,
					infrastructConfig,
					serviceAccount,
					tokenSecret,
					clusterInfoConfigMap(),
				),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileEndpointConfig{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}

			got, err := r.Reconcile(tt.args.request)

			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileEndpointConfig.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileEndpointConfig.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}
