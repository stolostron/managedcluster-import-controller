// Copyright (c) 2020 Red Hat, Inc.

package managedcluster

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	ocinfrav1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	imagePullSecretNameReconcile = "my-image-pul-secret"
	managedClusterNameReconcile  = "cluster-reconcile"
)

func TestReconcileManagedCluster_Reconcile(t *testing.T) {

	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameReconcile)
	os.Setenv("POD_NAMESPACE", managedClusterNameReconcile)

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
			Name: managedClusterNameReconcile,
		},
		Spec: clusterv1.ManagedClusterSpec{},
	}

	testManagedClusterHub := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: managedClusterNameReconcile,
			Labels: map[string]string{
				"local-cluster": "true",
			},
		},
		Spec: clusterv1.ManagedClusterSpec{},
	}

	testManagedClusterOnLine := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: managedClusterNameReconcile,
		},
		Spec: clusterv1.ManagedClusterSpec{},
		Status: clusterv1.ManagedClusterStatus{
			Conditions: []clusterv1.StatusCondition{
				{
					Type:   clusterv1.ManagedClusterConditionAvailable,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	testManagedClusterDeletion := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              managedClusterNameReconcile,
			DeletionTimestamp: &metav1.Time{time.Now()},
			Finalizers:        []string{managedClusterFinalizer},
		},
		Spec: clusterv1.ManagedClusterSpec{},
		Status: clusterv1.ManagedClusterStatus{
			Conditions: []clusterv1.StatusCondition{
				{
					Type:   clusterv1.ManagedClusterConditionAvailable,
					Status: metav1.ConditionUnknown,
				},
			},
		},
	}

	testManagedClusterDeletionNotOffLine := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              managedClusterNameReconcile,
			DeletionTimestamp: &metav1.Time{time.Now()},
			Finalizers:        []string{managedClusterFinalizer},
		},
		Spec: clusterv1.ManagedClusterSpec{},
		Status: clusterv1.ManagedClusterStatus{
			Conditions: []clusterv1.StatusCondition{
				{
					Type:   clusterv1.ManagedClusterConditionAvailable,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	testManagedClusterDeletionNoStatus := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              managedClusterNameReconcile,
			DeletionTimestamp: &metav1.Time{time.Now()},
			Finalizers:        []string{managedClusterFinalizer},
		},
		Spec: clusterv1.ManagedClusterSpec{
			HubAcceptsClient: true,
		},
	}

	clusterDeployment := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedClusterNameReconcile,
			Namespace: managedClusterNameReconcile,
		},
	}

	imagePullSecret := newFakeImagePullSecret()
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.SyncSet{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	testscheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWorkList{})
	testscheme.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	req := reconcile.Request{
		types.NamespacedName{
			Name: managedClusterNameReconcile,
		},
	}

	serviceAccount, err := newBootstrapServiceAccount(testManagedCluster)
	if err != nil {
		t.Errorf("fail to initialize bootstrap serviceaccount, error = %v", err)
	}

	tokenSecret, err := serviceAccountTokenSecret(serviceAccount)
	if err != nil {
		t.Errorf("fail to initialize serviceaccount token secret, error = %v", err)
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
			name: "success without clusterDeployment",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedCluster,
					tokenSecret,
					imagePullSecret,
					testInfraConfig,
				),
				scheme: testscheme,
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
			name: "success self import",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedClusterHub,
					tokenSecret,
					imagePullSecret,
					testInfraConfig,
				),
				scheme: testscheme,
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
			name: "success without clusterDeployment and online",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedClusterOnLine,
					tokenSecret,
					imagePullSecret,
					testInfraConfig,
				),
				scheme: testscheme,
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
			name: "success with clusterDeployment",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedCluster,
					tokenSecret,
					clusterDeployment,
					imagePullSecret,
					testInfraConfig,
				),
				scheme: testscheme,
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
			name: "Error missing imagePullSecret",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedCluster,
					tokenSecret,
					testInfraConfig,
				),
				scheme: testscheme,
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
			t.Logf("Test name: %s", tt.name)
			r := &ReconcileManagedCluster{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}
			var got reconcile.Result
			var err error
			i := 10
			for got, err = r.Reconcile(tt.args.request); err != nil && i != 0 &&
				(strings.Contains(err.Error(), imagePullSecretNameReconcile) ||
					strings.Contains(err.Error(), managedClusterNameReconcile+bootstrapServiceAccountNamePostfix)); i-- {
				t.Logf("Wait reconcile.... Error: %s adding secret to service account", err.Error())
				sa := &corev1.ServiceAccount{}
				errSA := r.client.Get(context.TODO(),
					types.NamespacedName{Name: testManagedCluster.Name + bootstrapServiceAccountNamePostfix,
						Namespace: testManagedCluster.Name},
					sa)
				if errSA != nil {
					t.Error(errSA)
				}
				sa.Secrets = append(serviceAccount.Secrets, corev1.ObjectReference{
					Name: tokenSecret.Name,
				})
				errSA = r.client.Update(context.TODO(), sa)
				if errSA != nil {
					t.Error(errSA)
				}
				time.Sleep(100 * time.Millisecond)
				got, err = r.Reconcile(tt.args.request)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileManagedCluster.Reconcile() Creation error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileManagedCluster.Reconcile() Creation= %v, want %v", got, tt.want)
			}
			if !tt.wantErr && !got.Requeue {
				managedCluster := &clusterv1.ManagedCluster{}
				err := r.client.Get(context.TODO(),
					types.NamespacedName{
						Name: testManagedCluster.Name,
					},
					managedCluster)
				if err != nil {
					t.Errorf("Managedcluster not found Error: %s", err.Error())
				}
				if len(managedCluster.Finalizers) != 1 {
					t.Error("No finalizer found in managedcluster")
				}
				if managedCluster.Finalizers[0] != managedClusterFinalizer {
					t.Errorf("Expects finalizer %s got %s ", managedClusterFinalizer, managedCluster.Finalizers[0])
				}
				importSecret := &corev1.Secret{}
				err = r.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      testManagedCluster.Name + importSecretNamePostfix,
						Namespace: testManagedCluster.Name,
					}, importSecret)
				if err != nil {
					t.Errorf("Import secret doesn't exists Error: %s", err.Error())
				}
				if _, ok := importSecret.Data[importYAMLKey]; !ok {
					t.Error("Import secret doesn't contains a place holder " + importYAMLKey)
				}
				if _, ok := importSecret.Data[crdsYAMLKey]; !ok {
					t.Error("Import secret doesn't contains a place holder " + crdsYAMLKey)
				}
				clusterDeploymentGet := &hivev1.ClusterDeployment{}
				err = r.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      clusterDeployment.Name,
						Namespace: clusterDeployment.Namespace,
					},
					clusterDeploymentGet)
				if err == nil {
					syncset := &hivev1.SyncSet{}
					err := r.client.Get(context.TODO(),
						types.NamespacedName{
							Name:      testManagedCluster.Name + syncsetNamePostfix,
							Namespace: testManagedCluster.Name,
						}, syncset)
					if err != nil {
						t.Errorf("SyncSet doesn't exist Error: %s", err.Error())
					}
				} else {
					manifestwork := &workv1.ManifestWork{}
					err := r.client.Get(context.TODO(),
						types.NamespacedName{
							Name:      testManagedCluster.Name + manifestWorkNamePostfix,
							Namespace: testManagedCluster.Name,
						}, manifestwork)
					if err == nil && checkOffLine(managedCluster) {
						t.Error("Manifestwork exist with a offline cluster")
					} else if err != nil && !checkOffLine(managedCluster) {
						t.Error("Manifestwork doesn't exist with an online cluster")
					}
				}
				if v, ok := managedCluster.GetLabels()["local-cluster"]; ok {
					b, err := strconv.ParseBool(v)
					if err != nil {
						t.Error(err)
					}
					if b {
						ns := &corev1.Namespace{}
						err := r.client.Get(context.TODO(), types.NamespacedName{
							Name: "open-cluster-management-agent",
						}, ns)
						if err != nil {
							t.Error(err)
						}
					}
				}
			}
		})
	}

	testsDeletion := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "Success deletion",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedClusterDeletion,
					tokenSecret,
					imagePullSecret,
				),
				scheme: testscheme,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "Error deletion not offline",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedClusterDeletionNotOffLine,
					tokenSecret,
					imagePullSecret,
				),
				scheme: testscheme,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 1 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "Success no status",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					testManagedClusterDeletionNoStatus,
					tokenSecret,
					imagePullSecret,
				),
				scheme: testscheme,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range testsDeletion {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			r := &ReconcileManagedCluster{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}
			got, err := r.Reconcile(tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileManagedCluster.Reconcile() Deletion error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileManagedCluster.Reconcile() Deletion = %v, want %v", got, tt.want)
			}
		})
	}

}

func newFakeImagePullSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
			Namespace: os.Getenv("POD_NAMESPACE"),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte("fake-token"),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}
}

func Test_checkOffLine(t *testing.T) {
	type args struct {
		managedCluster *clusterv1.ManagedCluster
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Online",
			args: args{
				managedCluster: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              managedClusterNameReconcile,
						DeletionTimestamp: &metav1.Time{time.Now()},
					},
					Spec: clusterv1.ManagedClusterSpec{},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []clusterv1.StatusCondition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "Offline",
			args: args{
				managedCluster: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              managedClusterNameReconcile,
						DeletionTimestamp: &metav1.Time{time.Now()},
					},
					Spec: clusterv1.ManagedClusterSpec{},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []clusterv1.StatusCondition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionFalse,
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "Offline",
			args: args{
				managedCluster: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              managedClusterNameReconcile,
						DeletionTimestamp: &metav1.Time{time.Now()},
					},
					Spec: clusterv1.ManagedClusterSpec{},
					Status: clusterv1.ManagedClusterStatus{
						Conditions: []clusterv1.StatusCondition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionUnknown,
							},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("name: %s", tt.name)
			if got := checkOffLine(tt.args.managedCluster); got != tt.want {
				t.Errorf("checkOffLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileManagedCluster_deleteNamespace(t *testing.T) {
	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	testscheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{})

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mycluster",
		},
	}

	now := metav1.NewTime(time.Now())

	nsDeletionTimestamp := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mycluster",
			DeletionTimestamp: &now,
		},
	}

	clusterDeployment := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mycluster",
			Namespace: "mycluster",
		},
	}

	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		namespaceName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "Namespace not exists",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					ns,
				),
				scheme: testscheme,
			},
			args: args{
				namespaceName: "wrongNamespace",
			},
			wantErr: false,
		},
		{
			name: "Namespace has deletionTimestamp",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					nsDeletionTimestamp,
				),
				scheme: testscheme,
			},
			args: args{
				namespaceName: "mycluster",
			},
			wantErr: false,
		},
		{
			name: "Namespace deleted without clusterDeployment",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					ns,
				),
				scheme: testscheme,
			},
			args: args{
				namespaceName: "mycluster",
			},
			wantErr: false,
		},
		{
			name: "Namespace deleted with clusterDeployment",
			fields: fields{
				client: fake.NewFakeClientWithScheme(testscheme,
					ns,
					clusterDeployment,
				),
				scheme: testscheme,
			},
			args: args{
				namespaceName: "mycluster",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileManagedCluster{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}
			if err := r.deleteNamespace(tt.args.namespaceName); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileManagedCluster.deleteNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
			gotNS := &corev1.Namespace{}
			err := tt.fields.client.Get(context.TODO(), types.NamespacedName{
				Name: tt.args.namespaceName,
			}, gotNS)
			if !tt.wantErr {
				switch tt.name {
				case "Namespace not exists", "Namespace deleted without clusterDeployment":
					if err != nil {
						if !errors.IsNotFound(err) {
							t.Errorf("ReconcileManagedCluster.deleteNamespace() got %s but wanted %s",
								errors.ReasonForError(err),
								metav1.StatusReasonNotFound)
						}
					} else {
						t.Errorf("ReconcileManagedCluster.deleteNamespace() %s namespace exits",
							tt.args.namespaceName)
					}
				case "Namespace has deletionTimestamp":
					if err != nil {
						if !errors.IsNotFound(err) {
							t.Errorf("ReconcileManagedCluster.deleteNamespace() got %s but wanted %s",
								errors.ReasonForError(err),
								metav1.StatusReasonNotFound)
						}
					}
				}
			} else {
				switch tt.name {
				case "Namespace deleted with clusterDeployment":
					if err != nil {
						if !errors.IsNotFound(err) {
							t.Errorf("ReconcileManagedCluster.deleteNamespace() got %s but wanted %s",
								errors.ReasonForError(err),
								metav1.StatusReasonNotFound)
						}
					}
				}
			}
		})
	}
}
