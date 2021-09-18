// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const bootstrapServiceAccountNamePostfix = "-bootstrap-sa"

func bootstrapServiceAccountNsN(managedCluster *clusterv1.ManagedCluster) (types.NamespacedName, error) {
	if managedCluster == nil {
		return types.NamespacedName{}, fmt.Errorf("managedCluster is nil")
	} else if managedCluster.Name == "" {
		return types.NamespacedName{}, fmt.Errorf("managedCluster.Name is blank")
	}
	return types.NamespacedName{
		Name:      managedCluster.Name + bootstrapServiceAccountNamePostfix,
		Namespace: managedCluster.Name,
	}, nil
}

func getBootstrapSecret(
	client client.Client,
	managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error) {
	sa := &corev1.ServiceAccount{}
	saNsN, err := bootstrapServiceAccountNsN(managedCluster)
	if err != nil {
		return nil, err
	}

	if err := client.Get(context.TODO(), saNsN, sa); err != nil {
		return nil, err
	}
	var secret *corev1.Secret
	log.Info("sa", "sa", sa.Name, "sa.Secrets", sa.Secrets)
	for _, objectRef := range sa.Secrets {
		log.Info("Bootstrap Service Account secret",
			"objectRef.Name", objectRef.Name,
			"objectRef.Namespace", objectRef.Namespace)
		if objectRef.Namespace != "" && objectRef.Namespace != managedCluster.Namespace {
			continue
		}
		prefix := saNsN.Name
		if len(prefix) > 63 {
			prefix = prefix[:37]
		}
		if strings.HasPrefix(objectRef.Name, prefix) {
			secret = &corev1.Secret{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: objectRef.Name, Namespace: managedCluster.Name}, secret)
			if err != nil {
				continue
			}
			if secret.Type == corev1.SecretTypeServiceAccountToken {
				break
			}
		}
	}
	if secret == nil {
		return nil, fmt.Errorf("secret with prefix %s amd type %s not found in service account %s/%s",
			managedCluster.Name+bootstrapServiceAccountNamePostfix,
			corev1.SecretTypeServiceAccountToken,
			saNsN.Name,
			managedCluster.Name)
	}
	return secret, nil
}
