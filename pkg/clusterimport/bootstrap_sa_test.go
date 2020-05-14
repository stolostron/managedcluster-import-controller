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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

func Test_bootstrapServiceAccountNsN(t *testing.T) {
	type args struct {
		klusterletConfig *klusterletcfgv1beta1.KlusterletConfig
	}

	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name: "nil KlusterletConfig",
			args: args{
				klusterletConfig: nil,
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "empty KlusterletConfig",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{},
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "good KlusterletConfig",
			args: args{
				klusterletConfig: &klusterletcfgv1beta1.KlusterletConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "klusterletConfig",
						Namespace: "namespace",
					},
					Spec: klusterletv1beta1.KlusterletSpec{
						ClusterName: "clustername",
					},
				},
			},
			want: types.NamespacedName{
				Name:      "clustername" + BootstrapServiceAccountNamePostfix,
				Namespace: "namespace",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bootstrapServiceAccountNsN(tt.args.klusterletConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("bootstrapServiceAccountNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bootstrapServiceAccountNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}
