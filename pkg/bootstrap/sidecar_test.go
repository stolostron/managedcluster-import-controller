// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"os"
	"testing"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/yaml"
)

func TestIsManagedClusterOpenShift(t *testing.T) {
	tests := []struct {
		name string
		mc   *clusterv1.ManagedCluster
		want bool
	}{
		{
			name: "nil managed cluster",
			mc:   nil,
			want: false,
		},
		{
			name: "no labels",
			mc:   &clusterv1.ManagedCluster{},
			want: false,
		},
		{
			name: "vendor OpenShift",
			mc: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"vendor": "OpenShift"},
				},
			},
			want: true,
		},
		{
			name: "vendor Kubernetes",
			mc: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"vendor": "Kubernetes"},
				},
			},
			want: false,
		},
		{
			name: "vendor empty",
			mc: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"vendor": ""},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isManagedClusterOpenShift(tt.mc); got != tt.want {
				t.Errorf("isManagedClusterOpenShift() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTLSProfileSyncImage(t *testing.T) {
	tests := []struct {
		name               string
		envValue           string
		kcRegistries       []klusterletconfigv1alpha1.Registries
		clusterAnnotations map[string]string
		wantImage          string
		wantErr            bool
	}{
		{
			name:      "env var set, no overrides",
			envValue:  "quay.io/ocm/import-controller:v1",
			wantImage: "quay.io/ocm/import-controller:v1",
		},
		{
			name:     "env var not set",
			envValue: "",
			wantErr:  true,
		},
		{
			name:     "env var set with registry override",
			envValue: "quay.io/ocm/import-controller:v1",
			kcRegistries: []klusterletconfigv1alpha1.Registries{
				{Source: "quay.io/ocm", Mirror: "mirror.example.com/ocm"},
			},
			wantImage: "mirror.example.com/ocm/import-controller:v1",
		},
		{
			name:     "env var set with non-matching registry",
			envValue: "quay.io/ocm/import-controller:v1",
			kcRegistries: []klusterletconfigv1alpha1.Registries{
				{Source: "docker.io/other", Mirror: "mirror.example.com/other"},
			},
			wantImage: "quay.io/ocm/import-controller:v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(constants.TLSProfileSyncImageEnvVarName, tt.envValue)
			} else {
				os.Unsetenv(constants.TLSProfileSyncImageEnvVarName)
			}
			defer os.Unsetenv(constants.TLSProfileSyncImageEnvVarName)

			got, err := getTLSProfileSyncImage(tt.kcRegistries, tt.clusterAnnotations)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTLSProfileSyncImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantImage {
				t.Errorf("getTLSProfileSyncImage() = %q, want %q", got, tt.wantImage)
			}
		})
	}
}

func TestInjectTLSProfileSyncSidecar(t *testing.T) {
	secCtx := corev1.SecurityContext{
		RunAsNonRoot:             boolPtr(true),
		ReadOnlyRootFilesystem:   boolPtr(true),
		AllowPrivilegeEscalation: boolPtr(false),
	}

	t.Run("injects sidecar into klusterlet deployment", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "klusterlet",
				Namespace: "open-cluster-management-agent",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "klusterlet", Image: "reg-operator:latest"},
						},
					},
				},
			},
		}
		depBytes, _ := yaml.Marshal(deployment)

		serviceAccount := []byte(
			"apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: klusterlet\n")

		objects := [][]byte{serviceAccount, depBytes}

		result, err := injectTLSProfileSyncSidecar(
			objects, "quay.io/ocm/import-controller:v1", secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if string(result[0]) != string(serviceAccount) {
			t.Error("ServiceAccount was modified unexpectedly")
		}

		modified := &appsv1.Deployment{}
		if err := yaml.Unmarshal(result[1], modified); err != nil {
			t.Fatalf("failed to unmarshal modified deployment: %v", err)
		}
		if len(modified.Spec.Template.Spec.Containers) != 2 {
			t.Fatalf("expected 2 containers, got %d",
				len(modified.Spec.Template.Spec.Containers))
		}

		sidecar := modified.Spec.Template.Spec.Containers[1]
		if sidecar.Name != "tls-profile-sync" {
			t.Errorf("sidecar name = %q, want tls-profile-sync", sidecar.Name)
		}
		if sidecar.Image != "quay.io/ocm/import-controller:v1" {
			t.Errorf("sidecar image = %q, want quay.io/ocm/import-controller:v1",
				sidecar.Image)
		}
		if len(sidecar.Command) != 1 ||
			sidecar.Command[0] != "/usr/local/bin/tls-profile-sync" {
			t.Errorf("sidecar command = %v, want [/usr/local/bin/tls-profile-sync]",
				sidecar.Command)
		}
		if len(sidecar.Env) != 1 || sidecar.Env[0].Name != "POD_NAMESPACE" {
			t.Errorf("sidecar env = %v, want POD_NAMESPACE", sidecar.Env)
		}
		if sidecar.SecurityContext == nil ||
			*sidecar.SecurityContext.RunAsNonRoot != true {
			t.Error("sidecar security context not set correctly")
		}
	})

	t.Run("no deployment in objects", func(t *testing.T) {
		sa := []byte(
			"apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: klusterlet\n")
		objects := [][]byte{sa}

		result, err := injectTLSProfileSyncSidecar(
			objects, "quay.io/ocm/import-controller:v1", secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(result[0]) != string(sa) {
			t.Error("objects were modified when no deployment present")
		}
	})

	t.Run("non-klusterlet deployment is not modified", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "other-deployment"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "some-image:latest"},
						},
					},
				},
			},
		}
		depBytes, _ := yaml.Marshal(deployment)
		objects := [][]byte{depBytes}

		result, err := injectTLSProfileSyncSidecar(
			objects, "quay.io/ocm/import-controller:v1", secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		modified := &appsv1.Deployment{}
		if err := yaml.Unmarshal(result[0], modified); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(modified.Spec.Template.Spec.Containers) != 1 {
			t.Errorf("expected 1 container, got %d",
				len(modified.Spec.Template.Spec.Containers))
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}
