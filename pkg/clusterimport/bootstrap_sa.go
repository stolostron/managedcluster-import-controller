// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package clusterimport

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

// BootstrapServiceAccountNamePostfix is the postfix for bootstrap service account
const BootstrapServiceAccountNamePostfix = "-bootstrap-sa"

func bootstrapServiceAccountNsN(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (types.NamespacedName, error) {
	if klusterletConfig == nil {
		return types.NamespacedName{}, fmt.Errorf("klusterletconfig can not be nil")
	}

	if klusterletConfig.Spec.ClusterName == "" {
		return types.NamespacedName{}, fmt.Errorf("klusterletconfig can not have empty ClusterName")
	}

	return types.NamespacedName{
		Name:      klusterletConfig.Spec.ClusterName + BootstrapServiceAccountNamePostfix,
		Namespace: klusterletConfig.Namespace,
	}, nil
}

func newBootstrapServiceAccount(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.ServiceAccount, error) {
	saNsN, err := bootstrapServiceAccountNsN(klusterletConfig)
	if err != nil {
		return nil, err
	}

	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saNsN.Name,
			Namespace: saNsN.Namespace,
		},
	}, nil
}

// GetBootstrapServiceAccount get the service account use for multicluster-endpoint bootstrap
func GetBootstrapServiceAccount(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.ServiceAccount, error) {
	saNsN, err := bootstrapServiceAccountNsN(klusterletConfig)
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{}

	if err := client.Get(context.TODO(), saNsN, sa); err != nil {
		return nil, err
	}

	return sa, nil
}

// CreateBootstrapServiceAccount create the service account use for multicluster-endpoint bootstrap
func CreateBootstrapServiceAccount(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.ServiceAccount, error) {
	sa, err := newBootstrapServiceAccount(klusterletConfig)
	if err != nil {
		return nil, err
	}

	if err := client.Create(context.TODO(), sa); err != nil {
		return nil, err
	}

	return sa, nil
}

func getBootstrapTokenSecret(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.Secret, error) {
	sa, err := GetBootstrapServiceAccount(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	for _, secret := range sa.Secrets {
		secretNsN := types.NamespacedName{
			Name:      secret.Name,
			Namespace: sa.Namespace,
		}

		saSecret := &corev1.Secret{}
		if err := client.Get(context.TODO(), secretNsN, saSecret); err != nil {
			continue
		}

		if saSecret.Type == corev1.SecretTypeServiceAccountToken {
			return saSecret, nil
		}
	}

	return nil, fmt.Errorf("fail to find service account token secret")
}

func getBootstrapToken(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) ([]byte, error) {
	secret, err := getBootstrapTokenSecret(client, klusterletConfig)
	if err != nil {
		return nil, err
	}

	token, ok := secret.Data["token"]
	if !ok {
		return nil, fmt.Errorf("data of bootstrap serviceaccount token secret does not contain token")
	}

	return token, nil
}
