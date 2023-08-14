package importconfig

import (
	"context"
	"os"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenerateAgentRegistrationManifests(t *testing.T) {
	cases := []struct {
		name         string
		clientObjs   []runtimeclient.Object
		runtimeObjs  []runtime.Object
		validateFunc func(t *testing.T, data []byte, err error)
	}{
		{
			name: "success",
			clientObjs: []runtimeclient.Object{
				&corev1.Namespace{
					ObjectMeta: v1.ObjectMeta{
						Name: "cluster-secret", // the same with PodNamespaceEnvVarName
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
					ObjectMeta: v1.ObjectMeta{
						Name:      AgentRegistrationDefaultBootstrapSAName,
						Namespace: "cluster-secret",
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      AgentRegistrationDefaultBootstrapSAName + "-token-abcde",
							Namespace: "cluster-secret",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: v1.ObjectMeta{
						Name:      AgentRegistrationDefaultBootstrapSAName + "-token-abcde",
						Namespace: "cluster-secret",
					},
					Data: map[string][]byte{
						"token": []byte("abcde"),
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
						Namespace: "cluster-secret",
					},
					Data: map[string]string{
						"ca.crt": "fake-root-ca",
					},
				},
			},
			validateFunc: func(t *testing.T, data []byte, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(data) == 0 {
					t.Fatalf("expected data to be non-empty")
				}

				objs := []runtime.Object{}
				for _, yaml := range helpers.SplitYamls(data) {
					objs = append(objs, helpers.MustCreateObject(yaml))
				}

				if len(objs) < 1 {
					t.Errorf("resources data %s, objs is empty: %v", constants.ImportSecretImportYamlKey, objs)
				}
				ns, ok := objs[0].(*corev1.Namespace)
				if !ok {
					t.Errorf("resources data %s, the first element is not namespace", constants.ImportSecretImportYamlKey)
				}

				if ns.Name != defaultKlusterletNamespace {
					t.Errorf("resources data %s, the namespace name %s is not %s", constants.ImportSecretImportYamlKey, ns.Name, defaultKlusterletNamespace)
				}

				pullSecret, ok := objs[9].(*corev1.Secret)
				if !ok {
					t.Errorf("resources data %s, the last element is not secret", constants.ImportSecretImportYamlKey)
				}
				if pullSecret.Type != corev1.SecretTypeDockerConfigJson {
					t.Errorf("resources data %s, the pull secret type %s is not %s", constants.ImportSecretImportYamlKey,
						pullSecret.Type, corev1.SecretTypeDockerConfigJson)
				}
				if _, ok := pullSecret.Data[corev1.DockerConfigJsonKey]; !ok {
					t.Errorf("resources data %s, the pull secret data %s is not %s", constants.ImportSecretImportYamlKey,
						pullSecret.Data, corev1.DockerConfigJsonKey)
				}

				klusterlet, ok := objs[8].(*operatorv1.Klusterlet)
				if !ok {
					t.Errorf("resources data %s, the last element is not klusterlet", constants.ImportSecretImportYamlKey)
				}
				if klusterlet.Spec.RegistrationConfiguration.ClusterAnnotations == nil {
					t.Errorf("resources data %s, the klusterlet annotations is nil", constants.ImportSecretImportYamlKey)
				}
				if klusterlet.Spec.RegistrationConfiguration.ClusterAnnotations["agent.open-cluster-management.io/create-with-default-klusterletaddonconfig"] != "true" {
					t.Errorf("resources data %s, the klusterlet annotations is not true", constants.ImportSecretImportYamlKey)
				}

				if len(strings.Split(strings.Replace(string(data), constants.YamlSperator, "", 1), constants.YamlSperator)) != 10 {
					t.Errorf("expect 10 files, but failed")
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

			data, err := GenerateAgentRegistrationManifests(context.TODO(), clientHolder, "test-cluster")
			c.validateFunc(t, data, err)
		})
	}
}
