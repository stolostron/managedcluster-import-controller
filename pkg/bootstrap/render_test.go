package bootstrap

import (
	"context"
	"os"
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func init() {
	os.Setenv(constants.RegistrationOperatorImageEnvVarName, "quay.io/open-cluster-management/registration-operator:latest")
	os.Setenv(constants.WorkImageEnvVarName, "quay.io/open-cluster-management/work:latest")
	os.Setenv(constants.RegistrationImageEnvVarName, "quay.io/open-cluster-management/registration:latest")
}

func TestKlusterletConfigGenerate(t *testing.T) {
	testcases := []struct {
		name         string
		clientObjs   []runtimeclient.Object
		runtimeObjs  []runtime.Object
		config       *KlusterletManifestsConfig
		validateFunc func(t *testing.T, objects []runtime.Object)
	}{
		{
			name: "default",
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
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
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
			runtimeObjs: []runtime.Object{
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
			name: "default customized",
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
						Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
						Namespace: os.Getenv("POD_NAMESPACE"),
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
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
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
