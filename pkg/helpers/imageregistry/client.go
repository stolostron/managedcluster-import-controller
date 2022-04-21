// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package imageregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	// ClusterImageRegistriesAnnotation value is a json string of ImageRegistries
	ClusterImageRegistriesAnnotation = "open-cluster-management.io/image-registries"
)

type Registry struct {
	// Mirror is the mirrored registry of the Source. Will be ignored if Mirror is empty.
	Mirror string `json:"mirror"`

	// Source is the source registry. All image registries will be replaced by Mirror if Source is empty.
	Source string `json:"source"`
}

// ImageRegistries is value of the image registries annotation includes the mirror and source registries.
// The source registry will be replaced by the Mirror.
// The larger index will work if the Sources are the same.
type ImageRegistries struct {
	PullSecret string     `json:"pullSecret"`
	Registries []Registry `json:"registries"`
}

type Interface interface {
	Cluster(cluster *clusterv1.ManagedCluster) Interface
	PullSecret() (*corev1.Secret, error)
	ImageOverride(imageName string) string
}

type Client struct {
	kubeClient      kubernetes.Interface
	imageRegistries ImageRegistries
}

func NewClient(kubeClient kubernetes.Interface) Interface {
	return &Client{
		kubeClient:      kubeClient,
		imageRegistries: ImageRegistries{},
	}
}

func (c *Client) Cluster(cluster *clusterv1.ManagedCluster) Interface {
	c.imageRegistries = ImageRegistries{}
	if cluster == nil {
		return c
	}
	annotations := cluster.GetAnnotations()
	if len(annotations) == 0 {
		return c
	}

	err := json.Unmarshal([]byte(annotations[ClusterImageRegistriesAnnotation]), &c.imageRegistries)
	if err != nil {
		klog.Errorf("failed to unmarshal imageRegistries from annotation. err: %v", err)
	}
	return c
}

func (c *Client) PullSecret() (*corev1.Secret, error) {
	if c.imageRegistries.PullSecret == "" {
		return nil, nil
	}
	segs := strings.Split(c.imageRegistries.PullSecret, ".")
	if len(segs) != 2 {
		return nil, fmt.Errorf("wrong pullSecret format %v in the annotation %s",
			c.imageRegistries.PullSecret, ClusterImageRegistriesAnnotation)
	}
	namespace := segs[0]
	pullSecret := segs[1]
	return c.kubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), pullSecret, metav1.GetOptions{})
}

// ImageOverride is to override the image by image-registries annotation of managedCluster.
// The source registry will be replaced by the Mirror.
// The larger index will work if the Sources are the same.
func (c *Client) ImageOverride(imageName string) string {
	if len(c.imageRegistries.Registries) == 0 {
		return imageName
	}
	overrideImageName := imageName
	for i := 0; i < len(c.imageRegistries.Registries); i++ {
		registry := c.imageRegistries.Registries[i]
		name := imageOverride(registry.Source, registry.Mirror, imageName)
		if name != imageName {
			overrideImageName = name
		}
	}
	return overrideImageName
}

func imageOverride(source, mirror, imageName string) string {
	source = strings.TrimSuffix(source, "/")
	mirror = strings.TrimSuffix(mirror, "/")
	imageSegments := strings.Split(imageName, "/")
	imageNameTag := imageSegments[len(imageSegments)-1]
	if source == "" {
		if mirror == "" {
			return imageNameTag
		}
		return fmt.Sprintf("%s/%s", mirror, imageNameTag)
	}

	if !strings.HasPrefix(imageName, source) {
		return imageName
	}

	trimSegment := strings.TrimPrefix(imageName, source)
	return fmt.Sprintf("%s%s", mirror, trimSegment)
}

// OverrideImageByAnnotation is to override the image by image-registries annotation of managedCluster.
// The source registry will be replaced by the Mirror.
// The larger index will work if the Sources are the same.
func OverrideImageByAnnotation(annotations map[string]string, imageName string) string {
	if len(annotations) == 0 {
		return imageName
	}

	if annotations[ClusterImageRegistriesAnnotation] == "" {
		return imageName
	}

	imageRegistries := ImageRegistries{}
	err := json.Unmarshal([]byte(annotations[ClusterImageRegistriesAnnotation]), &imageRegistries)
	if err != nil {
		klog.Errorf("failed to unmarshal the annotation %v,err %v", ClusterImageRegistriesAnnotation, err)
		return imageName
	}

	if len(imageRegistries.Registries) == 0 {
		return imageName
	}
	overrideImageName := imageName
	for i := 0; i < len(imageRegistries.Registries); i++ {
		registry := imageRegistries.Registries[i]
		name := imageOverride(registry.Source, registry.Mirror, imageName)
		if name != imageName {
			overrideImageName = name
		}
	}
	return overrideImageName
}
