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

	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func imagePullSecretNsN(endpointConfig *multicloudv1alpha1.EndpointConfig) types.NamespacedName {
	return types.NamespacedName{
		Name:      endpointConfig.Spec.ImagePullSecret,
		Namespace: endpointConfig.Namespace,
	}
}

func defaultImagePullSecretNsN() types.NamespacedName {
	return types.NamespacedName{
		Name:      os.Getenv("DEFAULT_IMAGE_PULL_SECRET"),
		Namespace: os.Getenv("POD_NAMESPACE"),
	}
}

func getImagePullSecret(client client.Client, endpointConfig *multicloudv1alpha1.EndpointConfig) (*corev1.Secret, error) {
	//if using default image pull secret the pre-process in Reconcile should already stuff the default imagePullSecret in the spec
	if endpointConfig.Spec.ImagePullSecret == "" {
		return nil, nil
	}

	foundSecret := &corev1.Secret{}
	secretNsN := imagePullSecretNsN(endpointConfig)
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
