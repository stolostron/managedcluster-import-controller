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
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
)

func endpointConfigNsN(name string, namespace string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
}

// GetResourceView - if resourceview is present it returns true otherwise it returns false
func getEndpointResourceView(client client.Client, cluster *clusterregistryv1alpha1.Cluster) (*mcmv1alpha1.ResourceView, error) {
	ncNsN := endpointConfigNsN(cluster.Name+"-get-endpoint", cluster.Namespace)
	resourceview := &mcmv1alpha1.ResourceView{}
	if err := client.Get(context.TODO(), ncNsN, resourceview); err != nil {
		return nil, err
	}
	return resourceview, nil
}

//isEndpointResourceviewProcessing - check if the resourceview completed
func isEndpointResourceviewProcessing(resourceview *mcmv1alpha1.ResourceView) bool {
	if len(resourceview.Status.Conditions) > 0 {
		for _, condition := range resourceview.Status.Conditions {
			if condition.Type == mcmv1alpha1.WorkProcessing {
				return true
			}
		}
	}
	return false
}

// createEndpointResourceview - Creates resourceview to fetch endpoint from managed cluster
func createEndpointResourceview(
	r *ReconcileEndpointConfig,
	cluster *clusterregistryv1alpha1.Cluster,
	endpointConf *multicloudv1alpha1.EndpointConfig) (*mcmv1alpha1.ResourceView, error) {

	resourceView := &mcmv1alpha1.ResourceView{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ResourceView",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-get-endpoint",
			Namespace: cluster.Namespace,
		},
		Spec: mcmv1alpha1.ResourceViewSpec{
			ClusterSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": cluster.Name,
				},
			},
			SummaryOnly:           false,
			Mode:                  mcmv1alpha1.PeriodicResourceUpdate,
			UpdateIntervalSeconds: 60,
			Scope: mcmv1alpha1.ViewFilter{
				Resource:     "endpoint.multicloud.ibm.com",
				ResourceName: "endpoint",
				NameSpace:    EndpointNamespace,
			},
		},
	}

	if err := controllerutil.SetControllerReference(endpointConf, resourceView, r.scheme); err != nil {
		return nil, err
	}

	err := r.client.Create(context.TODO(), resourceView)
	if err != nil {
		return nil, err
	}

	time.Sleep(3 * time.Second)
	return resourceView, nil
}

// getEndpointFromResourceView - Fetch the endpoint from managed cluster
func getEndpointFromResourceView(
	r *ReconcileEndpointConfig,
	cluster *clusterregistryv1alpha1.Cluster,
	resourceView *mcmv1alpha1.ResourceView) (*multicloudv1beta1.Endpoint, error) {

	resourceNamespace := types.NamespacedName{
		Name:      resourceView.Name,
		Namespace: cluster.Namespace,
	}

	completedResourceView := &mcmv1alpha1.ResourceView{}
	if err := r.apireader.Get(context.TODO(), resourceNamespace, completedResourceView); err != nil {
		return nil, err
	}

	endpoint := &multicloudv1beta1.Endpoint{}
	err := json.Unmarshal(completedResourceView.Status.Results[cluster.Name].Raw, endpoint)
	if err != nil {
		return nil, err
	}

	return endpoint, nil
}
