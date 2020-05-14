// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterimport ...
package clusterimport

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

func TestEqualRawExtensions(t *testing.T) {
	baseRawExtension1 := runtime.RawExtension{
		Object: &klusterletcfgv1beta1.KlusterletConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "klusterletConfig1",
				Namespace: "namespace",
			}},
	}
	baseRawExtension2 := runtime.RawExtension{
		Object: &klusterletcfgv1beta1.KlusterletConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "klusterletConfig2",
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
