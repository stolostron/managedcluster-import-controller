package bootstrap

import (
	"context"
	"k8s.io/apimachinery/pkg/api/equality"
	apifeature "open-cluster-management.io/api/feature"
	"os"
	"testing"
	"time"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	v1 "open-cluster-management.io/api/cluster/v1"
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
		validateFunc           func(t *testing.T, objects, crds []runtime.Object)
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
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objs, 10)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objs[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateBoostrapSecret(t, objs[3], "bootstrap-hub-kubeconfig", constants.DefaultKlusterletNamespace, "bootstrap kubeconfig")
				testinghelpers.ValidateImagePullSecret(t, objs[4], constants.DefaultKlusterletNamespace, "{}")

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
			).WithPriorityClassName(constants.DefaultKlusterletPriorityClassName),
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objs, 11)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objs[8], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateBoostrapSecret(t, objs[4], "bootstrap-hub-kubeconfig",
					constants.DefaultKlusterletNamespace, "bootstrap kubeconfig")
				testinghelpers.ValidateImagePullSecret(t, objs[5], constants.DefaultKlusterletNamespace, "fake-token")
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
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeHosted,
				"test", // cluster name
				[]byte("bootstrap kubeconfig"),
			).WithoutImagePullSecretGenerate().WithPriorityClassName(constants.DefaultKlusterletPriorityClassName),
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 3)
				testinghelpers.ValidateCRDs(t, crds, 0)
				testinghelpers.ValidateKlusterlet(t, objects[2], operatorv1.InstallModeHosted,
					"klusterlet-test", "test", "open-cluster-management-test")
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
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeHosted,
				"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong-cluster", // cluster name
				[]byte("bootstrap kubeconfig"),
			).WithoutImagePullSecretGenerate(),
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 3)
				testinghelpers.ValidateCRDs(t, crds, 0)
				testinghelpers.ValidateKlusterlet(t, objects[2], operatorv1.InstallModeHosted,
					"klusterlet-loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong-cluster",
					"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong-cluster",
					"open-cluster-management-loooooooooooooooooooooooooooooooo")
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
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletClusterAnnotations(map[string]string{
				"agent.open-cluster-management.io/test": "test",
			}).WithManagedCluster(
				&v1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"open-cluster-management/nodeSelector": "{\"kubernetes.io/os\":\"linux\"}",
							"open-cluster-management/tolerations":  "[{\"key\":\"foo\",\"operator\":\"Exists\",\"effect\":\"NoExecute\",\"tolerationSeconds\":20}]",
						},
					},
				},
			),
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 10)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objects[0], "open-cluster-management-agent")
				testinghelpers.ValidateKlusterlet(t, objects[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", "open-cluster-management-agent")
				klusterlet, _ := objects[7].(*operatorv1.Klusterlet)
				if klusterlet.Spec.RegistrationConfiguration.ClusterAnnotations["agent.open-cluster-management.io/test"] != "test" {
					t.Errorf("the klusterlet cluster annotations %s is not %s",
						klusterlet.Spec.RegistrationConfiguration.ClusterAnnotations["agent.open-cluster-management.io/test"], "test")
				}
				deployment, ok := objects[6].(*appv1.Deployment)
				if !ok {
					t.Errorf("the objects[6] is not an appv1.Deployment")
				}
				if deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
					t.Errorf("the operater node selector %s is not %s",
						deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/os"], "linux")
				}
				if deployment.Spec.Template.Spec.Tolerations[0].Key != "foo" {
					t.Errorf("the operater tolerations %s is not %s",
						deployment.Spec.Template.Spec.Tolerations[0].Key, "foo")
				}
			},
		},
		{
			name: "default customized with klusterletConfig",
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
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 10)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objects[0], "open-cluster-management-agent")
				testinghelpers.ValidateKlusterlet(t, objects[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", "open-cluster-management-agent")
				klusterlet, _ := objects[7].(*operatorv1.Klusterlet)
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
				deployment, ok := objects[6].(*appv1.Deployment)
				if !ok {
					t.Errorf("the objects[7] is not an appv1.Deployment")
				}
				if deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
					t.Errorf("the operater node selector %s is not %s",
						deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/os"], "linux")
				}
				if deployment.Spec.Template.Spec.Tolerations[0].Key != "foo" {
					t.Errorf("the operater tolerations %s is not %s",
						deployment.Spec.Template.Spec.Tolerations[0].Key, "foo")
				}
			},
		},
		{
			name: "customize namespace with klusterletConfig no klusterlet name postfix",
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
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 4)
				testinghelpers.ValidateCRDs(t, crds, 0)
				testinghelpers.ValidateNamespace(t, objects[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objects[3], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
			},
		},
		{
			name: "customize namespace with klusterletConfig",
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
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 4)
				testinghelpers.ValidateCRDs(t, crds, 0)
				testinghelpers.ValidateNamespace(t, objects[0], "open-cluster-management-local")
				testinghelpers.ValidateKlusterlet(t, objects[3], operatorv1.InstallModeDefault,
					"klusterlet-local", "test", "open-cluster-management-local")
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
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				[]byte("bootstrap kubeconfig"),
			).WithPriorityClassName(constants.DefaultKlusterletPriorityClassName),
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 11)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objects[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objects[8], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)

				klusterlet, _ := objects[8].(*operatorv1.Klusterlet)
				if klusterlet.Spec.PriorityClassName != constants.DefaultKlusterletPriorityClassName {
					t.Errorf("expected priorityClass in klusterlet")
				}

				priorityClass, ok := objects[3].(*schedulingv1.PriorityClass)
				if !ok {
					t.Errorf("expected priorityClass ")
				}
				if priorityClass.Name != constants.DefaultKlusterletPriorityClassName {
					t.Errorf("expected priorityClass %s,but got %s",
						constants.DefaultKlusterletPriorityClassName, priorityClass.Name)
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
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
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
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 10)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objects[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objects[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)

				klusterlet, _ := objects[7].(*operatorv1.Klusterlet)
				if klusterlet.Spec.WorkConfiguration == nil {
					t.Errorf("the klusterlet WorkConfiguration is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod == nil {
					t.Errorf("the klusterlet AppliedManifestWorkEvictionGracePeriod is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod.Duration != 60*time.Minute {
					t.Errorf("the expected AppliedManifestWorkEvictionGracePeriod of klusterlet is %v, but got %v",
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
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
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
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 10)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objects[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objects[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				klusterlet, _ := objects[7].(*operatorv1.Klusterlet)
				if klusterlet.Spec.WorkConfiguration == nil {
					t.Errorf("the klusterlet WorkConfiguration is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod == nil {
					t.Errorf("the klusterlet AppliedManifestWorkEvictionGracePeriod is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod.Duration != 100*365*24*time.Hour {
					t.Errorf("the expected AppliedManifestWorkEvictionGracePeriod of klusterlet is %v, but got %v",
						100*365*24*time.Hour, klusterlet.Spec.WorkConfiguration.AppliedManifestWorkEvictionGracePeriod.Duration)
				}
			},
		},
		{
			name: "with mutliplehubs enabled",
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
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bootstrapkubeconfig-hub1",
					},
					Data: map[string][]byte{
						"kubeconfig": []byte("fake-kubeconfig"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bootstrapkubeconfig-hub2",
					},
					Data: map[string][]byte{
						"kubeconfig": []byte("fake-kubeconfig"),
					},
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test",
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
						Type: operatorv1.LocalSecrets,
						LocalSecrets: &operatorv1.LocalSecretsConfig{
							KubeConfigSecrets: []operatorv1.KubeConfigSecret{
								{
									Name: "bootstrapkubeconfig-hub1",
								},
								{
									Name: "bootstrapkubeconfig-hub2",
								},
							},
							HubConnectionTimeoutSeconds: 500,
						},
					},
				},
			}),
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				// 12 objects for klusterlet manifests, 3 objects for bootstrap kubeconfig secrets
				testinghelpers.ValidateObjectCount(t, objs, 12)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objs[9], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				klusterlet, _ := objs[9].(*operatorv1.Klusterlet)
				if klusterlet.Spec.RegistrationConfiguration == nil {
					t.Errorf("the klusterlet features is not specified")
				}
				multiplehubsEnabled := false
				for _, fg := range klusterlet.Spec.RegistrationConfiguration.FeatureGates {
					if fg.Feature == "MultipleHubs" && fg.Mode == operatorv1.FeatureGateModeTypeEnable {
						multiplehubsEnabled = true
						break
					}
				}
				if !multiplehubsEnabled {
					t.Errorf("the klusterlet MultipleHubs feature is not enabled")
				}

				if klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.Type != operatorv1.LocalSecrets {
					t.Errorf("the klusterlet bootstrap kubeconfig type is not %s", operatorv1.LocalSecrets)
				}
				if len(klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets) != 3 {
					t.Errorf("the klusterlet bootstrap kubeconfig secrets count is not 3")
				}
				for _, secret := range klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets {
					if secret.Name != "bootstrapkubeconfig-hub1" && secret.Name != "bootstrapkubeconfig-hub2" && secret.Name != "bootstrap-hub-kubeconfig-current-hub" {
						t.Errorf("the klusterlet bootstrap kubeconfig secret name is not bootstrapkubeconfig-hub1 or bootstrapkubeconfig-hub2 or bootstrap-hub-kubeconfig-current-hub")
					}
				}
				if klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.HubConnectionTimeoutSeconds != 500 {
					t.Errorf("the klusterlet bootstrap kubeconfig hub connection timeout seconds is not 500")
				}

				testinghelpers.ValidateBoostrapSecret(t, objs[3], "bootstrap-hub-kubeconfig-current-hub",
					constants.DefaultKlusterletNamespace, "bootstrap kubeconfig")
				testinghelpers.ValidateBoostrapSecret(t, objs[4], "bootstrapkubeconfig-hub1",
					constants.DefaultKlusterletNamespace, "fake-kubeconfig")
				testinghelpers.ValidateBoostrapSecret(t, objs[5], "bootstrapkubeconfig-hub2",
					constants.DefaultKlusterletNamespace, "fake-kubeconfig")

			},
		},
		{
			name: "with mutliplehubs enabled but local-cluster",
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
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bootstrapkubeconfig-hub1",
					},
					Data: map[string][]byte{
						"kubeconfig": []byte("fake-kubeconfig"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bootstrapkubeconfig-hub2",
					},
					Data: map[string][]byte{
						"kubeconfig": []byte("fake-kubeconfig"),
					},
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test",
				[]byte("bootstrap kubeconfig"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
						Type: operatorv1.LocalSecrets,
						LocalSecrets: &operatorv1.LocalSecretsConfig{
							KubeConfigSecrets: []operatorv1.KubeConfigSecret{
								{
									Name: "bootstrapkubeconfig-hub1",
								},
								{
									Name: "bootstrapkubeconfig-hub2",
								},
							},
							HubConnectionTimeoutSeconds: 500,
						},
					},
				},
			}).WithManagedCluster(
				&v1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"local-cluster": "true",
						},
					},
				},
			),
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				// 10 objects for klusterlet manifests
				testinghelpers.ValidateObjectCount(t, objs, 10)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objs[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				klusterlet, _ := objs[7].(*operatorv1.Klusterlet)
				if klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.Type == operatorv1.LocalSecrets {
					t.Fatal("the klusterlet bootstrap kubeconfig type is not replaced")
				}

				testinghelpers.ValidateBoostrapSecret(t, objs[3], "bootstrap-hub-kubeconfig",
					constants.DefaultKlusterletNamespace, "bootstrap kubeconfig")
			},
		},
		{
			name:                   "default cluster claim configuration",
			defaultImagePullSecret: "",
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				[]byte("bootstrap kubeconfig"),
			),
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objs, 10)

				testinghelpers.ValidateKlusterlet(t, objs[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				klusterlet, _ := objs[7].(*operatorv1.Klusterlet)
				if klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration == nil {
					t.Errorf("the klusterlet ClusterClaimConfiguration should not be nil")
				}

				if klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.MaxCustomClusterClaims != 0 {
					t.Errorf("the klusterlet ClusterClaimConfiguration MaxCustomClusterClaims %d should be 0",
						klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.MaxCustomClusterClaims)
				}
				if !equality.Semantic.DeepEqual(
					klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.ReservedClusterClaimSuffixes,
					reservedClusterClaimSuffixes) {
					t.Errorf("not expected klusterlet ClusterClaimConfiguration ReservedClusterClaimSuffixes %v",
						klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.ReservedClusterClaimSuffixes)
				}
			},
		},
		{
			name: "with cluster claim configuration",
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
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					ClusterClaimConfiguration: &klusterletconfigv1alpha1.ClusterClaimConfiguration{
						MaxCustomClusterClaims: 25,
					},
				},
			}),
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objs, 10)

				testinghelpers.ValidateKlusterlet(t, objs[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				klusterlet, _ := objs[7].(*operatorv1.Klusterlet)
				if klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration == nil {
					t.Errorf("the klusterlet ClusterClaimConfiguration should not be nil")
				}

				if klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.MaxCustomClusterClaims != 25 {
					t.Errorf("the klusterlet ClusterClaimConfiguration MaxCustomClusterClaims %d should be 25",
						klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.MaxCustomClusterClaims)
				}
				if !equality.Semantic.DeepEqual(
					klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.ReservedClusterClaimSuffixes,
					reservedClusterClaimSuffixes) {
					t.Errorf("not expected klusterlet ClusterClaimConfiguration ReservedClusterClaimSuffixes %v",
						klusterlet.Spec.RegistrationConfiguration.ClusterClaimConfiguration.ReservedClusterClaimSuffixes)
				}
			},
		},
		{
			name: "with feature gates",
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
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					FeatureGates: []operatorv1.FeatureGate{
						{
							Feature: string(apifeature.RawFeedbackJsonString),
							Mode:    operatorv1.FeatureGateModeTypeEnable,
						},
						{
							Feature: string(apifeature.ClusterClaim),
							Mode:    operatorv1.FeatureGateModeTypeDisable,
						},
					},
				},
			}),
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				testinghelpers.ValidateObjectCount(t, objects, 10)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objects[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objects[7], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)

				klusterlet, _ := objects[7].(*operatorv1.Klusterlet)
				if klusterlet.Spec.WorkConfiguration == nil {
					t.Errorf("the klusterlet WorkConfiguration is not specified")
				}
				if !equality.Semantic.DeepEqual(klusterlet.Spec.WorkConfiguration.FeatureGates, []operatorv1.FeatureGate{
					{
						Feature: string(apifeature.RawFeedbackJsonString),
						Mode:    operatorv1.FeatureGateModeTypeEnable,
					},
				}) {
					t.Errorf("the klusterlet work feature gate is not set.")
				}
				if klusterlet.Spec.RegistrationConfiguration == nil {
					t.Errorf("the klusterlet RegistrationConfiguration is not specified")
				}
				if !equality.Semantic.DeepEqual(klusterlet.Spec.RegistrationConfiguration.FeatureGates, []operatorv1.FeatureGate{
					{
						Feature: string(apifeature.ClusterClaim),
						Mode:    operatorv1.FeatureGateModeTypeDisable,
					},
				}) {
					t.Errorf("the klusterlet registration feature gate is not set.")
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
			manifestsBytes, crdBytes, err := testcase.config.Generate(context.Background(), clientHolder)
			if err != nil {
				t.Fatalf("%s Failed to generate klusterlet manifests: %v", testcase.name, err)
			}

			crdObjs := []runtime.Object{}
			if len(crdBytes) != 0 {
				crdObjs = append(crdObjs, helpers.MustCreateObject(crdBytes))
			}

			objs := []runtime.Object{}
			for _, yaml := range helpers.SplitYamls(manifestsBytes) {
				objs = append(objs, helpers.MustCreateObject(yaml))
			}
			testcase.validateFunc(t, objs, crdObjs)
		})
	}
}
