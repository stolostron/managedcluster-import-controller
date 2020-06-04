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
	"os"
	"testing"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
	"github.com/stretchr/testify/assert"
)

func init() {
	os.Setenv("KLUSTERLET_CRD_FILE", "../../build/resources/agent.open-cluster-management.io_v1beta1_klusterlet_crd.yaml")
}

func TestNewOperatorDeployment(t *testing.T) {
	type args struct {
		klusterletConfig *klusterletcfgv1beta1.KlusterletConfig
		imageTagPostfix  string
	}
	type expectValues struct {
		imageName          string
		imageTagPostfixEnv string
		useSHA             string
	}

	tests := []struct {
		name string
		args args
		want expectValues
	}{
		{
			name: "Empty Postfix",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					Spec: klusterletv1beta1.KlusterletSpec{
						ImageRegistry: "sample-registry/uniquePath",
						Version:       "2.3.0",
					},
				},
				imageTagPostfix: "",
			},
			want: expectValues{"sample-registry/uniquePath/endpoint-operator:2.3.0", "", "true"},
		},
		{
			name: "With Postfix Set",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					Spec: klusterletv1beta1.KlusterletSpec{
						ImageRegistry: "sample-registry-2/uniquePath-2",
						Version:       "1.2.0",
					},
				},
				imageTagPostfix: "-Unique-Postfix",
			},
			want: expectValues{"sample-registry-2/uniquePath-2/endpoint-operator:1.2.0-Unique-Postfix", "-Unique-Postfix", "false"},
		},
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			err := os.Setenv(ImageTagPostfixKey, tt.args.imageTagPostfix)
			if err != nil {
				t.Errorf("Cannot set env %s", ImageTagPostfixKey)
			}
			deployment := newOperatorDeployment(tt.args.klusterletConfig)
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Image, tt.want.imageName, "image name should match")
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Env[3].Name, ImageTagPostfixKey)
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Env[3].Value, tt.want.imageTagPostfixEnv, "tag postfix should be passed to env")
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Env[4].Name, "USE_SHA_MANIFEST")
			assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Env[4].Value, tt.want.useSHA, "tag postfix should be passed to env")
		})
	}
}

func TestGenerateKlusterletCRD(t *testing.T) {
	_, err := GenerateKlusterletCRD()
	if err != nil {
		t.Errorf("Cannot generate klusterlet crd: %v", err)
		return
	}
}

func TestGetKlusterletOperatorImage(t *testing.T) {
	type args struct {
		klusterletConfig        *klusterletcfgv1beta1.KlusterletConfig
		imageTagPostfix         string
		klusterletOperatorImage string
	}
	type expectValues struct {
		image           string
		imageTagPostfix string
		useSHA          bool
	}
	tests := []struct {
		name string
		args args
		want expectValues
	}{
		{
			name: "SHA Only",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					Spec: klusterletv1beta1.KlusterletSpec{
						ImageRegistry: "sample-registry/uniquePath",
						Version:       "2.3.0",
					},
				},
				imageTagPostfix:         "",
				klusterletOperatorImage: "sample-registry/uniquePath/endpoint-operator@abcdefghijklmn",
			},
			want: expectValues{"sample-registry/uniquePath/endpoint-operator@abcdefghijklmn", "", true},
		},
		{
			name: "Empty Postfix",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					Spec: klusterletv1beta1.KlusterletSpec{
						ImageRegistry: "sample-registry/uniquePath",
						Version:       "2.3.0",
					},
				},
				imageTagPostfix:         "",
				klusterletOperatorImage: "",
			},
			want: expectValues{"sample-registry/uniquePath/endpoint-operator:2.3.0", "", true},
		},
		{
			name: "Postfix set",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					Spec: klusterletv1beta1.KlusterletSpec{
						ImageRegistry: "sample-registry/uniquePath",
						Version:       "2.3.0",
					},
				},
				imageTagPostfix:         "-postfix",
				klusterletOperatorImage: "",
			},
			want: expectValues{"sample-registry/uniquePath/endpoint-operator:2.3.0-postfix", "-postfix", false},
		},
		{
			name: "SHA and Postfix both set",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					Spec: klusterletv1beta1.KlusterletSpec{
						ImageRegistry: "sample-registry/uniquePath",
						Version:       "2.3.0",
					},
				},
				imageTagPostfix:         "-postfix",
				klusterletOperatorImage: "sample-registry/uniquePath/endpoint-operator@fdklfjasdklfj",
			},
			want: expectValues{"sample-registry/uniquePath/endpoint-operator:2.3.0-postfix", "-postfix", false},
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			if err := os.Setenv(ImageTagPostfixKey, tt.args.imageTagPostfix); err != nil {
				t.Errorf("Cannot set env %s", ImageTagPostfixKey)
			}

			if err := os.Setenv(KlusterletOperatorImageKey, tt.args.klusterletOperatorImage); err != nil {
				t.Errorf("Cannot set env %s", KlusterletOperatorImageKey)
			}
			image, postfix, useSHA := GetKlusterletOperatorImage(tt.args.klusterletConfig)
			assert.Equal(t, image, tt.want.image, "image name should match")
			assert.Equal(t, postfix, tt.want.imageTagPostfix, "postfix should match")
			assert.Equal(t, useSHA, tt.want.useSHA, "postfix should match")
		})
	}

}
