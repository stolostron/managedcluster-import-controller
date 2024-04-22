package bootstrap

import (
	"context"
	"os"
	"testing"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	os.Setenv(constants.RegistrationOperatorImageEnvVarName, "quay.io/open-cluster-management/registration-operator:latest")
	os.Setenv(constants.WorkImageEnvVarName, "quay.io/open-cluster-management/work:latest")
	os.Setenv(constants.RegistrationImageEnvVarName, "quay.io/open-cluster-management/registration:latest")
}

func TestKlusterletConfigGenerate(t *testing.T) {
	var tolerationSeconds int64 = 20

	testcases := []struct {
		name                   string
		defaultImagePullSecret string
		clientObjs             []runtimeclient.Object
		runtimeObjs            []runtime.Object
		config                 *KlusterletManifestsConfig
		validateFunc           func(t *testing.T, objects []runtime.Object)
	}{
		{
			name: "default without DEFAULT_IMAGE_PULL_SECRET set",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			defaultImagePullSecret: "",
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				"test", // klusterlet namespace
				[]byte("bootstrap kubeconfig"),
			),
			validateFunc: func(t *testing.T, objs []runtime.Object) {
				if len(objs) != 10 {
					t.Fatalf("Expected 10 objects, but got %d", len(objs))
				}
				if len(objs) != 10 {
					t.Fatalf("Expected 10 objects, but got %d", len(objs))
				}
				_, ok := objs[0].(*corev1.Namespace)
				if !ok {
					t.Errorf("import secret data %s, the first element is not namespace", constants.ImportSecretImportYamlKey)
				}
				pullSecret, ok := objs[9].(*corev1.Secret)
				if !ok {
					t.Errorf("import secret data %s, the last element is not secret", constants.ImportSecretImportYamlKey)
				}
				if pullSecret.Type != corev1.SecretTypeDockerConfigJson {
					t.Errorf("the pull secret type %s is not %s",
						pullSecret.Type, corev1.SecretTypeDockerConfigJson)
				}
				if _, ok := pullSecret.Data[corev1.DockerConfigJsonKey]; !ok {
					t.Errorf("the pull secret data %s is not %s",
						pullSecret.Data, corev1.DockerConfigJsonKey)
				}
				// the content of the pull secret is "{}"
				if string(pullSecret.Data[corev1.DockerConfigJsonKey]) != "{}" {
					t.Errorf("the pull secret data %s is not %s",
						pullSecret.Data[corev1.DockerConfigJsonKey], "{}")
				}
			},
		},
		{
			name: "default",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			defaultImagePullSecret: "test-image-pull-secret",
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-image-pull-secret",
					},
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				"test", // klusterlet namespace
				[]byte("bootstrap kubeconfig"),
			),
			validateFunc: func(t *testing.T, objs []runtime.Object) {
				if len(objs) != 10 {
					t.Fatalf("Expected 10 objects, but got %d", len(objs))
				}
				_, ok := objs[0].(*corev1.Namespace)
				if !ok {
					t.Errorf("import secret data %s, the first element is not namespace", constants.ImportSecretImportYamlKey)
				}
				pullSecret, ok := objs[9].(*corev1.Secret)
				if !ok {
					t.Errorf("import secret data %s, the last element is not secret", constants.ImportSecretImportYamlKey)
				}
				if pullSecret.Type != corev1.SecretTypeDockerConfigJson {
					t.Errorf("the pull secret type %s is not %s",
						pullSecret.Type, corev1.SecretTypeDockerConfigJson)
				}
				if _, ok := pullSecret.Data[corev1.DockerConfigJsonKey]; !ok {
					t.Errorf("the pull secret data %s is not %s",
						pullSecret.Data, corev1.DockerConfigJsonKey)
				}
			},
		},
		{
			name: "hosted",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			defaultImagePullSecret: "test-image-pull-secret",
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-image-pull-secret",
					},
					Data: map[string][]byte{
						corev1.DockerConfigKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockercfg,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeHosted,
				"test", // cluster name
				"test", // klusterlet namespace
				[]byte("bootstrap kubeconfig"),
			).WithImagePullSecretGenerate(false),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 2 {
					t.Fatalf("Expected 2 objects, but got %d", len(objects))
				}
			},
		},
		{
			name:                   "default customized with managed cluster annotations",
			defaultImagePullSecret: "test-image-pull-secret",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-image-pull-secret",
					},
					Data: map[string][]byte{
						corev1.DockerConfigKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockercfg,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				"test", // klusterlet namespace
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletClusterAnnotations(map[string]string{
				"agent.open-cluster-management.io/test": "test",
			}).WithManagedClusterAnnotations(map[string]string{
				"open-cluster-management/nodeSelector": "{\"kubernetes.io/os\":\"linux\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"foo\",\"operator\":\"Exists\",\"effect\":\"NoExecute\",\"tolerationSeconds\":20}]",
			}),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 10 {
					t.Fatalf("Expected 10 objects, but got %d", len(objects))
				}

				klusterlet, ok := objects[8].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.RegistrationConfiguration.ClusterAnnotations["agent.open-cluster-management.io/test"] != "test" {
					t.Errorf("the klusterlet cluster annotations %s is not %s",
						klusterlet.Spec.RegistrationConfiguration.ClusterAnnotations["agent.open-cluster-management.io/test"], "test")
				}

				operater, ok := objects[6].(*appv1.Deployment)
				if !ok {
					t.Fatal("the operater is not deployment")
				}

				if operater.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
					t.Errorf("the operater node selector %s is not %s",
						operater.Spec.Template.Spec.NodeSelector["kubernetes.io/os"], "linux")
				}
				if operater.Spec.Template.Spec.Tolerations[0].Key != "foo" {
					t.Errorf("the operater tolerations %s is not %s",
						operater.Spec.Template.Spec.Tolerations[0].Key, "foo")
				}
			},
		},
		{
			name: "default customized with klusterletconfig",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				},
			},
			defaultImagePullSecret: "test-image-pull-secret",
			runtimeObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-image-pull-secret",
					},
					Data: map[string][]byte{
						corev1.DockerConfigKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockercfg,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				"test", // klusterlet namespace
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					Registries: []klusterletconfigv1alpha1.Registries{
						{
							Source: "quay.io/open-cluster-management",
							Mirror: "quay.io/rhacm2",
						},
					},
					NodePlacement: &operatorv1.NodePlacement{
						NodeSelector: map[string]string{
							"kubernetes.io/os": "linux",
						},
						Tolerations: []corev1.Toleration{
							{
								Key:               "foo",
								Operator:          corev1.TolerationOpExists,
								Effect:            corev1.TaintEffectNoExecute,
								TolerationSeconds: &tolerationSeconds,
							},
						},
					},
				},
			}),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 10 {
					t.Fatalf("Expected 10 objects, but got %d", len(objects))
				}

				klusterlet, ok := objects[8].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.RegistrationImagePullSpec != "quay.io/rhacm2/registration:latest" {
					t.Fatal("the klusterlet registration image pull spec is not replaced")
				}
				if klusterlet.Spec.NodePlacement.NodeSelector["kubernetes.io/os"] != "linux" {
					t.Errorf("the klusterlet node selector %s is not %s",
						klusterlet.Spec.NodePlacement.NodeSelector["kubernetes.io/os"], "linux")
				}
				if klusterlet.Spec.NodePlacement.Tolerations[0].Key != "foo" {
					t.Errorf("the klusterlet tolerations %s is not %s",
						klusterlet.Spec.NodePlacement.Tolerations[0].Key, "foo")
				}

				operater, ok := objects[6].(*appv1.Deployment)
				if !ok {
					t.Fatal("the operater is not deployment")
				}

				if operater.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
					t.Errorf("the operater node selector %s is not %s",
						operater.Spec.Template.Spec.NodeSelector["kubernetes.io/os"], "linux")
				}
				if operater.Spec.Template.Spec.Tolerations[0].Key != "foo" {
					t.Errorf("the operater tolerations %s is not %s",
						operater.Spec.Template.Spec.Tolerations[0].Key, "foo")
				}
			},
		},
		{
			name: "customize namespace with klusterletconfig",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "new-ns",
					},
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				"test", // klusterlet namespace
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					AgentInstallNamespace: "new-ns",
				},
			}),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 10 {
					t.Fatalf("Expected 10 objects, but got %d", len(objects))
				}

				klusterlet, ok := objects[8].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.Namespace != "new-ns" {
					t.Fatal("the klusterlet namespace is not replaced")
				}
				if klusterlet.Name != "klusterlet-new-ns" {
					t.Fatal("the klusterlet name is not replaced.")
				}

				operater, ok := objects[6].(*appv1.Deployment)
				if !ok {
					t.Fatal("the operater is not deployment")
				}

				if operater.Namespace != "new-ns" {
					t.Errorf("the operater namespace %s is not new-ns", operater.Namespace)
				}
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			os.Setenv(constants.DefaultImagePullSecretEnvVarName, testcase.defaultImagePullSecret)

			kubeClient := kubefake.NewSimpleClientset(testcase.runtimeObjs...)
			clientHolder := &helpers.ClientHolder{
				KubeClient:          kubeClient,
				RuntimeClient:       fake.NewClientBuilder().WithScheme(testscheme).WithObjects(testcase.clientObjs...).Build(),
				ImageRegistryClient: imageregistry.NewClient(kubeClient),
			}
			manifestsBytes, err := testcase.config.Generate(context.Background(), clientHolder)
			if err != nil {
				t.Fatalf("%s Failed to generate klusterlet manifests: %v", testcase.name, err)
			}
			objs := []runtime.Object{}
			for _, yaml := range helpers.SplitYamls(manifestsBytes) {
				objs = append(objs, helpers.MustCreateObject(yaml))
			}
			testcase.validateFunc(t, objs)
		})
	}
}
