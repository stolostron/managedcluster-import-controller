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
package clusterdeployment

import (
	"os"
	"reflect"
	"testing"
	"time"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	multicloudv1alpha1 "github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/apis/multicloud/v1alpha1"
	multicloudv1beta1 "github.ibm.com/IBMPrivateCloud/ibm-klusterlet-operator/pkg/apis/multicloud/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func init() {
	os.Setenv("ENDPOINT_CRD_FILE", "../../../build/resources/multicloud_v1beta1_endpoint_crd.yaml")
}

func TestReconcileClusterDeployment_Reconcile(t *testing.T) {
	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}

	clusterInfoConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibmcloud-cluster-info",
			Namespace: "kube-public",
		},
		Data: map[string]string{
			"cluster_address":              "icp-console.apps.haos-new-playground.purple-chesterfield.com",
			"cluster_ca_domain":            "icp-console.apps.haos-new-playground.purple-chesterfield.com",
			"cluster_endpoint":             "https://icp-management-ingress.kube-system.svc:443",
			"cluster_kube_apiserver_host":  "api.haos-new-playground.purple-chesterfield.com",
			"cluster_kube_apiserver_port":  "6443",
			"cluster_name":                 "mycluster",
			"cluster_router_http_port":     "8080",
			"cluster_router_https_port":    "443",
			"edition":                      "Enterprise Edition",
			"openshift_router_base_domain": "apps.haos-new-playground.purple-chesterfield.com",
			"proxy_address":                "icp-proxy.apps.haos-new-playground.purple-chesterfield.com",
			"proxy_ingress_http_port":      "80",
			"proxy_ingress_https_port":     "443",
			"version":                      "3.2.2",
		},
	}
	imagePullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image-pull-secret",
			Namespace: "test",
		},
	}
	clusterDeployment := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "test",
		},
	}
	endpointConfig := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: multicloudv1beta1.EndpointSpec{},
	}
	endpointConfigWithSecret := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ImagePullSecret: imagePullSecret.Name,
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{}, &corev1.Secret{})
	s.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{}, &hivev1.SyncSet{})
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})
	s.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "test",
		},
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
			name: "ClusterDeployment DNE",
			fields: fields{
				client: fake.NewFakeClient([]runtime.Object{}...),
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
			name: "Only ClusterDeployment",
			fields: fields{
				client: fake.NewFakeClient([]runtime.Object{clusterDeployment}...),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "ClusterDeployment & EndpointConfig",
			fields: fields{
				client: fake.NewFakeClient([]runtime.Object{clusterDeployment, endpointConfig, clusterInfoConfigMap}...),
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
			name: "ClusterDeployment & EndpointConfig with ImagePullSecret",
			fields: fields{
				client: fake.NewFakeClient([]runtime.Object{imagePullSecret, clusterDeployment, endpointConfigWithSecret, clusterInfoConfigMap}...),
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
			r := &ReconcileClusterDeployment{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}

			got, err := r.Reconcile(tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileClusterDeployment.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileClusterDeployment.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}
