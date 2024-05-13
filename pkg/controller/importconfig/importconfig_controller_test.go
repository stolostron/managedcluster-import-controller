// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	fakeklusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/clientset/versioned/fake"
	klusterletconfiginformerv1alpha1 "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/informers/externalversions/klusterletconfig/v1alpha1"
	listerklusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/listers/klusterletconfig/v1alpha1"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &configv1.Infrastructure{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &configv1.APIServer{})

	os.Setenv(constants.RegistrationOperatorImageEnvVarName, "quay.io/open-cluster-management/registration-operator:latest")
	os.Setenv(constants.WorkImageEnvVarName, "quay.io/open-cluster-management/work:latest")
	os.Setenv(constants.RegistrationImageEnvVarName, "quay.io/open-cluster-management/registration:latest")
	os.Setenv(constants.DefaultImagePullSecretEnvVarName, "test-image-pull-secret-secret")
	os.Setenv(constants.PodNamespaceEnvVarName, "cluster-secret")
}

func TestReconcile(t *testing.T) {
	rootCACertData, rootCAKeyData, err := testinghelpers.NewRootCA("test root ca")
	if err != nil {
		t.Errorf("failed to create root ca: %v", err)
	}

	proxyServerCertData, _, err := testinghelpers.NewServerCertificate("proxy server", rootCACertData, rootCAKeyData)
	if err != nil {
		t.Errorf("failed to create default server cert: %v", err)
	}

	testKlusterletNamespace := "open-cluster-management-agent-test"
	cases := []struct {
		name             string
		clientObjs       []runtimeclient.Object
		runtimeObjs      []runtime.Object
		klusterletconfig *klusterletconfigv1alpha1.KlusterletConfig
		request          reconcile.Request
		validateFunc     func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface)
	}{
		{
			name:        "no clusters",
			clientObjs:  []runtimeclient.Object{},
			runtimeObjs: []runtime.Object{},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				// do nothing
			},
		},
		{
			name: "prepare cluster",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if data, ok := importSecret.Data[constants.ImportSecretCRDSYamlKey]; !ok || len(data) == 0 {
					t.Errorf("the %s is required", constants.ImportSecretCRDSYamlKey)
				}

				if data, ok := importSecret.Data[constants.ImportSecretCRDSV1beta1YamlKey]; !ok || len(data) == 0 {
					t.Errorf("the %s is required", constants.ImportSecretCRDSV1beta1YamlKey)
				}

				if data, ok := importSecret.Data[constants.ImportSecretCRDSV1YamlKey]; !ok || len(data) == 0 {
					t.Errorf("the %s is required", constants.ImportSecretCRDSV1YamlKey)
				}

				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					if len(objs) < 1 {
						t.Errorf("import secret data %s, objs is empty: %v", constants.ImportSecretImportYamlKey, objs)
					}
					ns, ok := objs[0].(*corev1.Namespace)
					if !ok {
						t.Errorf("import secret data %s, the first element is not namespace", constants.ImportSecretImportYamlKey)
					}
					if ns.Name != constants.DefaultKlusterletNamespace {
						t.Errorf("import secret data %s, the namespace name %s is not %s",
							constants.ImportSecretImportYamlKey, ns.Name, constants.DefaultKlusterletNamespace)
					}
					pullSecret, ok := objs[9].(*corev1.Secret)
					if !ok {
						t.Errorf("import secret data %s, the last element is not secret", constants.ImportSecretImportYamlKey)
					}
					if pullSecret.Type != corev1.SecretTypeDockerConfigJson {
						t.Errorf("import secret data %s, the pull secret type %s is not %s", constants.ImportSecretImportYamlKey,
							pullSecret.Type, corev1.SecretTypeDockerConfigJson)
					}
					if _, ok := pullSecret.Data[corev1.DockerConfigJsonKey]; !ok {
						t.Errorf("import secret data %s, the pull secret data %s is not %s", constants.ImportSecretImportYamlKey,
							pullSecret.Data, corev1.DockerConfigJsonKey)
					}
				}

				if len(strings.Split(strings.Replace(string(data), constants.YamlSperator, "", 1), constants.YamlSperator)) != 10 {
					t.Errorf("expect 10 files, but failed")
				}
			},
		},
		{
			name: "prepare secret (hosted mode)",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
						},
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockercfg,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					if len(objs) != 2 {
						t.Errorf("objs should be 2, but get %v", objs)
					}
				}
			},
		},
		{
			name: "default namespace in hosted mode",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
						},
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockercfg,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					if len(objs) < 2 {
						t.Errorf("import secret data %s, objs is empty: %v", constants.ImportSecretImportYamlKey, objs)
					}
					klusterlet, ok := objs[1].(*operatorv1.Klusterlet)
					if !ok {
						t.Fatalf("import secret data %s, the second element is not klusterlet", constants.ImportSecretImportYamlKey)
					}
					if klusterlet.Spec.Namespace != "open-cluster-management-test" {
						t.Errorf("import secret data %s, the klusterlet namespace %s is not %s",
							constants.ImportSecretImportYamlKey, klusterlet.Namespace, constants.DefaultKlusterletNamespace)
					}
				}
			},
		},
		{
			name: "namespace override",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletNamespaceAnnotation:  "test-ns",
							constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
						},
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockercfg,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					if len(objs) < 2 {
						t.Errorf("import secret data %s, objs is empty: %v", constants.ImportSecretImportYamlKey, objs)
					}
					klusterlet, ok := objs[1].(*operatorv1.Klusterlet)
					if !ok {
						t.Fatalf("import secret data %s, the second element is not klusterlet", constants.ImportSecretImportYamlKey)
					}
					if klusterlet.Spec.Namespace != "test-ns" {
						t.Errorf("import secret data %s, the klusterlet namespace %s is not %s",
							constants.ImportSecretImportYamlKey, klusterlet.Namespace, constants.DefaultKlusterletNamespace)
					}
				}
			},
		},
		{
			name: "pull secret ",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							constants.KlusterletNamespaceAnnotation: testKlusterletNamespace,
						},
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockercfg,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if data, ok := importSecret.Data[constants.ImportSecretCRDSYamlKey]; !ok || len(data) == 0 {
					t.Errorf("the %s is required", constants.ImportSecretCRDSYamlKey)
				}

				if data, ok := importSecret.Data[constants.ImportSecretCRDSV1beta1YamlKey]; !ok || len(data) == 0 {
					t.Errorf("the %s is required", constants.ImportSecretCRDSV1beta1YamlKey)
				}

				if data, ok := importSecret.Data[constants.ImportSecretCRDSV1YamlKey]; !ok || len(data) == 0 {
					t.Errorf("the %s is required", constants.ImportSecretCRDSV1YamlKey)
				}

				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					if len(objs) < 1 {
						t.Errorf("import secret data %s, objs is empty: %v", constants.ImportSecretImportYamlKey, objs)
					}
					ns, ok := objs[0].(*corev1.Namespace)
					if !ok {
						t.Errorf("import secret data %s, the first element is not namespace", constants.ImportSecretImportYamlKey)
					}
					if ns.Name != testKlusterletNamespace {
						t.Errorf("import secret data %s, the namespace name %s is not %s", constants.ImportSecretImportYamlKey, ns.Name, testKlusterletNamespace)
					}
					pullSecret, ok := objs[9].(*corev1.Secret)
					if !ok {
						t.Errorf("import secret data %s, the last element is not secret", constants.ImportSecretImportYamlKey)
					}
					if pullSecret.Type != corev1.SecretTypeDockercfg {
						t.Errorf("import secret data %s, the pull secret type %s is not %s", constants.ImportSecretImportYamlKey,
							pullSecret.Type, corev1.SecretTypeDockercfg)
					}
					if _, ok := pullSecret.Data[corev1.DockerConfigKey]; !ok {
						t.Errorf("import secret data %s, the pull secret data %s is not %s", constants.ImportSecretImportYamlKey,
							pullSecret.Data, corev1.DockerConfigKey)
					}
				}

				if len(strings.Split(strings.Replace(string(data), constants.YamlSperator, "", 1), constants.YamlSperator)) != 10 {
					t.Errorf("expect 10 files, but failed")
				}
			},
		},
		{
			name: "nodeSelector and tolerations from managed cluster annotations",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							"open-cluster-management/nodeSelector": "{\"kubernetes.io/os\":\"linux\"}",
							"open-cluster-management/tolerations":  "[{\"key\":\"foo\",\"operator\":\"Exists\",\"effect\":\"NoExecute\",\"tolerationSeconds\":20}]",
						},
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				_, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "klusterletconfig",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							apiconstants.AnnotationKlusterletConfig: "test-klusterletconfig",
						},
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			klusterletconfig: &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-klusterletconfig",
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					NodePlacement: &operatorv1.NodePlacement{
						NodeSelector: map[string]string{
							"kubernetes.io/os": "linux",
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "foo",
								Operator: corev1.TolerationOpExists,
								Effect:   corev1.TaintEffectNoExecute,
							},
						},
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					if len(objs) < 1 {
						t.Errorf("import secret data %s, objs is empty: %v", constants.ImportSecretImportYamlKey, objs)
					}
					klusterlet, ok := objs[8].(*operatorv1.Klusterlet)
					if !ok {
						t.Errorf("import secret data %s, the objs[8] is not klusterlet", constants.ImportSecretImportYamlKey)
					}
					if klusterlet.Spec.NodePlacement.NodeSelector["kubernetes.io/os"] != "linux" {
						t.Errorf("the klusterlet node selector %s is not %s",
							klusterlet.Spec.NodePlacement.NodeSelector["kubernetes.io/os"], "linux")
					}
					if klusterlet.Spec.NodePlacement.Tolerations[0].Key != "foo" {
						t.Errorf("the klusterlet tolerations %s is not %s",
							klusterlet.Spec.NodePlacement.Tolerations[0].Key, "foo")
					}
				}
			},
		},
		{
			name: "klusterletconfig with proxy config",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							apiconstants.AnnotationKlusterletConfig: "test-klusterletconfig",
						},
					},
				},
				&configv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa",
						Namespace: "test",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "test-bootstrap-sa-token-5pw5c",
							Namespace: "test",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-bootstrap-sa-token-5pw5c",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token": []byte("fake-token"),
					},
					Type: corev1.SecretTypeServiceAccountToken,
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
					},
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-root-ca.crt",
						Namespace: "test",
					},
					Data: map[string]string{
						"ca.crt": string(rootCACertData),
					},
				},
			},
			klusterletconfig: &klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-klusterletconfig",
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					HubKubeAPIServerProxyConfig: klusterletconfigv1alpha1.KubeAPIServerProxyConfig{
						HTTPSProxy: "https://127.0.0.1:3129",
						CABundle:   proxyServerCertData,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			validateFunc: func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface) {
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				kubeConfigData := extractBootstrapKubeConfigDataFromImportSecret(importSecret)
				if len(kubeConfigData) == 0 {
					t.Errorf("invalid bootstrap hub kubeconfig")
				}

				_, proxyURL, caData, _, err := parseKubeConfigData(kubeConfigData)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if proxyURL != "https://127.0.0.1:3129" {
					t.Errorf("expected proxy url https://127.0.0.1:3129, bug got %s", proxyURL)
				}

				ok, err := hasCertificates(caData, proxyServerCertData)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !ok {
					t.Errorf("the kubeconfig ca data does not include the proxy ca data")
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(c.runtimeObjs...)

			// setup klusterletconfig informer
			klusterletconfigs := []runtime.Object{}
			if c.klusterletconfig != nil {
				klusterletconfigs = append(klusterletconfigs, c.klusterletconfig)
			}
			fakeklusterletconfigClient := fakeklusterletconfigv1alpha1.NewSimpleClientset(klusterletconfigs...)
			klusterletconfigInformer := klusterletconfiginformerv1alpha1.NewKlusterletConfigInformer(fakeklusterletconfigClient, time.Second*30, cache.Indexers{
				cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
			})
			if c.klusterletconfig != nil {
				klusterletconfigInformer.GetStore().Add(c.klusterletconfig)
			}
			klusterletconfigLister := listerklusterletconfigv1alpha1.NewKlusterletConfigLister(klusterletconfigInformer.GetIndexer())

			clientHolder := &helpers.ClientHolder{
				KubeClient:          kubeClient,
				RuntimeClient:       fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.clientObjs...).Build(),
				ImageRegistryClient: imageregistry.NewClient(kubeClient),
			}

			r := &ReconcileImportConfig{
				clientHolder:           clientHolder,
				scheme:                 testscheme,
				klusterletconfigLister: klusterletconfigLister,
				recorder:               eventstesting.NewTestingEventRecorder(t),
			}

			_, err := r.Reconcile(context.TODO(), c.request)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c.validateFunc(t, r.clientHolder.RuntimeClient, r.clientHolder.KubeClient)
		})
	}
}
