// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"io/ioutil"
	"reflect"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
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

func Test_getManagedClusterKubeVersion(t *testing.T) {
	envTest, _, _, _ := setupEnvTestByName("import_detach")
	type args struct {
		rConfig *rest.Config
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "GetVersion",
			args: args{
				rConfig: envTest.Config,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getManagedClusterKubeVersion(tt.args.rConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("getManagedClusterKubeVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == "" {
				t.Error("getManagedClusterKubeVersion() empty, want non-empty")
			}
		})
	}
}

func Test_getClientFromToken(t *testing.T) {
	envTest, _, _, _ := setupEnvTestByName("import_detach")

	type args struct {
		token  string
		server string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Success",
			args: args{
				token:  envTest.Config.BearerToken,
				server: envTest.Config.Host,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, config, err := getClientFromToken(tt.args.token, tt.args.server)
			if (err != nil) != tt.wantErr {
				t.Errorf("getClientFromToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if config.BearerToken != tt.args.token {
				t.Errorf("getClientFromToken() got token = %v, want %v", config.BearerToken, tt.args.token)
			}
			if config.Host != tt.args.server {
				t.Errorf("getClientFromToken() got server = %v, want %v", config.Host, tt.args.server)
			}
		})
	}
}

func Test_getClientFromKubeConfig(t *testing.T) {
	_, kubeConfigBasic, _, _ := setupEnvTestByName("import_detach")

	kubeconfig, err := ioutil.ReadFile(kubeConfigBasic)
	if err != nil {
		t.Error(err)
	}
	type args struct {
		kubeconfig []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Success",
			args: args{
				kubeconfig: kubeconfig,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, config, err := getClientFromKubeConfig(tt.args.kubeconfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("getClientFromKubeConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if config.Host == "" {
				t.Errorf("Expect to get a config.Host not empty")
			}
		})
	}
}

func TestReconcileManagedCluster_getManagedClusterClientFromAutoImportSecret(t *testing.T) {
	envTest, _, kubeConfigToken, _ := setupEnvTestByName("import_detach")
	kubeconfig, err := ioutil.ReadFile(kubeConfigToken)
	if err != nil {
		t.Error(err)
	}
	config, err := envTest.ControlPlane.RESTClientConfig()
	if err != nil {
		t.Error(err)
	}
	c, err := client.New(config, client.Options{})
	if err != nil {
		t.Error(err)
	}
	autoImportSecretKC := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfig,
		},
	}
	autoImportSecretToken := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"token":  []byte(envTest.Config.BearerToken),
			"server": []byte(envTest.ControlPlane.APIURL().String()),
		},
	}
	autoImportSecretNoToken := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"server": []byte(envTest.ControlPlane.APIURL().String()),
		},
	}
	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		autoImportSecret *corev1.Secret
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    client.Client
		want1   *rest.Config
		wantErr bool
	}{
		{
			name: "Kubeconfing",
			fields: fields{
				client: c,
			},
			args: args{
				autoImportSecret: autoImportSecretKC,
			},
			wantErr: false,
		},
		{
			name: "token",
			fields: fields{
				client: c,
			},
			args: args{
				autoImportSecret: autoImportSecretToken,
			},
			wantErr: false,
		},
		{
			name: "no token",
			fields: fields{
				client: c,
			},
			args: args{
				autoImportSecret: autoImportSecretNoToken,
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
			_, config, err := r.getManagedClusterClientFromAutoImportSecret(tt.args.autoImportSecret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileManagedCluster.getManagedClusterClientFromAutoImportSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if config.Host == "" {
					t.Errorf("Expect to get a config.Host not empty")
				}
			}
		})
	}
}

func TestReconcileManagedCluster_getManagedClusterClientFromHive(t *testing.T) {
	envTest, _, kubeConfigToken, _ := setupEnvTestByName("import_detach")
	kubeconfig, err := ioutil.ReadFile(kubeConfigToken)
	if err != nil {
		t.Error(err)
	}
	envTest.ControlPlane.KubeCtl().Run("create", "ns", "mycluster")
	config, err := envTest.ControlPlane.RESTClientConfig()
	if err != nil {
		t.Error(err)
	}
	c, err := client.New(config, client.Options{})
	if err != nil {
		t.Error(err)
	}
	autoImportSecretKC := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hive-secret",
			Namespace: "mycluster",
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfig,
		},
	}
	err = c.Create(context.TODO(), autoImportSecretKC)
	if err != nil {
		t.Error(err)
	}
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hive-cd",
			Namespace: "mycluster",
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{
					Name: "hive-secret",
				},
			},
		},
	}
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mycluster",
		},
	}
	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		clusterDeployment *hivev1.ClusterDeployment
		managedCluster    *clusterv1.ManagedCluster
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    client.Client
		want1   *rest.Config
		wantErr bool
	}{
		{
			name: "succeed",
			fields: fields{
				client: c,
			},
			args: args{
				clusterDeployment: cd,
				managedCluster:    mc,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileManagedCluster{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}
			_, config, err := r.getManagedClusterClientFromHive(tt.args.clusterDeployment, tt.args.managedCluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileManagedCluster.getManagedClusterClientFromHive() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if config.Host == "" {
					t.Errorf("Expect to get a config.Host not empty")
				}
			}
		})
	}
}

func TestReconcileManagedCluster_updateAutoImportRetry(t *testing.T) {
	envTest, _, kubeConfigToken, _ := setupEnvTestByName("import_detach")
	kubeconfig, err := ioutil.ReadFile(kubeConfigToken)
	if err != nil {
		t.Error(err)
	}
	envTest.ControlPlane.KubeCtl().Run("create", "ns", "mycluster")
	config, err := envTest.ControlPlane.RESTClientConfig()
	if err != nil {
		t.Error(err)
	}
	c, err := client.New(config, client.Options{})
	if err != nil {
		t.Error(err)
	}
	autoImportSecret0 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hive-secret0",
			Namespace: "mycluster",
		},
		Data: map[string][]byte{
			"kubeconfig":      kubeconfig,
			"autoImportRetry": []byte("0"),
		},
	}
	err = c.Create(context.TODO(), autoImportSecret0)
	if err != nil {
		t.Error(err)
	}
	autoImportSecret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hive-secret1",
			Namespace: "mycluster",
		},
		Data: map[string][]byte{
			"kubeconfig":      kubeconfig,
			"autoImportRetry": []byte("1"),
		},
	}
	err = c.Create(context.TODO(), autoImportSecret1)
	if err != nil {
		t.Error(err)
	}
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mycluster",
		},
	}
	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		managedCluster   *clusterv1.ManagedCluster
		autoImportSecret *corev1.Secret
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "update",
			fields: fields{
				client: c,
			},
			args: args{
				managedCluster:   mc,
				autoImportSecret: autoImportSecret1,
			},
			wantErr: false,
		},
		{
			name: "delete",
			fields: fields{
				client: c,
			},
			args: args{
				managedCluster:   mc,
				autoImportSecret: autoImportSecret0,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileManagedCluster{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
			}
			if err := r.updateAutoImportRetry(tt.args.managedCluster, tt.args.autoImportSecret); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileManagedCluster.updateAutoImportRetry() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				switch tt.name {
				case "update":
					secret := corev1.Secret{}
					err := c.Get(context.TODO(), client.ObjectKey{Name: "hive-secret1", Namespace: "mycluster"}, &secret)
					if err != nil {
						t.Error(err)
					}
					if string(secret.Data["autoImportRetry"]) != "0" {
						t.Errorf("expect autoImportRetry to be zero but got %s ", secret.StringData["autoImportRetry"])
					}
				case "delete":
					secret := corev1.Secret{}
					err := c.Get(context.TODO(), client.ObjectKey{Name: "hive-secret0", Namespace: "mycluster"}, &secret)
					if err == nil {
						t.Error("expect the auto-import-secret to be deleted")
					}
				}
			}
		})
	}
}
