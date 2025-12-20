package utils

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	workv1 "open-cluster-management.io/api/work/v1"
)

func TestEncodeManifests(t *testing.T) {
	cases := []struct {
		name             string
		work             *workv1.ManifestWork
		expectedManifest runtime.Object
	}{
		{
			name: "the manifest of a work does not have raw",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							{
								RawExtension: runtime.RawExtension{
									Object: configMap(),
								},
							},
						},
					},
				},
			},
			expectedManifest: configMap(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := EncodeManifests(c.work)
			if err != nil {
				t.Errorf("unexpected error %v", err)
			}

			manifest := c.work.Spec.Workload.Manifests[0]
			if manifest.Raw == nil {
				t.Errorf("the raw is nil")
			}

			cm := &corev1.ConfigMap{}
			if err := json.Unmarshal(manifest.Raw, cm); err != nil {
				t.Errorf("unexpected error %v", err)
			}

			if !equality.Semantic.DeepEqual(cm, c.expectedManifest) {
				t.Errorf("expected %v, but got %v", c.expectedManifest, cm)
			}
		})
	}
}

func configMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Data: map[string]string{
			"test": "test",
		},
	}
}
