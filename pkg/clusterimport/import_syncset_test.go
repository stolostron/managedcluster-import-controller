//Package clusterimport ...
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package clusterimport

import (
	"testing"

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestEqualRawExtensions(t *testing.T) {
	baseRawExtension1 := runtime.RawExtension{
		Object: &multicloudv1alpha1.EndpointConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "endpointConfig1",
				Namespace: "namespace",
			}},
	}
	baseRawExtension2 := runtime.RawExtension{
		Object: &multicloudv1alpha1.EndpointConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "endpointConfig2",
				Namespace: "namespace",
			}},
	}
	jsonRawExtension1 := runtime.RawExtension{}
	bytes, err := baseRawExtension1.MarshalJSON()
	if err != nil {
		t.Errorf("failed to convert rawExtension")
	}
	jsonRawExtension1.UnmarshalJSON(bytes)

	tests := []struct {
		name    string
		a       runtime.RawExtension
		b       runtime.RawExtension
		isEqual bool
	}{
		// two identical extensions should be the same
		{
			name:    "Identical",
			a:       baseRawExtension1,
			b:       baseRawExtension1,
			isEqual: true,
		},
		// same RawExtension in different form (obj & raw) should be the same
		{
			name:    "Same Content, different form",
			a:       baseRawExtension1,
			b:       jsonRawExtension1,
			isEqual: true,
		},
		// different extensions should return false
		{
			name:    "Different content",
			a:       baseRawExtension1,
			b:       baseRawExtension2,
			isEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := equalRawExtensions(&tt.a, &tt.b)
			if err != nil {
				t.Errorf("failed to compare")
			}
			if tt.isEqual != got {
				t.Errorf("Result doesn't match. want %t, get %t\n", tt.isEqual, got)
			}

		})
	}

}
