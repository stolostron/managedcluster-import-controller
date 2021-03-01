// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"os"
	"reflect"
	"testing"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	ocinfrav1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	/* #nosec */
	imagePullSecretNameSyncSet = "my-image-pul-secret-syncset"
	managedClusterNameSyncSet  = "cluster-syncseet"
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

func Test_newSyncSets(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSyncSet)
	os.Setenv("POD_NAMESPACE", managedClusterNameSyncSet)
	imagePullSecret := newFakeImagePullSecret()

	testInfraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "newsyncset",
		},
	}

	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.SyncSet{})
	testscheme.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	testSA := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "newsyncset" + bootstrapServiceAccountNamePostfix,
			Namespace: "newsyncset",
		},
	}

	tokenSecret, err := serviceAccountTokenSecret(testSA)
	if err != nil {
		t.Errorf("fail to initialize serviceaccount token secret, error = %v", err)
	}

	testSA.Secrets = append(testSA.Secrets, corev1.ObjectReference{
		Name: tokenSecret.Name,
	})

	testClient := fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
		testSA, tokenSecret, testInfraConfig, imagePullSecret,
	}...)

	type args struct {
		managedCluster *clusterv1.ManagedCluster
	}
	type syncsets struct {
		crds  *hivev1.SyncSet
		yamls *hivev1.SyncSet
	}
	tests := []struct {
		name    string
		args    args
		want    syncsets
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				managedCluster: nil,
			},
			want: syncsets{
				crds:  nil,
				yamls: nil,
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				managedCluster: testManagedCluster,
			},
			want: syncsets{
				crds: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "newsyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
						Namespace: "newsyncset",
					},
				},
				yamls: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "newsyncset" + syncsetNamePostfix,
						Namespace: "newsyncset",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			crds, yamls, err := generateImportYAMLs(testClient, tt.args.managedCluster, []string{})
			if (err != nil) != tt.wantErr {
				t.Errorf("generateImportYAMLs error=%v, wantErr %v", err, tt.wantErr)
			}
			gotCRDs, gotYAMLs, err := newSyncSets(testClient, tt.args.managedCluster, crds, yamls)
			if (err != nil) != tt.wantErr {
				t.Errorf("newSyncSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want.crds == nil {
				if gotCRDs != nil {
					t.Errorf("newSyncSets() = %v, want %v", gotCRDs, tt.want)
				}
			} else {
				if gotCRDs.GetNamespace() != tt.want.crds.GetNamespace() || gotCRDs.GetName() != tt.want.crds.GetName() {
					t.Errorf("newSyncSets() = %v, want %v", gotCRDs, tt.want.crds)
				}
			}
			if tt.want.yamls == nil {
				if gotYAMLs != nil {
					t.Errorf("newSyncSets() = %v, want %v", gotYAMLs, tt.want)
				}
			} else {
				if gotCRDs.GetNamespace() != tt.want.yamls.GetNamespace() || gotYAMLs.GetName() != tt.want.yamls.GetName() {
					t.Errorf("newSyncSets() = %v, want %v", gotYAMLs, tt.want.yamls)
				}
			}
		})
	}

}

func Test_createOrUpdateSyncSets(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSyncSet)
	os.Setenv("POD_NAMESPACE", managedClusterNameSyncSet)
	imagePullSecret := newFakeImagePullSecret()

	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "createsyncset",
		},
	}

	testScheme := scheme.Scheme

	testScheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.SyncSet{})
	testScheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testScheme.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	testInfraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	testSA := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createsyncset" + bootstrapServiceAccountNamePostfix,
			Namespace: "createsyncset",
		},
	}

	tokenSecret, err := serviceAccountTokenSecret(testSA)
	if err != nil {
		t.Errorf("fail to initialize serviceaccount token secret, error = %v", err)
	}

	testSA.Secrets = append(testSA.Secrets, corev1.ObjectReference{
		Name: tokenSecret.Name,
	})

	crds := &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createsyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
			Namespace: "createsyncset",
		},
	}
	yamls := &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createsyncset" + syncsetNamePostfix,
			Namespace: "createsyncset",
		},
	}

	crdsUpdate := &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createsyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
			Namespace: "createsyncset",
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{Name: "testclusterdeployment1"},
				{Name: "testclusterdeployment2"},
			},
		},
	}
	yamlsUpdate := &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createsyncset" + syncsetNamePostfix,
			Namespace: "createsyncset",
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{Name: "testclusterdeployment1"},
				{Name: "testclusterdeployment2"},
			},
		},
	}

	type args struct {
		client         client.Client
		managedCluster *clusterv1.ManagedCluster
	}

	type syncsets struct {
		crds  *hivev1.SyncSet
		yamls *hivev1.SyncSet
	}
	tests := []struct {
		name    string
		args    args
		want    syncsets
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, imagePullSecret,
				}...),
				managedCluster: nil,
			},
			want: syncsets{
				crds:  nil,
				yamls: nil,
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			want: syncsets{
				crds: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createsyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
						Namespace: "createsyncset",
					},
				},
				yamls: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createsyncset" + syncsetNamePostfix,
						Namespace: "createsyncset",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Update no change",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, crds, yamls, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			want: syncsets{
				crds: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createsyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
						Namespace: "createsyncset",
					},
				},
				yamls: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createsyncset" + syncsetNamePostfix,
						Namespace: "createsyncset",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Update with change",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, crdsUpdate, yamlsUpdate, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			want: syncsets{
				crds: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createsyncset" + syncsetNamePostfix + syncsetCRDSPostfix,
						Namespace: "createsyncset",
					},
				},
				yamls: &hivev1.SyncSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "SyncSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createsyncset" + syncsetNamePostfix,
						Namespace: "createsyncset",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			crds, yamls, err := generateImportYAMLs(tt.args.client, tt.args.managedCluster, []string{})
			if (err != nil) != tt.wantErr {
				t.Errorf("generateImportYAMLs error=%v, wantErr %v", err, tt.wantErr)
			}
			gotCRDs, gotYAMLs, err := createOrUpdateSyncSets(tt.args.client, testScheme, tt.args.managedCluster, crds, yamls)
			if (err != nil) != tt.wantErr {
				t.Errorf("createSyncSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want.crds == nil {
				if gotCRDs != nil {
					t.Errorf("createSyncSets() = %v, want %v", gotCRDs, tt.want)
				}
			} else {
				if gotCRDs.GetNamespace() != tt.want.crds.GetNamespace() || gotCRDs.GetName() != tt.want.crds.GetName() {
					t.Errorf("createSyncSets() = %v, want %v", gotCRDs, tt.want.crds)
				}
				if len(gotCRDs.Spec.ClusterDeploymentRefs) != 1 {
					t.Errorf("createSyncSets() crds not updated, got %d expect 1", len(gotCRDs.Spec.ClusterDeploymentRefs))
				}
			}
			if tt.want.yamls == nil {
				if gotYAMLs != nil {
					t.Errorf("createSyncSets() = %v, want %v", gotYAMLs, tt.want)
				}
			} else {
				if gotYAMLs.GetNamespace() != tt.want.yamls.GetNamespace() || gotYAMLs.GetName() != tt.want.yamls.GetName() {
					t.Errorf("createSyncSets() = %v, want %v", gotYAMLs, tt.want.yamls)
				}
				if len(gotYAMLs.Spec.ClusterDeploymentRefs) != 1 {
					t.Errorf("createSyncSets() yamls not updated, got %d expect 1", len(gotYAMLs.Spec.ClusterDeploymentRefs))
				}
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
			if err := deleteKlusterletSyncSets(tt.args.client, tt.args.managedCluster); (err != nil) != tt.wantErr {
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
