// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"os"
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

				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}

					testinghelpers.ValidateObjectCount(t, objs, 10)
					testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
					testinghelpers.ValidateImagePullSecret(t, objs[4], constants.DefaultKlusterletNamespace, "fake-token")
					secret, ok := objs[3].(*corev1.Secret)
					if !ok {
						t.Errorf("expected secret, but got %v", objs[3])
					}
					if secret.Name != "bootstrap-hub-kubeconfig" {
						t.Errorf("expected secret bootstrap-hub-kubeconfig, but got %s", secret.Name)
					}
					if secret.Namespace != constants.DefaultKlusterletNamespace {
						t.Errorf("expected secret ns %s, but got %s", constants.DefaultKlusterletNamespace, secret.Namespace)
					}
					if secret.Type != corev1.SecretTypeOpaque {
						t.Errorf("expected bootstrap secret, but got %#v", secret)
					}
					if data := secret.Data["kubeconfig"]; string(data) == "" {
						t.Errorf("expected bootstrap secret data %v, but got empty", string(data))
					}
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
				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					testinghelpers.ValidateObjectCount(t, objs, 3)
					testinghelpers.ValidateKlusterlet(t, objs[2], operatorv1.InstallModeHosted, "klusterlet-test",
						"test", "open-cluster-management-test")
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
					testinghelpers.ValidateObjectCount(t, objs, 3)
					testinghelpers.ValidateKlusterlet(t, objs[2], operatorv1.InstallModeHosted, "klusterlet-test",
						"test", "open-cluster-management-test")
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
					testinghelpers.ValidateObjectCount(t, objs, 3)
					testinghelpers.ValidateKlusterlet(t, objs[2], operatorv1.InstallModeHosted, "klusterlet-test",
						"test", "test-ns")
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

				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					testinghelpers.ValidateObjectCount(t, objs, 10)
					testinghelpers.ValidateNamespace(t, objs[0], "open-cluster-management-agent-test")
					testinghelpers.ValidateKlusterlet(t, objs[7], operatorv1.InstallModeSingleton, "klusterlet",
						"test", "open-cluster-management-agent-test")
					testinghelpers.ValidateImagePullSecret(t, objs[4], "open-cluster-management-agent-test", "fake-token")
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
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if data, ok := importSecret.Data[constants.ImportSecretCRDSYamlKey]; !ok || len(data) == 0 {
					t.Errorf("the %s is required", constants.ImportSecretCRDSYamlKey)
				}

				data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]
				if !ok {
					t.Errorf("the %s is required, %s", constants.ImportSecretImportYamlKey, string(data))
				} else {
					objs := []runtime.Object{}
					for _, yaml := range helpers.SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
						objs = append(objs, helpers.MustCreateObject(yaml))
					}
					testinghelpers.ValidateObjectCount(t, objs, 10)
					testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
					testinghelpers.ValidateKlusterlet(t, objs[7], operatorv1.InstallModeSingleton, "klusterlet",
						"test", constants.DefaultKlusterletNamespace)
					testinghelpers.ValidateImagePullSecret(t, objs[4], constants.DefaultKlusterletNamespace, "fake-token")
					klusterlet, ok := objs[7].(*operatorv1.Klusterlet)
					if !ok {
						t.Errorf("the klusterlet is not found")
					}
					if len(klusterlet.Spec.NodePlacement.NodeSelector) != 1 {
						t.Errorf("the klusterlet node selector is not found")
					}
					if len(klusterlet.Spec.NodePlacement.Tolerations) != 1 {
						t.Errorf("the klusterlet node Tolerations is not found")
					}
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
					testinghelpers.ValidateObjectCount(t, objs, 10)
					testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
					testinghelpers.ValidateKlusterlet(t, objs[7], operatorv1.InstallModeSingleton, "klusterlet",
						"test", constants.DefaultKlusterletNamespace)
					testinghelpers.ValidateImagePullSecret(t, objs[4], constants.DefaultKlusterletNamespace, "fake-token")
					klusterlet, ok := objs[7].(*operatorv1.Klusterlet)
					if !ok {
						t.Errorf("the klusterlet is not found")
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

				_, proxyURL, _, caData, _, _, err := parseKubeConfigData(kubeConfigData)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if proxyURL != "https://127.0.0.1:3129" {
					t.Errorf("expected proxy url https://127.0.0.1:3129, bug got %s", proxyURL)
				}

				ok, err := helpers.HasCertificates(caData, proxyServerCertData)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !ok {
					t.Errorf("the kubeconfig ca data does not include the proxy ca data")
				}
			},
		},
		{
			name: "self managed cluster",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Labels: map[string]string{
							constants.SelfManagedLabel: "true",
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
				importSecret, err := kubeClient.CoreV1().Secrets("test").Get(context.TODO(), "test-import", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				kubeConfigData := extractBootstrapKubeConfigDataFromImportSecret(importSecret)
				if len(kubeConfigData) == 0 {
					t.Errorf("invalid bootstrap hub kubeconfig")
				}

				kubeAPIServer, _, ca, caData, _, _, err := parseKubeConfigData(kubeConfigData)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if kubeAPIServer != "https://kubernetes.default.svc:443" {
					t.Errorf("expected apiserver address https://kubernetes.default.svc:443, bug got %s", kubeAPIServer)
				}

				if ca != "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt" {
					t.Errorf("expected ca file /var/run/secrets/kubernetes.io/serviceaccount/ca.crt, bug got %s", ca)
				}

				if len(caData) > 0 {
					t.Errorf("expected empty ca data, bug got %s", string(caData))
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
