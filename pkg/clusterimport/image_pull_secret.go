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
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
)

func imagePullSecretNsN(klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) types.NamespacedName {
	return types.NamespacedName{
		Name:      klusterletConfig.Spec.ImagePullSecret,
		Namespace: klusterletConfig.Namespace,
	}
}

func defaultImagePullSecretNsN() types.NamespacedName {
	return types.NamespacedName{
		Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
		Namespace: os.Getenv("POD_NAMESPACE"),
	}
}

func getImagePullSecret(client client.Client, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*corev1.Secret, error) {
	//if using default image pull secret the pre-process in Reconcile should already stuff the default imagePullSecret in the spec
	if klusterletConfig.Spec.ImagePullSecret == "" {
		return nil, nil
	}

	foundSecret := &corev1.Secret{}
	secretNsN := imagePullSecretNsN(klusterletConfig)
	defaultSecretNsN := defaultImagePullSecretNsN()

	//fetch secret from cluster namespace
	if err := client.Get(context.TODO(), secretNsN, foundSecret); err != nil {
		if !errors.IsNotFound(err) && secretNsN.Name != defaultSecretNsN.Name {
			//fail to fetch cluster namespace secret and secret name does not match default secret
			return nil, err
		}

		//if not found fetch default secret
		if err := client.Get(context.TODO(), defaultSecretNsN, foundSecret); err != nil {
			//fail to fetch default secret
			return nil, err
		}
	}

	//invalid secret type check
	if foundSecret.Type != corev1.SecretTypeDockerConfigJson {
		return nil, fmt.Errorf("secret is not of type corev1.SecretTypeDockerConfigJson")
	}

	return foundSecret, nil
}
