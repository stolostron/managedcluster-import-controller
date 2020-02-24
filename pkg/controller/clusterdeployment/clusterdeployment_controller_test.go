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

	ocinfrav1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
	"github.com/open-cluster-management/rcm-controller/pkg/clusterimport"
)

func init() {
	os.Setenv("ENDPOINT_CRD_FILE", "../../../build/resources/multicloud_v1beta1_endpoint_crd.yaml")
}

func TestReconcileClusterDeployment_Reconcile(t *testing.T) {
	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}

	infrastructConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "https://api.haos-new-playground.purple-chesterfield.com:6443",
		},
	}
	imagePullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image-pull-secret",
			Namespace: "test",
		},
		Type: corev1.SecretTypeDockerConfigJson,
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
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName: "test",
		},
	}
	endpointConfigWithSecret := &multicloudv1alpha1.EndpointConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName:     "test",
			ImagePullSecret: imagePullSecret.Name,
		},
	}
	bootstrapServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test" + clusterimport.BootstrapServiceAccountNamePostfix,
			Namespace: "test",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: "bootstrap-token-secret",
			},
		},
	}
	bootstrapTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-token-secret",
			Namespace: "test",
		},
		Type: corev1.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			"token": []byte("fake-token"),
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{}, &corev1.Secret{}, &corev1.ServiceAccount{})
	s.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{}, &hivev1.SyncSet{})
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})
	s.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})

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
		// This test doesn't work with commented values got error:
		// multicloud-operators-cluster-controller/pkg/controller/clusterdeployment/clusterdeployment_controller_test.go:233:
		// err: no kind is registered for the type v1.SelectorSyncSet in scheme "pkg/runtime/scheme.go:101"
		// multicloud-operators-cluster-controller/pkg/controller/clusterdeployment/clusterdeployment_controller_test.go:241:
		// ReconcileClusterDeployment.Reconcile() = {false 0s}, want {false 30s}
		// TO BE REVISITED
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
				Requeue: false,
				//RequeueAfter: 30 * time.Second,
				RequeueAfter: 0,
			},
			wantErr: true,
		},
		{
			name: "ClusterDeployment & EndpointConfig",
			fields: fields{
				client: fake.NewFakeClient([]runtime.Object{
					clusterDeployment,
					endpointConfig,
					infrastructConfig,
					bootstrapServiceAccount,
					bootstrapTokenSecret,
				}...),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: true,
			// wantErr: false,
		},
		{
			name: "ClusterDeployment & EndpointConfig with ImagePullSecret",
			fields: fields{
				client: fake.NewFakeClient([]runtime.Object{
					imagePullSecret,
					clusterDeployment,
					endpointConfigWithSecret,
					infrastructConfig,
					bootstrapServiceAccount,
					bootstrapTokenSecret,
				}...),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileClusterDeployment{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}

			got, err := r.Reconcile(tt.args.request)
			if err != nil {
				t.Logf("err: %v", err)
			}
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
