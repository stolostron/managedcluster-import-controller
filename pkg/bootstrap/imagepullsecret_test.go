// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"context"
	"os"
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestGetImagePullSecret(t *testing.T) {
	cases := []struct {
		name                            string
		clientObjs                      []client.Object
		secret                          *corev1.Secret
		managedCluster                  *clusterv1.ManagedCluster
		klusterletconfigImagePullSecret corev1.ObjectReference
		expectedSecret                  *corev1.Secret
		defaultImagePullSecret          string
	}{
		{
			name:       "no registry",
			clientObjs: []client.Object{},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-image-pull-secret",
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("fake-token"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
			defaultImagePullSecret: "test-image-pull-secret",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			klusterletconfigImagePullSecret: corev1.ObjectReference{},
			expectedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-image-pull-secret",
				},
			},
		},
		{
			name:       "no registry, no default secret found in env var",
			clientObjs: []client.Object{},
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			defaultImagePullSecret:          "",
			klusterletconfigImagePullSecret: corev1.ObjectReference{},
			expectedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: EmptyImagePullSecret,
				},
			},
		},
		{
			name:       "has registry",
			clientObjs: []client.Object{},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test1",
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("fake-token"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
			defaultImagePullSecret: "test-image-pull-secret",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"open-cluster-management.io/image-registry": "test1.test2",
					},
					Annotations: map[string]string{
						"open-cluster-management.io/image-registries": `{"pullSecret":"test1.test"}`,
					},
				},
			},
			klusterletconfigImagePullSecret: corev1.ObjectReference{},
			expectedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test1",
				},
			},
		},
		{
			name:       "with klusterletconfig image pull secret",
			clientObjs: []client.Object{},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test1",
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("fake-token"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
			defaultImagePullSecret: "test-image-pull-secret",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			klusterletconfigImagePullSecret: corev1.ObjectReference{
				Name:      "test",
				Namespace: "test1",
			},
			expectedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test1",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			os.Setenv(constants.DefaultImagePullSecretEnvVarName, c.defaultImagePullSecret)

			var kubeClient *kubefake.Clientset
			if c.secret != nil {
				kubeClient = kubefake.NewSimpleClientset(c.secret)
			} else {
				kubeClient = kubefake.NewSimpleClientset()
			}
			clientHolder := &helpers.ClientHolder{
				KubeClient:          kubeClient,
				ImageRegistryClient: imageregistry.NewClient(kubeClient),
			}

			secret, err := getImagePullSecret(context.Background(), clientHolder, c.klusterletconfigImagePullSecret, c.managedCluster.Annotations)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if secret.Name != c.expectedSecret.Name {
				t.Errorf("expected secret %v, but got %v", c.expectedSecret.Name, secret.Name)
			}
		})
	}
}
