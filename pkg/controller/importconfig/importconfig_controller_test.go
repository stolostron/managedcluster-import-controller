// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &configv1.Infrastructure{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &configv1.APIServer{})

	os.Setenv(registrationOperatorImageEnvVarName, "quay.io/open-cluster-management/registration-operator:latest")
	os.Setenv(workImageEnvVarName, "quay.io/open-cluster-management/work:latest")
	os.Setenv(registrationImageEnvVarName, "quay.io/open-cluster-management/registration:latest")
	os.Setenv(defaultImagePullSecretEnvVarName, "test-image-pul-secret-secret")
	os.Setenv(constants.PodNamespaceEnvVarName, "cluster-secret")
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		clientObjs   []runtimeclient.Object
		runtimeObjs  []runtime.Object
		request      reconcile.Request
		validateFunc func(t *testing.T, client runtimeclient.Client, kubeClient kubernetes.Interface)
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
					if ns.Name != klusterletNamespace {
						t.Errorf("import secret data %s, the namespace name %s is not %s", constants.ImportSecretImportYamlKey, ns.Name, klusterletNamespace)
					}
					pullSecret, ok := objs[8].(*corev1.Secret)
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

				if len(strings.Split(strings.Replace(string(data), constants.YamlSperator, "", 1), constants.YamlSperator)) != 9 {
					t.Errorf("expect 9 files, but failed")
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
					if ns.Name != klusterletNamespace {
						t.Errorf("import secret data %s, the namespace name %s is not %s", constants.ImportSecretImportYamlKey, ns.Name, klusterletNamespace)
					}
					pullSecret, ok := objs[8].(*corev1.Secret)
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

				if len(strings.Split(strings.Replace(string(data), constants.YamlSperator, "", 1), constants.YamlSperator)) != 9 {
					t.Errorf("expect 9 files, but failed")
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(c.runtimeObjs...)
			clientHolder := &helpers.ClientHolder{
				KubeClient:          kubeClient,
				RuntimeClient:       fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.clientObjs...).Build(),
				ImageRegistryClient: imageregistry.NewClient(kubeClient),
			}

			r := &ReconcileImportConfig{
				clientHolder:  clientHolder,
				scheme:        testscheme,
				recorder:      eventstesting.NewTestingEventRecorder(t),
				workerFactory: &workerFactory{clientHolder: clientHolder},
			}

			_, err := r.Reconcile(context.TODO(), c.request)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			c.validateFunc(t, r.clientHolder.RuntimeClient, r.clientHolder.KubeClient)
		})
	}
}
