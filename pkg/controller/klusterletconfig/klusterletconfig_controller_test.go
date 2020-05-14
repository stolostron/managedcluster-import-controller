// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package klusterletconfig

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

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
	"github.com/open-cluster-management/rcm-controller/pkg/controller/clusterregistry"
)

func TestReconcileKlusterletConfig_Reconcile(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})
	s.AddKnownTypes(klusterletcfgv1beta1.SchemeGroupVersion, &klusterletcfgv1beta1.KlusterletConfig{})
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})

	terminatingKlusterletConfig := &klusterletcfgv1beta1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cluster-name",
			Namespace:         "cluster-namespace",
			DeletionTimestamp: &metav1.Time{time.Now()},
		},
		Spec: klusterletv1beta1.KlusterletSpec{
			ClusterName:      "not-cluster-name",
			ClusterNamespace: "not-cluster-namespace",
		},
	}
	invalidKlusterletConfig := &klusterletcfgv1beta1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
		Spec: klusterletv1beta1.KlusterletSpec{
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

	klusterletConfig := &klusterletcfgv1beta1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
		},
		Spec: klusterletv1beta1.KlusterletSpec{
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
			name: "klusterletConfig do not exist",
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
			name: "terminating klusterletConfig",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, terminatingKlusterletConfig),
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
			name: "invalid klusterletConfig",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, invalidKlusterletConfig),
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
				client: fake.NewFakeClientWithScheme(s, klusterletConfig),
				scheme: s,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "missing resource to generate secret",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, klusterletConfig, cluster),
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
					klusterletConfig,
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
			r := &ReconcileKlusterletConfig{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}

			got, err := r.Reconcile(tt.args.request)

			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileKlusterletConfig.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileKlusterletConfig.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}
