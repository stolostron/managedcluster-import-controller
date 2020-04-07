// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterregistry contains common utility functions that gets call by many differerent packages

package endpointconfig

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

// constants for update endpoint on managed cluster
const (
	EndpointUpdateWork = "update-multicluster-endpoint"
	EndpointNamespace  = "multicluster-endpoint"
)

// getEndpointUpdateWork - fetch the endpoint update work
func getEndpointUpdateWork(r *ReconcileEndpointConfig, endpointc *multicloudv1alpha1.EndpointConfig) (*mcmv1alpha1.Work, error) {
	ncNsN := types.NamespacedName{
		Name:      endpointc.Name + "-update-endpoint",
		Namespace: endpointc.Namespace,
	}
	endpointWork := &mcmv1alpha1.Work{}
	if err := r.client.Get(context.TODO(), ncNsN, endpointWork); err != nil {
		return nil, err
	}

	return endpointWork, nil
}

// createEndpointUpdateWork - creates work to update endpoint on the managed cluster
func createEndpointUpdateWork(r *ReconcileEndpointConfig, endpointc *multicloudv1alpha1.EndpointConfig, endpoint *multicloudv1beta1.Endpoint) error {
	endpoint.Spec = endpointc.Spec
	work := &mcmv1alpha1.Work{
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointc.Name + "-update-endpoint",
			Namespace: endpointc.Namespace,
		},
		Spec: mcmv1alpha1.WorkSpec{
			Cluster: corev1.LocalObjectReference{
				Name: endpointc.Name,
			},
			ActionType: mcmv1alpha1.UpdateActionType,
			Type:       mcmv1alpha1.ActionWorkType,
			KubeWork: &mcmv1alpha1.KubeWorkSpec{
				Resource:  "endpoints.multicloud.ibm.com",
				Name:      EndpointUpdateWork,
				Namespace: EndpointNamespace,
				ObjectTemplate: runtime.RawExtension{
					Object: endpoint,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(endpointc, work, r.scheme); err != nil {
		return err
	}

	if err := r.client.Create(context.TODO(), work); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)

	return nil
}

// deleteEndpointUpdateWork - delete endpoint update work
func deleteEndpointUpdateWork(r *ReconcileEndpointConfig, work *mcmv1alpha1.Work) error {
	if err := r.client.Delete(context.TODO(), work); err != nil {
		return err
	}

	return nil
}
