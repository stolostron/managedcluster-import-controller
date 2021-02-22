// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) Red Hat, Inc.

//Package managedcluster ...
package managedcluster

import (
	"context"
	"fmt"
	"strings"

	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	infrastructureConfigName = "cluster"
	apiserverConfigName      = "cluster"
	openshiftConfigNamespace = "openshift-config"
)

func infrastructureConfigNameNsN() types.NamespacedName {
	return types.NamespacedName{
		Name: infrastructureConfigName,
	}
}

func getKubeAPIServerAddress(client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}

	if err := client.Get(context.TODO(), infrastructureConfigNameNsN(), infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// getKubeAPIServerSecretName iterate through all namespacedCertificates
// returns the first one which has a name matches the given dnsName
func getKubeAPIServerSecretName(client client.Client, dnsName string) (string, error) {
	apiserver := &ocinfrav1.APIServer{}
	if err := client.Get(
		context.TODO(),
		types.NamespacedName{Name: apiserverConfigName},
		apiserver,
	); err != nil {
		if errors.IsNotFound(err) {
			log.Info("APIServer cluster not found")
			return "", nil
		}
		return "", err
	}
	// iterate through all namedcertificates
	for _, namedCert := range apiserver.Spec.ServingCerts.NamedCertificates {
		for _, name := range namedCert.Names {
			if strings.EqualFold(name, dnsName) {
				return namedCert.ServingCertificate.Name, nil
			}
		}
	}
	return "", nil
}

// checkIsIBMCloud detects if the current cloud vendor is ibm or not
// we know we are on OCP already, so if it's also ibm cloud, it's roks
func checkIsIBMCloud(client client.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	err := client.List(context.TODO(), nodes)
	if err != nil {
		log.Error(err, "failed to get nodes list")
		return false, err
	}
	if len(nodes.Items) == 0 {
		log.Error(err, "failed to list any nodes")
		return false, nil
	}

	providerID := nodes.Items[0].Spec.ProviderID
	if strings.Contains(providerID, "ibm") {
		return true, nil
	}

	return false, nil
}

// getKubeAPIServerCertificate looks for secret in openshift-config namespace, and returns tls.crt
func getKubeAPIServerCertificate(client client.Client, secretName string) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := client.Get(
		context.TODO(),
		types.NamespacedName{Name: secretName, Namespace: openshiftConfigNamespace},
		secret,
	); err != nil {
		log.Error(err, fmt.Sprintf("Failed to get secret %s/%s", openshiftConfigNamespace, secretName))
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if secret.Type != corev1.SecretTypeTLS {
		return nil, fmt.Errorf(
			"secret %s/%s should have type=kubernetes.io/tls",
			openshiftConfigNamespace,
			secretName,
		)
	}
	res, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf(
			"failed to find data[tls.crt] in secret %s/%s",
			openshiftConfigNamespace,
			secretName,
		)
	}
	return res, nil
}
