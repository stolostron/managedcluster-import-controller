package bootstrap

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apifeature "open-cluster-management.io/api/feature"

	routev1 "github.com/openshift/api/route/v1"
	routefake "github.com/openshift/client-go/route/clientset/versioned/fake"
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
	"open-cluster-management.io/ocm/pkg/operator/helpers/chart"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
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
		expectError            bool
		errorMessage           string
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
					MultipleHubsConfig: &klusterletconfigv1alpha1.MultipleHubsConfig{
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
						GenBootstrapKubeConfigStrategy: klusterletconfigv1alpha1.GenBootstrapKubeConfigStrategyDefault,
					},
				},
			}),
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				// 11 objects for klusterlet manifests, 2 objects for bootstrap kubeconfig secrets (no current hub)
				// The current hub secret should NOT be present in this case.
				// Keep the original validation logic for Klusterlet and other fields.
				// Find the Klusterlet object
				testinghelpers.ValidateObjectCount(t, objs, 11)
				testinghelpers.ValidateCRDs(t, crds, 1)
				testinghelpers.ValidateNamespace(t, objs[0], constants.DefaultKlusterletNamespace)
				testinghelpers.ValidateKlusterlet(t, objs[8], operatorv1.InstallModeDefault,
					"klusterlet", "test", constants.DefaultKlusterletNamespace)
				klusterlet, _ := objs[8].(*operatorv1.Klusterlet)
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
				if len(klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets) != 2 {
					t.Errorf("the klusterlet bootstrap kubeconfig secrets count is not 2")
				}
				for _, secret := range klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets {
					if secret.Name != "bootstrapkubeconfig-hub1" && secret.Name != "bootstrapkubeconfig-hub2" {
						t.Errorf("the klusterlet bootstrap kubeconfig secret name is not bootstrapkubeconfig-hub1 or bootstrapkubeconfig-hub2")
					}
				}
				t.Logf("klusterlet: %v", klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets)
				if klusterlet.Spec.RegistrationConfiguration.BootstrapKubeConfigs.LocalSecrets.HubConnectionTimeoutSeconds != 500 {
					t.Errorf("the klusterlet bootstrap kubeconfig hub connection timeout seconds is not 500")
				}
				// Validate the actual secrets in the objects (should not include current hub)
				var foundSecrets []string
				for _, obj := range objs {
					secret, ok := obj.(*corev1.Secret)
					if ok && (secret.Name == "bootstrapkubeconfig-hub1" || secret.Name == "bootstrapkubeconfig-hub2") {
						foundSecrets = append(foundSecrets, secret.Name)
					}
				}
				if len(foundSecrets) != 2 {
					t.Errorf("expected 2 bootstrap secrets, got %d: %v", len(foundSecrets), foundSecrets)
				}
				for _, name := range foundSecrets {
					if name != "bootstrapkubeconfig-hub1" && name != "bootstrapkubeconfig-hub2" {
						t.Errorf("unexpected secret name: %s", name)
					}
				}
			},
		},
		{
			name: "with mutliplehubs enabled and IncludeCurrentHub strategy (should include current hub)",
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
					MultipleHubsConfig: &klusterletconfigv1alpha1.MultipleHubsConfig{
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
						GenBootstrapKubeConfigStrategy: klusterletconfigv1alpha1.GenBootstrapKubeConfigStrategyIncludeCurrentHub,
					},
				},
			}),
			validateFunc: func(t *testing.T, objs, crds []runtime.Object) {
				// Only count bootstrap kubeconfig secrets, not unrelated secrets like image pull credentials
				var foundSecrets []string
				for _, obj := range objs {
					secret, ok := obj.(*corev1.Secret)
					if ok && (secret.Name == "bootstrapkubeconfig-hub1" || secret.Name == "bootstrapkubeconfig-hub2" || secret.Name == "bootstrap-hub-kubeconfig-current-hub") {
						foundSecrets = append(foundSecrets, secret.Name)
					}
				}
				if len(foundSecrets) != 3 {
					t.Errorf("expected 3 bootstrap secrets, got %d: %v", len(foundSecrets), foundSecrets)
				}
				for _, name := range foundSecrets {
					if name != "bootstrapkubeconfig-hub1" && name != "bootstrapkubeconfig-hub2" && name != "bootstrap-hub-kubeconfig-current-hub" {
						t.Errorf("unexpected secret name: %s", name)
					}
				}
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
					MultipleHubsConfig: &klusterletconfigv1alpha1.MultipleHubsConfig{
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
						GenBootstrapKubeConfigStrategy: klusterletconfigv1alpha1.GenBootstrapKubeConfigStrategyDefault,
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
		{
			name: "with customized workStatusSyncInterval",
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
					WorkStatusSyncInterval: &metav1.Duration{Duration: 5 * time.Second},
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
				if klusterlet.Spec.WorkConfiguration.StatusSyncInterval == nil {
					t.Errorf("the klusterlet StatusSyncInterval is not specified")
				}
				if klusterlet.Spec.WorkConfiguration.StatusSyncInterval.Duration != 5*time.Second {
					t.Errorf("the expected StatusSyncInterval of klusterlet is %v, but got %v",
						5*time.Second, klusterlet.Spec.WorkConfiguration.StatusSyncInterval.Duration)
				}
			},
		},
		{
			name: "with GRPC registration driver",
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
						Name:      "test-image-pull-secret",
						Namespace: "multicluster-engine", // Put secret in the correct namespace for GRPC tests
					},
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
				&routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      grpcRouteName,
						Namespace: helpers.HubNamespace,
					},
					Spec: routev1.RouteSpec{
						Host: "grpc-server.apps.example.com",
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      grpcCAConfigmap,
						Namespace: helpers.HubNamespace,
					},
					Data: map[string]string{
						"ca-bundle.crt": "-----BEGIN CERTIFICATE-----\nMIIDtest\n-----END CERTIFICATE-----",
					},
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				[]byte("apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://api.example.com:6443\n  name: test\ncontexts:\n- context:\n    cluster: test\n    user: test\n  name: test\ncurrent-context: test\nusers:\n- name: test\n  user:\n    token: test-token-123"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					RegistrationDriver: &operatorv1.RegistrationDriver{
						AuthType: "grpc",
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
				if klusterlet.Spec.RegistrationConfiguration.RegistrationDriver.AuthType != "grpc" {
					t.Errorf("the klusterlet registration driver auth type should be grpc, but got %s",
						klusterlet.Spec.RegistrationConfiguration.RegistrationDriver.AuthType)
				}
			},
		},
		{
			name: "with GRPC registration driver but route not found",
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
						Name:      "test-image-pull-secret",
						Namespace: "multicluster-engine", // Put secret in the correct namespace for GRPC tests
					},
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte("fake-token"),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      grpcCAConfigmap,
						Namespace: helpers.HubNamespace,
					},
					Data: map[string]string{
						"ca-bundle.crt": "-----BEGIN CERTIFICATE-----\nMIIDtest\n-----END CERTIFICATE-----",
					},
				},
			},
			config: NewKlusterletManifestsConfig(
				operatorv1.InstallModeDefault,
				"test", // cluster name
				[]byte("apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://api.example.com:6443\n  name: test\ncontexts:\n- context:\n    cluster: test\n    user: test\n  name: test\ncurrent-context: test\nusers:\n- name: test\n  user:\n    token: test-token-123"),
			).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					RegistrationDriver: &operatorv1.RegistrationDriver{
						AuthType: "grpc",
					},
				},
			}),
			expectError:  true,
			errorMessage: "failed to get GRPC config yaml:",
			validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
				// Should not be called for error cases
			},
		},
	{
		name: "hosted with nodePlacement from klusterletConfig",
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
		).WithoutImagePullSecretGenerate().WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
			Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
				NodePlacement: &operatorv1.NodePlacement{
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:               "node.kubernetes.io/hosted",
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: &tolerationSeconds,
						},
					},
				},
			},
		}),
		validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
			testinghelpers.ValidateObjectCount(t, objects, 3)
			testinghelpers.ValidateCRDs(t, crds, 0)
			testinghelpers.ValidateKlusterlet(t, objects[2], operatorv1.InstallModeHosted,
				"klusterlet-test", "test", "open-cluster-management-test")
			klusterlet, _ := objects[2].(*operatorv1.Klusterlet)
			if klusterlet.Spec.NodePlacement.NodeSelector["kubernetes.io/os"] != "linux" {
				t.Errorf("the klusterlet node selector %s is not %s",
					klusterlet.Spec.NodePlacement.NodeSelector["kubernetes.io/os"], "linux")
			}
			if klusterlet.Spec.NodePlacement.Tolerations[0].Key != "node.kubernetes.io/hosted" {
				t.Errorf("the klusterlet tolerations %s is not %s",
					klusterlet.Spec.NodePlacement.Tolerations[0].Key, "node.kubernetes.io/hosted")
			}
		},
	},
	{
		name: "hosted with pullSecret and registries from klusterletConfig",
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
					Name:      "custom-pull-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte("custom-fake-token"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
		},
		config: NewKlusterletManifestsConfig(
			operatorv1.InstallModeHosted,
			"test", // cluster name
			[]byte("bootstrap kubeconfig"),
		).WithKlusterletConfig(&klusterletconfigv1alpha1.KlusterletConfig{
			Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
				PullSecret: corev1.ObjectReference{
					Name:      "custom-pull-secret",
					Namespace: "default",
				},
				Registries: []klusterletconfigv1alpha1.Registries{
					{
						Source: "quay.io/open-cluster-management",
						Mirror: "quay.io/rhacm2",
					},
					{
						Source: "quay.io/stolostron",
						Mirror: "quay.io/rhacm2",
					},
				},
			},
		}),
		validateFunc: func(t *testing.T, objects, crds []runtime.Object) {
			testinghelpers.ValidateObjectCount(t, objects, 4)
			testinghelpers.ValidateCRDs(t, crds, 0)

			// Find the klusterlet object
			var klusterlet *operatorv1.Klusterlet
			var imagePullSecretIdx int
			for i, obj := range objects {
				if k, ok := obj.(*operatorv1.Klusterlet); ok {
					klusterlet = k
				}
				if s, ok := obj.(*corev1.Secret); ok {
					if s.Type == corev1.SecretTypeDockerConfigJson {
						imagePullSecretIdx = i
					}
				}
			}

			if klusterlet == nil {
				t.Fatal("klusterlet not found in objects")
			}

			// Verify klusterlet properties
			if klusterlet.Name != "klusterlet-test" {
				t.Errorf("expected klusterlet name klusterlet-test, got %s", klusterlet.Name)
			}
			if klusterlet.Spec.ClusterName != "test" {
				t.Errorf("expected cluster name test, got %s", klusterlet.Spec.ClusterName)
			}

			// Verify that custom registries are applied
			if !strings.HasPrefix(klusterlet.Spec.RegistrationImagePullSpec, "quay.io/rhacm2/registration") {
				t.Errorf("the klusterlet registration image pull spec %s does not use custom registry",
					klusterlet.Spec.RegistrationImagePullSpec)
			}
			if !strings.HasPrefix(klusterlet.Spec.WorkImagePullSpec, "quay.io/rhacm2/work") {
				t.Errorf("the klusterlet work image pull spec %s does not use custom registry",
					klusterlet.Spec.WorkImagePullSpec)
			}
			// Verify that custom pull secret is applied
			testinghelpers.ValidateImagePullSecret(t, objects[imagePullSecretIdx], "open-cluster-management-test", "custom-fake-token")
		},
	},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			os.Setenv(constants.DefaultImagePullSecretEnvVarName, testcase.defaultImagePullSecret)

			// Set POD_NAMESPACE only for GRPC tests
			if strings.Contains(testcase.name, "GRPC") {
				os.Setenv(constants.PodNamespaceEnvVarName, "multicluster-engine")
			}

			// Separate route objects from Kubernetes objects for GRPC tests
			var routes []runtime.Object
			var kubeObjs []runtime.Object
			hasRoutes := false
			for _, obj := range testcase.runtimeObjs {
				if route, ok := obj.(*routev1.Route); ok {
					routes = append(routes, route)
					hasRoutes = true
				} else {
					kubeObjs = append(kubeObjs, obj)
				}
			}

			// Use original objects if no routes, otherwise use separated objects
			var kubeClient *kubefake.Clientset
			if hasRoutes {
				kubeClient = kubefake.NewSimpleClientset(kubeObjs...)
			} else {
				kubeClient = kubefake.NewSimpleClientset(testcase.runtimeObjs...)
			}

			clientHolder := &helpers.ClientHolder{
				KubeClient:          kubeClient,
				RuntimeClient:       fake.NewClientBuilder().WithScheme(testscheme).WithObjects(testcase.clientObjs...).Build(),
				ImageRegistryClient: imageregistry.NewClient(kubeClient),
			}

			// Add route client for GRPC tests (both success and error cases)
			if hasRoutes || strings.Contains(testcase.name, "GRPC") {
				if hasRoutes {
					clientHolder.RouteV1Client = routefake.NewSimpleClientset(routes...)
				} else {
					clientHolder.RouteV1Client = routefake.NewSimpleClientset()
				}
			}

			manifestsBytes, crdBytes, valuesBytes, err := testcase.config.Generate(context.Background(), clientHolder)
			if testcase.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", testcase.name)
					return
				}
				if !strings.Contains(err.Error(), testcase.errorMessage) {
					t.Errorf("%s: expected error message to contain %q, but got %v", testcase.name, testcase.errorMessage, err)
				}
				return
			}
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
			if testcase.validateFunc != nil {
				testcase.validateFunc(t, objs, crdObjs)
			}

			klusterletChartConfig := &chart.KlusterletChartConfig{}
			err = yaml.Unmarshal(valuesBytes, klusterletChartConfig)
			if err != nil {
				t.Fatalf("%s Failed to unmarshal values: %v", testcase.name, err)
			}
		})
	}

}

func TestBuildGRPCConfigData(t *testing.T) {
	testcases := []struct {
		name               string
		token              string
		route              *routev1.Route
		createRoute        bool
		expectedGRPCConfig string
		expectError        bool
		errorMessage       string
	}{
		{
			name:        "with customized GRPCConfig and token",
			token:       "test-token",
			createRoute: true,
			route: &routev1.Route{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      grpcRouteName,
					Namespace: helpers.HubNamespace,
				},
				Spec: routev1.RouteSpec{
					Host: "grpc.config.com",
				},
			},
			expectedGRPCConfig: "caData: Y2FEYXRh\nkeepAliveConfig: {}\ntoken: test-token\nurl: grpc.config.com\n",
		},
		{
			name:        "with customized GRPCConfig without token",
			token:       "",
			createRoute: true,
			route: &routev1.Route{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      grpcRouteName,
					Namespace: helpers.HubNamespace,
				},
				Spec: routev1.RouteSpec{
					Host: "grpc.example.com",
				},
			},
			expectedGRPCConfig: "caData: Y2FEYXRh\nkeepAliveConfig: {}\ntoken: \"\"\nurl: grpc.example.com\n",
		},
		{
			name:        "route with empty host",
			token:       "test-token",
			createRoute: true,
			route: &routev1.Route{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      grpcRouteName,
					Namespace: helpers.HubNamespace,
				},
				Spec: routev1.RouteSpec{
					Host: "",
				},
			},
			expectError:  true,
			errorMessage: "grpc-server route has no host specified",
		},
		{
			name:         "route not found",
			token:        "test-token",
			createRoute:  false,
			expectError:  true,
			errorMessage: "failed to get grpc-server route",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			var routeClient *routefake.Clientset
			if testcase.createRoute {
				routeClient = routefake.NewSimpleClientset(testcase.route)
			} else {
				routeClient = routefake.NewSimpleClientset()
			}

			clientHolder := &helpers.ClientHolder{RouteV1Client: routeClient}

			grpcConfigData, err := buildGRPCConfigData(context.TODO(), clientHolder, testcase.token, []byte("caData"))
			if testcase.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), testcase.errorMessage) {
					t.Errorf("expected error message to contain %q, but got %v", testcase.errorMessage, err)
				}
			} else {
				if err != nil {
					t.Errorf("failed to build grpc config data: %v", err)
				}
				if grpcConfigData != testcase.expectedGRPCConfig {
					t.Errorf("expected grpc config data to be %q, but got %q", testcase.expectedGRPCConfig, grpcConfigData)
				}
			}
		})
	}
}

func TestGetGRCPCaBundleFromConfigMap(t *testing.T) {
	testcases := []struct {
		name            string
		createConfigMap bool
		configMap       *corev1.ConfigMap
		expectedData    string
		expectError     bool
		errorMessage    string
	}{
		{
			name:            "successful ca bundle retrieval",
			createConfigMap: true,
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grpcCAConfigmap,
					Namespace: helpers.HubNamespace,
				},
				Data: map[string]string{
					"ca-bundle.crt": "-----BEGIN CERTIFICATE-----\nMIIDtest\n-----END CERTIFICATE-----",
				},
			},
			expectedData: "-----BEGIN CERTIFICATE-----\nMIIDtest\n-----END CERTIFICATE-----",
		},
		{
			name:            "configmap not found",
			createConfigMap: false,
			expectError:     true,
			errorMessage:    "failed to get ca-bundle-configmap from hub namespace",
		},
		{
			name:            "ca-bundle.crt key missing",
			createConfigMap: true,
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grpcCAConfigmap,
					Namespace: helpers.HubNamespace,
				},
				Data: map[string]string{
					"other-key": "some-value",
				},
			},
			expectError:  true,
			errorMessage: "ca-bundle.crt key not found in configmap ca-bundle-configmap",
		},
		{
			name:            "empty ca bundle data",
			createConfigMap: true,
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grpcCAConfigmap,
					Namespace: helpers.HubNamespace,
				},
				Data: map[string]string{
					"ca-bundle.crt": "",
				},
			},
			expectedData: "",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			var kubeClient *kubefake.Clientset
			if testcase.createConfigMap {
				kubeClient = kubefake.NewSimpleClientset(testcase.configMap)
			} else {
				kubeClient = kubefake.NewSimpleClientset()
			}

			clientHolder := &helpers.ClientHolder{
				KubeClient: kubeClient,
			}

			caBundleData, err := GetGRCPCaBundleFromConfigMap(context.Background(), clientHolder)
			if testcase.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), testcase.errorMessage) {
					t.Errorf("expected error message to contain %q, but got %v", testcase.errorMessage, err)
				}
			} else {
				if err != nil {
					t.Errorf("failed to get ca bundle from configmap: %v", err)
				}
				if string(caBundleData) != testcase.expectedData {
					t.Errorf("expected ca bundle data to be %q, but got %q", testcase.expectedData, string(caBundleData))
				}
			}
		})
	}
}
