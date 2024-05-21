package bootstrap

import (
	"context"
	"os"
	"testing"
	"time"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
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
				[]byte("bootstrap kubeconfig"),
			).WithImagePullSecretGenerate(false),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 2 {
					t.Fatalf("Expected 2 objects, but got %d", len(objects))
				}
				klusterlet, ok := objects[1].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.Namespace != "open-cluster-management-test" {
					t.Fatal("the klusterlet namespace is not replaced")
				}
				if klusterlet.Name != "klusterlet-test" {
					t.Fatal("the klusterlet name is not replaced.")
				}
			},
		},
		{
			name: "hosted with long cluster name",
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
				"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong-cluster", // cluster name
				[]byte("bootstrap kubeconfig"),
			).WithImagePullSecretGenerate(false),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 2 {
					t.Fatalf("Expected 2 objects, but got %d", len(objects))
				}
				klusterlet, ok := objects[1].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.Namespace != "open-cluster-management-loooooooooooooooooooooooooooooooo" {
					t.Fatal("the klusterlet namespace is not replaced")
				}
				if klusterlet.Name != "klusterlet-loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong-cluster" {
					t.Fatal("the klusterlet name is not replaced.")
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
			name: "customize namespace with klusterletconfig no klusterlet name postfix",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management-local",
					},
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					InstallMode: &klusterletconfigv1alpha1.InstallMode{
						Type: klusterletconfigv1alpha1.InstallModeNoOperator,
					},
				},
			}),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 3 {
					t.Fatalf("Expected 10 objects, but got %d", len(objects))
				}

				klusterlet, ok := objects[1].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.Namespace != constants.DefaultKlusterletNamespace {
					t.Fatal("the klusterlet namespace is not replaced")
				}
				if klusterlet.Name != "klusterlet" {
					t.Fatal("the klusterlet name is not replaced.")
				}
			},
		},
		{
			name: "customize namespace with klusterletconfig",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "open-cluster-management-local",
					},
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					InstallMode: &klusterletconfigv1alpha1.InstallMode{
						Type: klusterletconfigv1alpha1.InstallModeNoOperator,
						NoOperator: &klusterletconfigv1alpha1.NoOperator{
							Postfix: "local",
						},
					},
				},
			}),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 3 {
					t.Fatalf("Expected 10 objects, but got %d", len(objects))
				}

				klusterlet, ok := objects[1].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.Namespace != "open-cluster-management-local" {
					t.Fatal("the klusterlet namespace is not replaced")
				}
				if klusterlet.Name != "klusterlet-local" {
					t.Fatal("the klusterlet name is not replaced.")
				}
			},
		},
		{
			name: "with priority class",
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
				[]byte("bootstrap kubeconfig"),
			).WithPriorityClassName(constants.DefaultKlusterletPriorityClassName),
			validateFunc: func(t *testing.T, objects []runtime.Object) {
				if len(objects) != 12 {
					t.Fatalf("Expected 10 objects, but got %d", len(objects))
				}

				priorityClass, ok := objects[1].(*schedulingv1.PriorityClass)
				if !ok {
					t.Fatal("the PriorityClass is not PriorityClass")
				}
				if priorityClass.Name != constants.DefaultKlusterletPriorityClassName {
					t.Fatalf("expected PriorityClass %s, but got: %s",
						constants.DefaultKlusterletPriorityClassName, priorityClass.Name)
				}

				klusterlet, ok := objects[10].(*operatorv1.Klusterlet)
				if !ok {
					t.Fatal("the klusterlet is not klusterlet")
				}
				if klusterlet.Spec.PriorityClassName != constants.DefaultKlusterletPriorityClassName {
					t.Fatalf("the expected klusterlet PriorityClass is %s, but got: %s",
						constants.DefaultKlusterletPriorityClassName, klusterlet.Spec.PriorityClassName)
				}
			},
		},
		{
			name: "with customized appliedManifestWorkEvictionGracePeriod",
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
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					AppliedManifestWorkEvictionGracePeriod: "60m",
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
				if klusterlet.Spec.WorkConfiguration == nil {
					t.Fatal("the klusterlet WorkConfiguration is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod == nil {
					t.Fatal("the klusterlet AppliedManifestWorkEvictionGracePeriod is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod.Duration != 60*time.Minute {
					t.Fatalf("the expected AppliedManifestWorkEvictionGracePeriod of klusterlet is %v, but got %v",
						60*time.Minute, klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod.Duration)
				}
			},
		},
		{
			name: "with appliedManifestWorkEviction disabled",
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
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					AppliedManifestWorkEvictionGracePeriod: constants.AppliedManifestWorkEvictionGracePeriodInfinite,
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
				if klusterlet.Spec.WorkConfiguration == nil {
					t.Fatal("the klusterlet WorkConfiguration is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod == nil {
					t.Fatal("the klusterlet AppliedManifestWorkEvictionGracePeriod is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod.Duration != 100*365*24*time.Hour {
					t.Fatalf("the expected AppliedManifestWorkEvictionGracePeriod of klusterlet is %v, but got %v",
						100*365*24*time.Hour, klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod.Duration)
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
