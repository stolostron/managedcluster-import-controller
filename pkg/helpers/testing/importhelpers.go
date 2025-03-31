package testinghelpers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

func ValidateObjectCount(t *testing.T, objs []runtime.Object, expectedCount int) {
	if len(objs) != expectedCount {
		t.Errorf("expected %d objects, but got %d", expectedCount, len(objs))
	}
}
func ValidateCRDs(t *testing.T, objs []runtime.Object, expectedCount int) {
	if len(objs) != expectedCount {
		t.Errorf("expected %d crd, but got %d", expectedCount, (objs))
	}
}
func ValidateNamespace(t *testing.T, obj runtime.Object, expectedNamespace string) {
	namespace, ok := obj.(*corev1.Namespace)
	if !ok {
		t.Errorf("expected namespace object, but got %#v", obj)
	}
	if namespace.Name != expectedNamespace {
		t.Errorf("expected namespace %s, but got %s", expectedNamespace, namespace.Name)
	}
}

func ValidateBoostrapSecret(t *testing.T, obj runtime.Object, expectedName, expectedNamespace, expectedData string) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		t.Errorf("expected secret, but got %#v", obj)
	}
	if secret.Name != expectedName {
		t.Errorf("expected secret %s, but got %s", expectedName, secret.Name)
	}
	if secret.Namespace != expectedNamespace {
		t.Errorf("expected secret %s, but got %s", expectedNamespace, secret.Namespace)
	}
	if secret.Type != corev1.SecretTypeOpaque {
		t.Errorf("expected bootstrap secret, but got %#v", secret)
	}
	if data := secret.Data["kubeconfig"]; string(data) != expectedData {
		t.Errorf("expected bootstrap secret data %v, but got %#v", expectedData, string(data))
	}
}

func ValidateImagePullSecret(t *testing.T, obj runtime.Object, expectedNamespace, expectedData string) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		t.Errorf("expected secret, but got %#v", obj)
	}
	if secret.Name != "open-cluster-management-image-pull-credentials" {
		t.Errorf("expected secret %s, but got %s", "open-cluster-management-image-pull-credentials", secret.Name)
	}
	if secret.Namespace != expectedNamespace {
		t.Errorf("expected secret %s, but got %s", expectedNamespace, secret.Namespace)
	}
	if secret.Type != corev1.SecretTypeDockerConfigJson {
		t.Errorf("expected image pull secret, but got %#v", secret)
	}
	if data := secret.Data[corev1.DockerConfigJsonKey]; string(data) != expectedData {
		t.Errorf("the image pull secret data is not expected. %#v", data)
	}
}

func ValidateKlusterlet(t *testing.T, obj runtime.Object, installMode operatorv1.InstallMode,
	expectedName, expectedClusterName, expectedNamespace string) {
	klusterlet, ok := obj.(*operatorv1.Klusterlet)
	if !ok {
		t.Errorf("expected klusterlet, but got %#v", obj)
	}
	if klusterlet.Name != expectedName {
		t.Errorf("expected klusterlet %s, but got %s", expectedName, klusterlet.Name)
	}
	if klusterlet.Spec.Namespace != expectedNamespace {
		t.Errorf("expected klusterlet namespace %s, but got %s", expectedNamespace, klusterlet.Spec.Namespace)
	}
	if klusterlet.Spec.ClusterName != expectedClusterName {
		t.Errorf("expected klusterlet cluster %s, but got %s", expectedClusterName, klusterlet.Spec.ClusterName)
	}
	if klusterlet.Spec.DeployOption.Mode != installMode {
		t.Errorf("expected install mode %s, but got %s", installMode, klusterlet.Spec.DeployOption.Mode)
	}
}
