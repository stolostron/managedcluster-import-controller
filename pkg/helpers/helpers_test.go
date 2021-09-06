// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	operatorfake "github.com/open-cluster-management/api/client/operator/clientset/versioned/fake"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	operatorv1 "github.com/open-cluster-management/api/operator/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	testinghelpers "github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers/testing"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crdv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/diff"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	testscheme.AddKnownTypes(operatorv1.SchemeGroupVersion, &operatorv1.Klusterlet{})
	testscheme.AddKnownTypes(crdv1beta1.SchemeGroupVersion, &crdv1beta1.CustomResourceDefinition{})
	testscheme.AddKnownTypes(crdv1.SchemeGroupVersion, &crdv1.CustomResourceDefinition{})
}

func TestGetMaxConcurrentReconciles(t *testing.T) {
	os.Setenv(maxConcurrentReconcilesEnvVarName, "invalid")
	defer os.Unsetenv(maxConcurrentReconcilesEnvVarName)

	reconciles := GetMaxConcurrentReconciles()
	if reconciles != 1 {
		t.Errorf("expected 1, but failed")
	}
}

func TestGenerateClientFromSecret(t *testing.T) {
	apiServer := &envtest.Environment{}
	config, err := apiServer.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer apiServer.Stop()

	cases := []struct {
		name           string
		generateSecret func(server string, config *rest.Config) *corev1.Secret
		expectedErr    string
	}{
		{
			name: "no client config",
			generateSecret: func(server string, config *rest.Config) *corev1.Secret {
				return &corev1.Secret{
					Data: map[string][]byte{
						"test": {},
					},
				}
			},
			expectedErr: "kubeconfig or token and server are missing",
		},
		{
			name: "using kubeconfig",
			generateSecret: func(server string, config *rest.Config) *corev1.Secret {
				apiConfig := createBasic(server, "test", config.Username, config.CAData)
				bconfig, err := clientcmd.Write(*apiConfig)
				if err != nil {
					t.Fatal(err)
				}
				return &corev1.Secret{
					Data: map[string][]byte{
						"kubeconfig": bconfig,
					},
				}
			},
		},
		{
			name: "using token",
			generateSecret: func(server string, config *rest.Config) *corev1.Secret {
				return &corev1.Secret{
					Data: map[string][]byte{
						"token":  []byte(config.BearerToken),
						"server": []byte(server),
					},
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			secret := c.generateSecret(config.Host, config)
			_, _, err = GenerateClientFromSecret(secret)
			if c.expectedErr != "" && err == nil {
				t.Errorf("expected error, but failed")
			}
			if c.expectedErr == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpdateManagedClusterStatus(t *testing.T) {
	cases := []struct {
		name           string
		managedCluster *clusterv1.ManagedCluster
		cond           metav1.Condition
	}{
		{
			name: "add condition",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    "test",
				Status:  metav1.ConditionTrue,
				Message: "test",
				Reason:  "test",
			},
		},
		{
			name: "update an existing condition",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "test",
							Status:  metav1.ConditionTrue,
							Message: "test",
							Reason:  "test",
						},
					},
				},
			},
			cond: metav1.Condition{
				Type:    "test",
				Status:  metav1.ConditionTrue,
				Message: "test",
				Reason:  "test",
			},
		},
		{
			name: "update condition",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "test",
							Status:  metav1.ConditionTrue,
							Message: "test",
							Reason:  "test",
						},
					},
				},
			},
			cond: metav1.Condition{
				Type:    "test",
				Status:  metav1.ConditionFalse,
				Message: "test",
				Reason:  "test",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(testscheme, c.managedCluster)

			err := UpdateManagedClusterStatus(fakeClient, eventstesting.NewTestingEventRecorder(t), c.managedCluster.Name, c.cond)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

}

func TestAddManagedClusterFinalizer(t *testing.T) {
	cases := []struct {
		name               string
		managedCluster     *clusterv1.ManagedCluster
		finalizer          string
		expectedFinalizers []string
	}{
		{
			name: "Add a finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{"test"},
		},
		{
			name: "Add an existent finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test"},
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{"test"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			modified := resourcemerge.BoolPtr(false)
			AddManagedClusterFinalizer(modified, c.managedCluster, c.finalizer)
			assertFinalizers(t, c.managedCluster, c.expectedFinalizers)
		})
	}
}

func TestRemoveManagedClusterFinalizer(t *testing.T) {
	cases := []struct {
		name               string
		managedCluster     *clusterv1.ManagedCluster
		finalizer          string
		expectedFinalizers []string
	}{
		{
			name: "Remove a finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test1", "test2"},
				},
			},
			finalizer:          "test2",
			expectedFinalizers: []string{"test1"},
		},
		{
			name: "Empty finalizers",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test"},
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{},
		},
		{
			name: "Remove a nonexistent finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test1"},
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{"test1"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(testscheme, c.managedCluster)

			managedCluster := &clusterv1.ManagedCluster{}
			if err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: c.managedCluster.Name}, managedCluster); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			err := RemoveManagedClusterFinalizer(fakeClient, eventstesting.NewTestingEventRecorder(t), managedCluster, c.finalizer)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			updatedManagedCluster := &clusterv1.ManagedCluster{}
			if err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: c.managedCluster.Name}, updatedManagedCluster); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			assertFinalizers(t, updatedManagedCluster, c.expectedFinalizers)
		})
	}
}

func TestApplyResources(t *testing.T) {
	var replicas int32 = 2

	cases := []struct {
		name           string
		kubeObjs       []runtime.Object
		klusterletObjs []runtime.Object
		clientObjs     []runtime.Object
		crds           []runtime.Object
		requiredObjs   []runtime.Object
		owner          *clusterv1.ManagedCluster
	}{
		{
			name:           "create resources",
			kubeObjs:       []runtime.Object{},
			klusterletObjs: []runtime.Object{},
			clientObjs:     []runtime.Object{},
			crds:           []runtime.Object{},
			requiredObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
			},
			owner: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
		},
		{
			name: "update resources",
			kubeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Subjects: []rbacv1.Subject{
						{
							Name: "test1",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Name: "test1",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
			},
			crds: []runtime.Object{
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
			},
			klusterletObjs: []runtime.Object{
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
			},
			clientObjs: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
			},
			requiredObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Rules: []rbacv1.PolicyRule{
						{
							Resources: []string{"test"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Subjects: []rbacv1.Subject{
						{
							Name: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Name: "test",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Data: map[string][]byte{
						"test": []byte("test"),
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
					},
				},
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: operatorv1.KlusterletSpec{
						Namespace: "test",
					},
				},
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: crdv1beta1.CustomResourceDefinitionSpec{
						Version: "test",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: crdv1.CustomResourceDefinitionSpec{
						PreserveUnknownFields: true,
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Spec: workv1.ManifestWorkSpec{
						Workload: workv1.ManifestsTemplate{
							Manifests: []workv1.Manifest{
								{
									RawExtension: runtime.RawExtension{Raw: []byte("{\"test\":\"test1\"}")},
								},
							},
						},
					},
				},
			},
			owner: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clientHolder := &ClientHolder{
				KubeClient:          kubefake.NewSimpleClientset(c.kubeObjs...),
				APIExtensionsClient: apiextensionsfake.NewSimpleClientset(c.crds...),
				OperatorClient:      operatorfake.NewSimpleClientset(c.klusterletObjs...),
				RuntimeClient:       fake.NewFakeClientWithScheme(testscheme, c.clientObjs...),
			}
			err := ApplyResources(clientHolder, eventstesting.NewTestingEventRecorder(t), testscheme, c.owner, c.requiredObjs...)
			if err != nil {
				t.Errorf("unexpect err %v", err)
			}
		})
	}
}

var tb = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: klusterlet
  namespace: "{{ .KlusterletNamespace }}"
{{- if .UseImagePullSecret }}
imagePullSecrets:
- name: "{{ .ImagePullSecretName }}"
{{- end}}
`

func TestAssetFromTemplate(t *testing.T) {
	cases := []struct {
		name     string
		config   interface{}
		validate func(t *testing.T, raw []byte)
	}{
		{
			name: "without ImagePullSecret",
			config: struct {
				KlusterletNamespace string
				UseImagePullSecret  bool
				ImagePullSecretName string
			}{
				KlusterletNamespace: "test",
			},
			validate: func(t *testing.T, raw []byte) {
				_, _, err := genericCodec.Decode(raw, nil, nil)
				if err != nil {
					t.Errorf("unexpect err %v, %v", string(raw), err)
				}
			},
		},
		{
			name: "with ImagePullSecret",
			config: struct {
				KlusterletNamespace string
				UseImagePullSecret  bool
				ImagePullSecretName string
			}{
				KlusterletNamespace: "test",
				UseImagePullSecret:  true,
				ImagePullSecretName: "test",
			},
			validate: func(t *testing.T, raw []byte) {
				_, _, err := genericCodec.Decode(raw, nil, nil)
				if err != nil {
					t.Errorf("unexpect err %v, %v", string(raw), err)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.validate(t, MustCreateAssetFromTemplate("test", []byte(tb), c.config))
		})
	}
}

func TestImportManagedClusterFromSecret(t *testing.T) {
	cases := []struct {
		name              string
		apiGroupResources []*restmapper.APIGroupResources
	}{
		{
			name: "only have crdv1beta1",
			apiGroupResources: []*restmapper.APIGroupResources{
				{
					Group: metav1.APIGroup{
						Name: "apiextensions.k8s.io",
						Versions: []metav1.GroupVersionForDiscovery{
							{Version: "v1beta1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1beta1"},
					},
					VersionedResources: map[string][]metav1.APIResource{
						"v1beta1": {
							{Name: "customresourcedefinitions", Namespaced: false, Kind: "CustomResourceDefinition"},
						},
					},
				},
			},
		},
		{
			name: "have crdv1beta1 and crdv1",
			apiGroupResources: []*restmapper.APIGroupResources{
				{
					Group: metav1.APIGroup{
						Name: "apiextensions.k8s.io",
						Versions: []metav1.GroupVersionForDiscovery{
							{Version: "v1beta1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1beta1"},
					},
					VersionedResources: map[string][]metav1.APIResource{
						"v1beta1": {
							{Name: "customresourcedefinitions", Namespaced: false, Kind: "CustomResourceDefinition"},
						},
					},
				},
				{
					Group: metav1.APIGroup{
						Name: "apiextensions.k8s.io",
						Versions: []metav1.GroupVersionForDiscovery{
							{Version: "v1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1"},
					},
					VersionedResources: map[string][]metav1.APIResource{
						"v1": {
							{Name: "customresourcedefinitions", Namespaced: false, Kind: "CustomResourceDefinition"},
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mapper := restmapper.NewDiscoveryRESTMapper(c.apiGroupResources)
			fakeRecorder := eventstesting.NewTestingEventRecorder(t)
			importSecret := testinghelpers.GetImportSecret("test_cluster")
			clientHolder := &ClientHolder{
				KubeClient:          kubefake.NewSimpleClientset(),
				APIExtensionsClient: apiextensionsfake.NewSimpleClientset(),
				OperatorClient:      operatorfake.NewSimpleClientset(),
				RuntimeClient:       fake.NewFakeClientWithScheme(testscheme),
			}
			err := ImportManagedClusterFromSecret(clientHolder, mapper, fakeRecorder, importSecret)
			if err != nil {
				t.Errorf("unexpect err %v", err)
			}
		})
	}
}

func assertFinalizers(t *testing.T, obj runtime.Object, finalizers []string) {
	accessor, _ := meta.Accessor(obj)
	actual := accessor.GetFinalizers()
	if len(actual) == 0 && len(finalizers) == 0 {
		return
	}
	if !reflect.DeepEqual(actual, finalizers) {
		t.Error(diff.ObjectDiff(actual, finalizers))
	}
}

func createBasic(serverURL, clusterName, userName string, caCert []byte) *clientcmdapi.Config {
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)
	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   serverURL,
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{},
		CurrentContext: contextName,
	}
}
