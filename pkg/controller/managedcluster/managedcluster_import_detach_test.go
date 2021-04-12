// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"reflect"
	"testing"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	operatorv1 "github.com/open-cluster-management/api/operator/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileManagedCluster_importClusterWithClient(t *testing.T) {
	schemeHub := scheme.Scheme

	schemeHub.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	schemeHub.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
	schemeHub.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	schemeHub.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	schemeManaged := scheme.Scheme

	schemeManaged.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{}, &corev1.Namespace{}, &corev1.ServiceAccount{})
	schemeManaged.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.Deployment{})
	schemeManaged.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{}, &rbacv1.ClusterRoleBinding{})
	schemeManaged.AddKnownTypes(operatorv1.SchemeGroupVersion, &operatorv1.Klusterlet{})

	clusterNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mc",
		},
	}

	testInfraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mc",
		},
	}

	serviceAccount, err := newBootstrapServiceAccount(managedCluster)
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

	imagePullSecret := newFakeImagePullSecret()

	autoImportSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoImportSecretName,
			Namespace: "mc",
		},
		Data: map[string][]byte{},
	}
	autoImportSecret.Data[autoImportRetryName] = []byte("5")

	clientHubNoSecret := fake.NewFakeClientWithScheme(schemeHub,
		clusterNamespace,
		tokenSecret,
		imagePullSecret,
		testInfraConfig,
		managedCluster,
		serviceAccount)
	clientHubWithSecret := fake.NewFakeClientWithScheme(schemeHub,
		clusterNamespace,
		tokenSecret,
		imagePullSecret,
		testInfraConfig,
		managedCluster,
		serviceAccount,
		autoImportSecret)
	clientManaged := fake.NewFakeClientWithScheme(schemeHub)

	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		managedCluster            *clusterv1.ManagedCluster
		autoImportSecret          *corev1.Secret
		managedClusterClient      client.Client
		managedClusterKubeVersion string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "No-autoImportSecret",
			fields: fields{
				client: clientHubNoSecret,
				scheme: schemeHub,
			},
			args: args{
				managedCluster:            managedCluster,
				managedClusterClient:      clientManaged,
				managedClusterKubeVersion: "v1.15.0",
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "With-autoImportSecret",
			fields: fields{
				client: clientHubWithSecret,
				scheme: schemeHub,
			},
			args: args{
				managedCluster:            managedCluster,
				autoImportSecret:          autoImportSecret,
				managedClusterClient:      clientManaged,
				managedClusterKubeVersion: "v1.15.0",
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileManagedCluster{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}
			got, errTest := r.importClusterWithClient(
				tt.args.managedCluster,
				tt.args.autoImportSecret,
				tt.args.managedClusterClient,
				tt.args.managedClusterKubeVersion)
			if (errTest != nil) != tt.wantErr {
				t.Errorf("ReconcileManagedCluster.importClusterWithClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileManagedCluster.importClusterWithClient() = %v, want %v", got, tt.want)
			}
			ais := &corev1.Secret{}
			err = r.client.Get(context.TODO(), client.ObjectKey{Name: autoImportSecretName, Namespace: tt.args.managedCluster.Name}, ais)
			if err == nil {
				t.Errorf("The autoImportSecret is not deleted: %s", autoImportSecretName)
			}
			if errTest == nil {
				bs := &corev1.Secret{}
				err = tt.args.managedClusterClient.Get(context.TODO(), client.ObjectKey{Name: "bootstrap-hub-kubeconfig", Namespace: klusterletNamespace}, bs)
				if err != nil {
					t.Errorf("Boostrapsecret not found")
				}
				crb := &rbacv1.ClusterRoleBinding{}
				err = tt.args.managedClusterClient.Get(context.TODO(), client.ObjectKey{Name: "klusterlet"}, crb)
				if err != nil {
					t.Errorf("ClusterRoleBiding klusterlet not found")
				}
				cr := &rbacv1.ClusterRole{}
				err = tt.args.managedClusterClient.Get(context.TODO(), client.ObjectKey{Name: "klusterlet"}, cr)
				if err != nil {
					t.Errorf("ClusterRole klusterlet not found")
				}
				cra := &rbacv1.ClusterRole{}
				err = tt.args.managedClusterClient.Get(context.TODO(), client.ObjectKey{Name: "open-cluster-management:klusterlet-admin-aggregate-clusterrole"}, cra)
				if err != nil {
					t.Errorf("ClusterRole open-cluster-management:klusterlet-admin-aggregate-clusterrole not found")
				}
				k := &operatorv1.Klusterlet{}
				err = tt.args.managedClusterClient.Get(context.TODO(), client.ObjectKey{Name: "klusterlet"}, k)
				if err != nil {
					t.Errorf("klusterlet not found")
				}
				op := &appsv1.Deployment{}
				err = tt.args.managedClusterClient.Get(context.TODO(), client.ObjectKey{Name: "klusterlet", Namespace: klusterletNamespace}, op)
				if err != nil {
					t.Errorf("klusterlet operator not found")
				}
				sa := &corev1.ServiceAccount{}
				err = tt.args.managedClusterClient.Get(context.TODO(), client.ObjectKey{Name: "klusterlet", Namespace: klusterletNamespace}, sa)
				if err != nil {
					t.Errorf("klusterlet serviceaccount not found")
				}
			}
		})
	}
}
