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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

func Test_clusterRegistryNsN(t *testing.T) {
	type args struct {
		klusterletConfig *klusterletcfgv1beta1.KlusterletConfig
	}

	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name:    "nil KlusterletConfig",
			args:    args{},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "empty KlusterletConfig.Spec.ClusterName",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
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
			name: "empty KlusterletConfig.Spec.ClusterNamespace",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: klusterletv1beta1.KlusterletSpec{
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
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: klusterletv1beta1.KlusterletSpec{
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
			got, err := clusterRegistryNsN(tt.args.klusterletConfig)
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
	s.AddKnownTypes(klusterletcfgv1beta1.SchemeGroupVersion, &klusterletcfgv1beta1.KlusterletConfig{})

	type args struct {
		client           client.Client
		klusterletConfig *klusterletcfgv1beta1.KlusterletConfig
	}

	klusterletConfig := &klusterletcfgv1beta1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: klusterletv1beta1.KlusterletSpec{
			ClusterName:      "cluster-name",
			ClusterNamespace: "cluster-namespace",
		},
	}

	cluster := &clusterregistryv1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "clusterregistry.k8s.io/v1alpha1",
		},
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
		{name: "nil KlusterletConfig",
			args: args{
				client:           fake.NewFakeClient([]runtime.Object{}...),
				klusterletConfig: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cluster does not exist",
			args: args{
				client:           fake.NewFakeClient([]runtime.Object{}...),
				klusterletConfig: klusterletConfig,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cluster exist",
			args: args{
				client:           fake.NewFakeClientWithScheme(s, []runtime.Object{cluster}...),
				klusterletConfig: klusterletConfig,
			},
			want:    cluster,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getClusterRegistryCluster(tt.args.client, tt.args.klusterletConfig)
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
