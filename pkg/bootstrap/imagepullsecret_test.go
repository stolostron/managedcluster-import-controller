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

func init() {
	os.Setenv(constants.DefaultImagePullSecretEnvVarName, "test-image-pull-secret-secret") // this is also depend by render_test.go
}

func TestGetImagePullSecret(t *testing.T) {
	cases := []struct {
		name           string
		clientObjs     []client.Object
		secret         *corev1.Secret
		managedCluster *clusterv1.ManagedCluster
		expectedSecret *corev1.Secret
	}{
		{
			name:       "no registry",
			clientObjs: []client.Object{},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
					Namespace: os.Getenv("POD_NAMESPACE"),
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte("fake-token"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			expectedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
					Namespace: os.Getenv("POD_NAMESPACE"),
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
			kubeClient := kubefake.NewSimpleClientset(c.secret)
			clientHolder := &helpers.ClientHolder{
				KubeClient:          kubeClient,
				ImageRegistryClient: imageregistry.NewClient(kubeClient),
			}

			secret, err := getImagePullSecret(context.Background(), clientHolder, c.managedCluster.Annotations)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if secret.Name != c.expectedSecret.Name {
				t.Errorf("expected secret %v, but got %v", c.expectedSecret.Name, secret.Name)
			}
		})
	}
}
