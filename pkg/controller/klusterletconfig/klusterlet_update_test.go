// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
// Copyright (c) 2020 Red Hat, Inc.

package klusterletconfig

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

func Test_getKlusterletUpdateWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})
	testscheme.AddKnownTypes(klusterletcfgv1beta1.SchemeGroupVersion, &klusterletcfgv1beta1.KlusterletConfig{})

	testKlusterletUpdateWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-update-klusterlet",
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
		klusterletconfig *klusterletcfgv1beta1.KlusterletConfig
	}

	tests := []struct {
		name    string
		args    args
		want    *mcmv1alpha1.Work
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						klusterletConf, testKlusterletUpdateWork,
					}...),
					scheme: testscheme,
				},
				klusterletconfig: klusterletConf,
			},
			want:    testKlusterletUpdateWork,
			wantErr: false,
		},
		{
			name: "klusterlet update work does not exists",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						klusterletConf,
					}...),
					scheme: testscheme,
				},
				klusterletconfig: klusterletConf,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKlusterletUpdateWork(tt.args.r, tt.args.klusterletconfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("getKlusterletUpdateWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil {
				if got.Namespace != tt.want.GetNamespace() || got.Name != tt.want.GetName() {
					t.Errorf("getKlusterletUpdateWork() = %v, want = %v", got, tt.want)
				}
			}
		})
	}
}

func Test_createKlusterletUpdateWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})
	testscheme.AddKnownTypes(klusterletcfgv1beta1.SchemeGroupVersion, &klusterletcfgv1beta1.KlusterletConfig{})
	testscheme.AddKnownTypes(klusterletv1beta1.SchemeGroupVersion, &klusterletv1beta1.Klusterlet{})

	klusterletConf := &klusterletcfgv1beta1.KlusterletConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: klusterletcfgv1beta1.SchemeGroupVersion.String(),
			Kind:       "Klusterletconfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-cluster",
		},
		Spec: klusterletv1beta1.KlusterletSpec{
			ClusterName: "cluster-name",
		},
	}

	testKlusterletUpdateWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      klusterletConf.Name + "-update-klusterlet",
			Namespace: "test-cluster",
		},
	}

	testKlusterlet := &klusterletv1beta1.Klusterlet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: klusterletv1beta1.SchemeGroupVersion.String(),
			Kind:       "Klusterlet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "klusterlet",
			Namespace: "klusterlet",
		},
		Spec: klusterletv1beta1.KlusterletSpec{
			ClusterName: "cluster-name",
		},
	}

	type args struct {
		r                *ReconcileKlusterletConfig
		klusterletConfig *klusterletcfgv1beta1.KlusterletConfig
		klusterlet       *klusterletv1beta1.Klusterlet
	}

	tests := []struct {
		name    string
		args    args
		want    *mcmv1alpha1.Work
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						klusterletConf, testKlusterlet,
					}...),
					scheme: testscheme,
				},
				klusterletConfig: klusterletConf,
				klusterlet:       testKlusterlet,
			},
			want:    testKlusterletUpdateWork,
			wantErr: false,
		},
		{
			name: "update klusterlet work already exists",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						klusterletConf, testKlusterlet, testKlusterletUpdateWork,
					}...),
					scheme: testscheme,
				},
				klusterletConfig: klusterletConf,
				klusterlet:       testKlusterlet,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createKlusterletUpdateWork(tt.args.r, tt.args.klusterletConfig, tt.args.klusterlet)
			if (err != nil) != tt.wantErr {
				t.Errorf("createKlusterletUpdateWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func Test_deleteKlusterletUpdateWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})

	testKlusterletUpdateWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-update-klusterlet",
			Namespace: "test-cluster",
		},
	}

	type args struct {
		r    *ReconcileKlusterletConfig
		work *mcmv1alpha1.Work
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testKlusterletUpdateWork,
					}...),
					scheme: testscheme,
				},
				work: testKlusterletUpdateWork,
			},
			wantErr: false,
		},
		{
			name: "update klusterlet work not exists",
			args: args{
				r: &ReconcileKlusterletConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{}...),
					scheme: testscheme,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := deleteKlusterletUpdateWork(tt.args.r, tt.args.work)
			if (err != nil) != tt.wantErr {
				t.Errorf("deleteKlusterletUpdateWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
