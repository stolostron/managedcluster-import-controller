// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"reflect"
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_syncSettNsN(t *testing.T) {
	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testmanagedcluster",
		},
	}

	type args struct {
		managedCluster *clusterv1.ManagedCluster
	}
	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name: "nil managedCluster",
			args: args{
				managedCluster: nil,
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				managedCluster: testManagedCluster,
			},
			want: types.NamespacedName{
				Name:      "testmanagedcluster" + syncsetNamePostfix,
				Namespace: "testmanagedcluster",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			got, err := syncSetNsN(tt.args.managedCluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("bootstrapServiceAccountNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bootstrapServiceAccountNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_deleteSyncSets(t *testing.T) {
	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deletesyncset",
		},
	}

	testScheme := scheme.Scheme

	testScheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.SyncSet{})
	testScheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})

	crds := &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deletesyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
			Namespace: "deletesyncset",
		},
	}
	yamls := &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deletesyncset" + syncsetNamePostfix,
			Namespace: "deletesyncset",
		},
	}
	type args struct {
		client         client.Client
		managedCluster *clusterv1.ManagedCluster
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					crds, yamls,
				}...),
				managedCluster: nil,
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					crds, yamls,
				}...),
				managedCluster: testManagedCluster,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Set upsert
			if _, err := deleteKlusterletSyncSets(tt.args.client, tt.args.managedCluster); (err != nil) != tt.wantErr {
				t.Errorf("deleteSyncSets() error = %v, wantErr %v", err, tt.wantErr)
			}
			//Delete syncset as upsert is set
			if _, err := deleteKlusterletSyncSets(tt.args.client, tt.args.managedCluster); (err != nil) != tt.wantErr {
				t.Errorf("deleteSyncSets() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				crds := &hivev1.SyncSet{}
				err := tt.args.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "deletesyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
						Namespace: "deletesyncset",
					}, crds)
				if err == nil {
					t.Error("deletesyncset crds manifest not deleted")
				}
				yamls := &hivev1.SyncSet{}
				err = tt.args.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "deletesyncset" + syncsetNamePostfix,
						Namespace: "deletesyncset",
					}, yamls)
				if err == nil {
					t.Error("deletesyncset yamls manifest not deleted")
				}
			}
		})
	}
}
