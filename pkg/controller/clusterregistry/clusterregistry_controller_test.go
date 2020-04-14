// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
// Copyright (c) 2020 Red Hat, Inc.

package clusterregistry

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
)

func TestReconcileClusterregistry_Reconcile(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, &clusterregistryv1alpha1.Cluster{})

	clusterOnline := &clusterregistryv1alpha1.Cluster{
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

	clusterDeleting := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
			Finalizers: []string{
				"propagator.finalizer.mcm.ibm.com",
				"rcm-controller.cluster",
			},
			DeletionTimestamp: &metav1.Time{time.Now()},
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

	clusterOffline := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name",
			Namespace: "cluster-namespace",
			Finalizers: []string{
				"propagator.finalizer.mcm.ibm.com",
				"rcm-controller.cluster",
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

	clusterPending := &clusterregistryv1alpha1.Cluster{
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
			name: "cluster does not exist",
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
			name: "add finalizer when cluster is online",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, clusterOnline),
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
			name: "remove finalizer when cluster is offline",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, clusterOffline),
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
			name: "do nothing when cluster is pending",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, clusterPending),
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
			name: "cluster deleting",
			fields: fields{
				client: fake.NewFakeClientWithScheme(s, clusterDeleting),
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
			r := &ReconcileCluster{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}

			got, err := r.Reconcile(tt.args.request)

			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileCluster.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileCluster.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}
