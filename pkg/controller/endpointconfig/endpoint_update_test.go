// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
// Copyright (c) 2020 Red Hat, Inc.

package endpointconfig

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func Test_getEndpointUpdateWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})
	testscheme.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})

	testEndpointUpdateWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-update-endpoint",
			Namespace: "test-cluster",
		},
	}

	endpointConf := &multicloudv1alpha1.EndpointConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: multicloudv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Endpointconfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-cluster",
		},
	}
	type args struct {
		r              *ReconcileEndpointConfig
		endpointconfig *multicloudv1alpha1.EndpointConfig
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
				r: &ReconcileEndpointConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						endpointConf, testEndpointUpdateWork,
					}...),
					scheme: testscheme,
				},
				endpointconfig: endpointConf,
			},
			want:    testEndpointUpdateWork,
			wantErr: false,
		},
		{
			name: "endpoint update work does not exists",
			args: args{
				r: &ReconcileEndpointConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						endpointConf,
					}...),
					scheme: testscheme,
				},
				endpointconfig: endpointConf,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getEndpointUpdateWork(tt.args.r, tt.args.endpointconfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("getEndpointUpdateWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil {
				if got.Namespace != tt.want.GetNamespace() || got.Name != tt.want.GetName() {
					t.Errorf("getEndpointUpdateWork() = %v, want = %v", got, tt.want)
				}
			}
		})
	}
}

func Test_createEndpointUpdateWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})
	testscheme.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})
	testscheme.AddKnownTypes(multicloudv1beta1.SchemeGroupVersion, &multicloudv1beta1.Endpoint{})

	endpointConf := &multicloudv1alpha1.EndpointConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: multicloudv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Endpointconfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-cluster",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName: "cluster-name",
		},
	}

	testEndpointUpdateWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointConf.Name + "-update-endpoint",
			Namespace: "test-cluster",
		},
	}

	testEndpoint := &multicloudv1beta1.Endpoint{
		TypeMeta: metav1.TypeMeta{
			APIVersion: multicloudv1beta1.SchemeGroupVersion.String(),
			Kind:       "Endpoint",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoint",
			Namespace: "multicluster-endpoint",
		},
		Spec: multicloudv1beta1.EndpointSpec{
			ClusterName: "cluster-name",
		},
	}

	type args struct {
		r              *ReconcileEndpointConfig
		endpointconfig *multicloudv1alpha1.EndpointConfig
		endpoint       *multicloudv1beta1.Endpoint
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
				r: &ReconcileEndpointConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						endpointConf, testEndpoint,
					}...),
					scheme: testscheme,
				},
				endpointconfig: endpointConf,
				endpoint:       testEndpoint,
			},
			want:    testEndpointUpdateWork,
			wantErr: false,
		},
		{
			name: "update endpoint work already exists",
			args: args{
				r: &ReconcileEndpointConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						endpointConf, testEndpoint, testEndpointUpdateWork,
					}...),
					scheme: testscheme,
				},
				endpointconfig: endpointConf,
				endpoint:       testEndpoint,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createEndpointUpdateWork(tt.args.r, tt.args.endpointconfig, tt.args.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("createEndpointUpdateWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func Test_deleteEndpointUpdateWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})

	testEndpointUpdateWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-update-endpoint",
			Namespace: "test-cluster",
		},
	}

	type args struct {
		r    *ReconcileEndpointConfig
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
				r: &ReconcileEndpointConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testEndpointUpdateWork,
					}...),
					scheme: testscheme,
				},
				work: testEndpointUpdateWork,
			},
			wantErr: false,
		},
		{
			name: "update endpoint work not exists",
			args: args{
				r: &ReconcileEndpointConfig{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{}...),
					scheme: testscheme,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := deleteEndpointUpdateWork(tt.args.r, tt.args.work)
			if (err != nil) != tt.wantErr {
				t.Errorf("createEndpointUpdateWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
