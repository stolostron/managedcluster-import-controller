// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
// Copyright (c) 2020 Red Hat, Inc.

package clusterregistry

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func Test_getEndpointDeleteWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})

	testDeleteWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      EndpointDeleteWork,
			Namespace: "test-cluster",
		},
	}

	testclient := fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
		testDeleteWork,
	}...)

	type args struct {
		r       *ReconcileCluster
		cluster *clusterregistryv1alpha1.Cluster
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
				r: &ReconcileCluster{
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
			want:    testDeleteWork,
			wantErr: false,
		},
		{
			name: "delete work does not exists",
			args: args{
				r: &ReconcileCluster{
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
			got, err := getDeleteWork(tt.args.r, tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDeleteWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil {
				if got.Namespace != tt.want.GetNamespace() || got.Name != tt.want.GetName() {
					t.Errorf("getDeleteWork() = %v, want = %v", got, tt.want)
				}
			}
		})
	}
}

func Test_IsClusterOnline(t *testing.T) {
	testClusterOnline := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
			Finalizers: []string{
				"propagator.finalizer.mcm.ibm.com",
			},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Status: corev1.ConditionTrue,
					Type:   clusterregistryv1alpha1.ClusterOK,
				},
			},
		},
	}

	testClusterOffline := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
			Finalizers: []string{
				"propagator.finalizer.mcm.ibm.com",
			},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Type: "Offline",
				},
			},
		},
	}

	testClusterPending := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
			Finalizers: []string{
				"propagator.finalizer.mcm.ibm.com",
			},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Type: "Pending",
				},
			},
		},
	}

	tests := []struct {
		name     string
		cluster  *clusterregistryv1alpha1.Cluster
		Expected bool
	}{
		{"online cluster", testClusterOnline, true},
		{"offline cluster", testClusterOffline, false},
		{"pending cluster", testClusterPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := IsClusterOnline(tt.cluster)
			assert.Equal(t, actual, tt.Expected)
		})
	}
}

func Test_createDeleteWork(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, &mcmv1alpha1.Work{})
	testscheme.AddKnownTypes(multicloudv1alpha1.SchemeGroupVersion, &multicloudv1alpha1.EndpointConfig{})

	testcluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-cluster",
		},
	}

	testDeleteWork := &mcmv1alpha1.Work{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Work",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      EndpointDeleteWork,
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
		r              *ReconcileCluster
		cluster        *clusterregistryv1alpha1.Cluster
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
				r: &ReconcileCluster{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testcluster, endpointConf,
					}...),
					scheme: testscheme,
				},
				cluster:        testcluster,
				endpointconfig: endpointConf,
			},
			want:    testDeleteWork,
			wantErr: false,
		},
		{
			name: "delete work already exists",
			args: args{
				r: &ReconcileCluster{
					client: fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
						testcluster, endpointConf, testDeleteWork,
					}...),
					scheme: testscheme,
				},
				cluster:        testcluster,
				endpointconfig: endpointConf,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createDeleteWork(tt.args.r, tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDeleteWork() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
