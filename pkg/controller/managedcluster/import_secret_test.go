// Copyright (c) 2020 Red Hat, Inc.

//Package managedcluster ...

package managedcluster

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ghodss/yaml"
	. "github.com/onsi/gomega"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/library-go/pkg/templateprocessor"
	"github.com/open-cluster-management/rcm-controller/pkg/bindata"
	ocinfrav1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	imagePullSecretNameSecret = "my-image-pul-secret-secret"
	managedClusterNameSecret  = "cluster-secret"
)

func init() {
	os.Setenv("KLUSTERLET_CRD_FILE", "../../../build/resources/agent.open-cluster-management.io_v1beta1_klusterlet_crd.yaml")
	os.Setenv(registrationOperatorImageEnvVarName, "quay.io/open-cluster-management/registration-operator:latest")
	os.Setenv(workImageEnvVarName, "quay.io/open-cluster-management/work:latest")
	os.Setenv(registrationImageEnvVarName, "quay.io/open-cluster-management/registration:latest")
}

func Test_importSecretNsN(t *testing.T) {
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
			name:    "nil ManagedCluster",
			args:    args{},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "no name",
			args: args{
				managedCluster: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{},
				},
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "no error",
			args: args{
				managedCluster: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			want: types.NamespacedName{
				Name:      "test" + importSecretNamePostfix,
				Namespace: "test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := importSecretNsN(tt.args.managedCluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("importSecretNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("importSecretNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newImportSecret(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSecret)
	os.Setenv("POD_NAMESPACE", managedClusterNameSecret)
	imagePullSecret := newFakeImagePullSecret()

	s := scheme.Scheme
	s.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	infraConfig := &ocinfrav1.Infrastructure{
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
			Name: "cluster-newimportsecret",
		},
		Spec: clusterv1.ManagedClusterSpec{},
	}

	managedClusterUnnamed := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       clusterv1.ManagedClusterSpec{},
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

	type args struct {
		client         client.Client
		scheme         *runtime.Scheme
		managedCluster *clusterv1.ManagedCluster
	}

	tests := []struct {
		name    string
		args    args
		wantNil bool
		wantErr bool
	}{
		{
			name: "nil scheme",
			args: args{
				client:         fake.NewFakeClient([]runtime.Object{}...),
				scheme:         nil,
				managedCluster: nil,
			},
			wantNil: true,
			wantErr: true,
		},
		{
			name: "nil managedCluster",
			args: args{
				client:         fake.NewFakeClientWithScheme(s, imagePullSecret),
				scheme:         s,
				managedCluster: nil,
			},
			wantNil: true,
			wantErr: true,
		},
		{
			name: "unanamed managedCluster",
			args: args{
				client:         fake.NewFakeClientWithScheme(s, managedClusterUnnamed, imagePullSecret),
				scheme:         s,
				managedCluster: nil,
			},
			wantNil: true,
			wantErr: true,
		},
		{
			name: "empty managedCluster",
			args: args{
				client:         fake.NewFakeClientWithScheme(s, imagePullSecret),
				scheme:         s,
				managedCluster: &clusterv1.ManagedCluster{},
			},
			wantNil: true,
			wantErr: true,
		},
		{
			name: "no error",
			args: args{
				client: fake.NewFakeClientWithScheme(s,
					managedCluster,
					serviceAccount,
					tokenSecret,
					infraConfig,
					imagePullSecret,
				),
				scheme:         s,
				managedCluster: managedCluster,
			},
			wantNil: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			got, err := newImportSecret(tt.args.client, tt.args.managedCluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("newImportSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if (got == nil) != tt.wantNil {
				t.Errorf("newImportSecret() = %v, want %v", got, tt.wantNil)
				return
			}
			if got != nil {
				t.Log("got is not nil")
				if got.Data == nil {
					t.Errorf("import secret data should not be empty")
					return
				}
				t.Log("got.Data is not nil")
				if _, ok := got.Data[importYAMLKey]; !ok {
					t.Error("Data " + importYAMLKey + " not found")
				}
				t.Log("got.Data[importYAMLKey] exists")
				if len(got.Data[importYAMLKey]) == 0 {
					t.Errorf(importYAMLKey + " should not be empty")
					return
				}
				if _, ok := got.Data[crdsYAMLKey]; !ok {
					t.Error("Data " + crdsYAMLKey + " not found")
				}
				t.Log("got.Data[crdsYAMLKey] exists")
				if len(got.Data[crdsYAMLKey]) == 0 {
					t.Errorf(crdsYAMLKey + " should not be empty")
					return
				}
			}
		})
	}
}

func Test_createOrUpdateImportSecret(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSecret)
	os.Setenv("POD_NAMESPACE", managedClusterNameSecret)
	imagePullSecret := newFakeImagePullSecret()

	infraConfig := &ocinfrav1.Infrastructure{
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
			Name: "cluster-createimportsecret",
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{URL: "url1"},
				{URL: "url2"},
			},
			HubAcceptsClient: true,
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

	s := scheme.Scheme
	s.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	s.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})

	fakeClient := fake.NewFakeClientWithScheme(s,
		managedCluster,
		serviceAccount,
		tokenSecret,
		infraConfig,
		imagePullSecret,
	)

	importSecret, err := newImportSecret(fakeClient, managedCluster)
	if err != nil {
		t.Errorf("fail to initialize import secret, error = %v", err)
	}

	fakeClientUpdate := fake.NewFakeClientWithScheme(s,
		managedCluster,
		serviceAccount,
		tokenSecret,
		infraConfig,
		imagePullSecret,
	)

	importSecretUpdate, err := newImportSecret(fakeClientUpdate, managedCluster)
	if err != nil {
		t.Errorf("fail to initialize import secret, error = %v", err)
	}
	delete(importSecretUpdate.Data, importYAMLKey)

	importSecret.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.SchemeGroupVersion.Group + "/" + clusterv1.SchemeGroupVersion.Version,
		Kind:       "ManagedCluster",
		Name:       "cluster-createimportsecret",
		UID:        "",
	}}

	type args struct {
		client         client.Client
		scheme         *runtime.Scheme
		managedCluster *clusterv1.ManagedCluster
	}

	tests := []struct {
		name    string
		args    args
		want    *corev1.Secret
		wantErr bool
	}{
		{
			name: "no error",
			args: args{
				client: fake.NewFakeClientWithScheme(s,
					managedCluster,
					serviceAccount,
					tokenSecret,
					imagePullSecret,
					infraConfig,
				),
				scheme:         s,
				managedCluster: managedCluster,
			},
			want:    importSecret,
			wantErr: false,
		},
		{
			name: "secret already exist",
			args: args{
				client: fake.NewFakeClientWithScheme(s,
					managedCluster,
					serviceAccount,
					tokenSecret,
					infraConfig,
					imagePullSecret,
					importSecret,
				),
				scheme:         s,
				managedCluster: managedCluster,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "secret already exist and update",
			args: args{
				client: fake.NewFakeClientWithScheme(s,
					managedCluster,
					serviceAccount,
					tokenSecret,
					infraConfig,
					imagePullSecret,
					importSecretUpdate,
				),
				scheme:         s,
				managedCluster: managedCluster,
			},
			want:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			got, err := createOrUpdateImportSecret(tt.args.client, tt.args.scheme, tt.args.managedCluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("createImportSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && got != nil {
				tt.want.ObjectMeta.ResourceVersion = got.ObjectMeta.ResourceVersion
				tt.want.ObjectMeta.OwnerReferences[0].Controller = got.ObjectMeta.OwnerReferences[0].Controller
				tt.want.ObjectMeta.OwnerReferences[0].BlockOwnerDeletion = got.ObjectMeta.OwnerReferences[0].BlockOwnerDeletion
			}
			if got != nil {
				if importYAML, ok := got.Data[importYAMLKey]; !ok {
					t.Error("Data " + importYAMLKey + " not found")
					if !reflect.DeepEqual(importYAML, tt.want.Data[importYAMLKey]) {
						t.Errorf("importYAML = %v, want %v", importYAML, tt.want.Data[importYAMLKey])
					}
				}
				if crdsYAML, ok := got.Data[crdsYAMLKey]; !ok {
					t.Error("Data " + crdsYAMLKey + "not found")
					if !reflect.DeepEqual(crdsYAML, tt.want.Data[crdsYAMLKey]) {
						t.Errorf("crdsYAML = %v, want %v", crdsYAML, tt.want.Data[crdsYAMLKey])
					}
				}
			}
		})
	}
}

func serviceAccountTokenSecret(serviceAccount *corev1.ServiceAccount) (*corev1.Secret, error) {
	if serviceAccount == nil {
		return nil, fmt.Errorf("serviceAccount can not be nil")
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount.GetName(),
			Namespace: serviceAccount.GetNamespace(),
		},
		Data: map[string][]byte{
			"token": []byte("fake-token"),
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}, nil
}

func TestTemplating(t *testing.T) {
	g := NewGomegaWithT(t)
	config := struct {
		KlusterletNamespace       string
		ClusterRoleName           string
		ServiceAccountName        string
		ManagedClusterName        string
		ClusterName               string
		BootstrapSecretToken      []byte
		BootstrapSecretCaCert     []byte
		HubKubeConfigSecretName   string
		HubKubeConfigSecret       string
		RegistrationOperatorImage string
	}{
		ClusterName:               "klusterlet",
		KlusterletNamespace:       "KlusterletNamespace",
		ClusterRoleName:           "ClusterRoleName",
		ServiceAccountName:        "ServiceAccountName",
		ManagedClusterName:        "ManagedClusterName",
		BootstrapSecretToken:      []byte("BootstrapSecretToken"),
		BootstrapSecretCaCert:     []byte("BootstrapSecretCaCert"),
		HubKubeConfigSecretName:   "HubKubeConfigSecretName",
		HubKubeConfigSecret:       "HubKubeConfigSecret",
		RegistrationOperatorImage: "RegistrationOperatorImage",
	}

	tp, err := templateprocessor.NewTemplateProcessor(bindata.NewBindataReader(), &templateprocessor.Options{})
	if err != nil {
		t.Error(err)
	}
	results, err := tp.TemplateAssets([]string{"klusterlet/cluster_role_binding.yaml",
		"klusterlet/operator.yaml"}, config)
	if err != nil {
		t.Error(err)
	}
	clusterRole := &rbacv1.ClusterRoleBinding{}
	t.Logf("ClusterRole %s", string(results[0]))
	err = yaml.Unmarshal(results[0], clusterRole)
	if err != nil {
		t.Errorf("Errorr %s %s", err.Error(), string(results[0]))
	}
	g.Expect(clusterRole.Subjects[0].Namespace).To(Equal("KlusterletNamespace"))

	deployment := &appsv1.Deployment{}
	t.Logf("Deployment %s", string(results[1]))
	err = yaml.Unmarshal(results[1], deployment)
	if err != nil {
		t.Errorf("Errorr %s %s", err.Error(), string(results[1]))
	}
	g.Expect(deployment.Namespace).Should(Equal("KlusterletNamespace"))
}

// newBootstrapServiceAccount initialize a new bootstrap serviceaccount
func newBootstrapServiceAccount(managedCluster *clusterv1.ManagedCluster) (*corev1.ServiceAccount, error) {
	saNsN, err := bootstrapServiceAccountNsN(managedCluster)
	if err != nil {
		return nil, err
	}

	config := struct {
		BootstrapServiceAccountName string
		ManagedClusterNamespace     string
	}{
		BootstrapServiceAccountName: saNsN.Name,
		ManagedClusterNamespace:     saNsN.Namespace,
	}
	tp, err := templateprocessor.NewTemplateProcessor(bindata.NewBindataReader(), &templateprocessor.Options{})
	if err != nil {
		return nil, err
	}
	result, err := tp.TemplateAsset("hub/managedcluster/manifests/managedcluster-service-account.yaml", config)
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{}
	err = yaml.Unmarshal(result, sa)
	if err != nil {
		return nil, err
	}

	return sa, nil
}
