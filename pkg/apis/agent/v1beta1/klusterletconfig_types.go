// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KlusterletConfigStatus defines the observed state of KlusterletConfig
// +k8s:openapi-gen=true
type KlusterletConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KlusterletConfig is the Schema for the klusterletconfigs API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type KlusterletConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              klusterletv1beta1.KlusterletSpec `json:"spec,omitempty"`
	Status            KlusterletConfigStatus           `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KlusterletConfigList contains a list of KlusterletConfig
type KlusterletConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KlusterletConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KlusterletConfig{}, &KlusterletConfigList{})
}
