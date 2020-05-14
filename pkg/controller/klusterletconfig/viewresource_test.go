// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterregistry contains common utility functions that gets call by many differerent packages
package klusterletconfig

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

func Test_getKlusterletResourceView(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.ResourceView{})

	testResourceView := &mcmv1alpha1.ResourceView{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ResourceView",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster" + "-get-klusterlet",
			Namespace: "test-cluster",
		},
	}

	testclient := fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
		testResourceView,
	}...)

	type args struct {
		r       *ReconcileKlusterletConfig
		cluster *clusterregistryv1alpha1.Cluster
	}

	tests := []struct {
		name    string
		args    args
		want    *mcmv1alpha1.ResourceView
		wantErr bool
	}{
		{
			name: "empty cluster",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: testclient,
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
				r: &ReconcileKlusterletConfig{
					client: testclient,
					scheme: testscheme,
				},
				cluster: &clusterregistryv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "test-cluster",
					},
				},
			},
			want:    testResourceView,
			wantErr: false,
		},
		{
			name: "resourceview does not exists",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{}...),
					scheme: testscheme,
				},
				cluster: &clusterregistryv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "test-cluster",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKlusterletResourceView(tt.args.r.client, tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("getKlusterletResourceView() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil {
				if got.Namespace != tt.want.GetNamespace() || got.Name != tt.want.GetName() {
					t.Errorf("getKlusterletResourceView() = %v, want = %v", got, tt.want)
				}
			}
		})
	}
}

func Test_createResourceView(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.ResourceView{})

	testcluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-cluster",
		},
	}
	resourceView := &mcmv1alpha1.ResourceView{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ResourceView",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster" + "-get-klusterlet",
			Namespace: "test-cluster",
		},
	}
	klusterletConf := &klusterletcfgv1beta1.KlusterletConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: klusterletcfgv1beta1.SchemeGroupVersion.String(),
			Kind:       "Klusterletconfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-cluster",
		},
	}

	type args struct {
		r                *ReconcileKlusterletConfig
		cluster          *clusterregistryv1alpha1.Cluster
		klusterletConfig *klusterletcfgv1beta1.KlusterletConfig
	}

	tests := []struct {
		name    string
		args    args
		want    *mcmv1alpha1.ResourceView
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testcluster, klusterletConf,
					}...),
					scheme: testscheme,
				},
				cluster:          testcluster,
				klusterletConfig: klusterletConf,
			},
			want:    resourceView,
			wantErr: false,
		},
		{
			name: "resourceView already exists",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testcluster, resourceView, klusterletConf,
					}...),
					scheme: testscheme,
				},
				cluster:          testcluster,
				klusterletConfig: klusterletConf,
			},
			want:    resourceView,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createKlusterletResourceview(tt.args.r, tt.args.cluster, tt.args.klusterletConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("createKlusterletResourceview() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil {
				if got.Namespace != tt.want.GetNamespace() || got.Name != tt.want.GetName() {
					t.Errorf("createKlusterletResourceview() = %v, want = %v", got, tt.want)
				}
			}
		})
	}
}
