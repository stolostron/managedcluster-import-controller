// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package bootstrap

import (
	"fmt"
	"os"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/yaml"
)

// isManagedClusterOpenShift returns true if the managed cluster is an OpenShift cluster.
func isManagedClusterOpenShift(mc *clusterv1.ManagedCluster) bool {
	if mc == nil {
		return false
	}
	return mc.Labels["vendor"] == "OpenShift"
}

// getTLSProfileSyncImage returns the tls-profile-sync sidecar image, applying registry
// overrides from KlusterletConfig or managed cluster annotations.
func getTLSProfileSyncImage(
	kcRegistries []klusterletconfigv1alpha1.Registries,
	clusterAnnotations map[string]string,
) (string, error) {
	defaultImage := os.Getenv(constants.TLSProfileSyncImageEnvVarName)
	if defaultImage == "" {
		return "", fmt.Errorf("environment variable %s not defined",
			constants.TLSProfileSyncImageEnvVarName)
	}

	if len(kcRegistries) != 0 {
		for i := 0; i < len(kcRegistries); i++ {
			name := imageOverride(kcRegistries[i].Source, kcRegistries[i].Mirror, defaultImage)
			if name != defaultImage {
				return name, nil
			}
		}
		return defaultImage, nil
	}

	return imageregistry.OverrideImageByAnnotation(clusterAnnotations, defaultImage)
}

// injectTLSProfileSyncSidecar finds the klusterlet-operator Deployment in the rendered
// objects and appends a tls-profile-sync sidecar container. Returns the modified objects slice.
func injectTLSProfileSyncSidecar(
	objects [][]byte,
	image string,
	securityContext corev1.SecurityContext,
) ([][]byte, error) {
	for i, obj := range objects {
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(obj, u); err != nil {
			continue
		}
		if u.GetKind() != "Deployment" || u.GetName() != "klusterlet" {
			continue
		}

		deployment := &appsv1.Deployment{}
		if err := yaml.Unmarshal(obj, deployment); err != nil {
			return nil, fmt.Errorf("failed to unmarshal klusterlet Deployment: %w", err)
		}

		sidecar := corev1.Container{
			Name:            "tls-profile-sync",
			Image:           image,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/usr/local/bin/tls-profile-sync"},
			Env: []corev1.EnvVar{
				{
					Name: "POD_NAMESPACE",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.namespace",
						},
					},
				},
			},
			SecurityContext: &securityContext,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("32Mi"),
					corev1.ResourceCPU:    resource.MustParse("10m"),
				},
			},
		}

		deployment.Spec.Template.Spec.Containers = append(
			deployment.Spec.Template.Spec.Containers, sidecar)

		modified, err := yaml.Marshal(deployment)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal modified klusterlet Deployment: %w", err)
		}

		objects[i] = modified
		break
	}

	return objects, nil
}
