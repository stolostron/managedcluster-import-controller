// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterregistry contains common utility functions that gets call by many differerent packages
package klusterletconfig

import (
	"context"
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
	"github.com/open-cluster-management/rcm-controller/pkg/clusterimport"
)

func klusterletConfigNsN(name string, namespace string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
}

// getKlusterletResourceView - if resourceview is present it returns true otherwise it returns false
func getKlusterletResourceView(client client.Client, cluster *clusterregistryv1alpha1.Cluster) (*mcmv1alpha1.ResourceView, error) {
	ncNsN := klusterletConfigNsN(cluster.Name+"-get-klusterlet", cluster.Namespace)
	resourceview := &mcmv1alpha1.ResourceView{}
	if err := client.Get(context.TODO(), ncNsN, resourceview); err != nil {
		return nil, err
	}
	return resourceview, nil
}

//isKlusterletResourceviewProcessing - check if the resourceview completed
func isKlusterletResourceviewProcessing(resourceview *mcmv1alpha1.ResourceView) bool {
	if len(resourceview.Status.Conditions) > 0 {
		for _, condition := range resourceview.Status.Conditions {
			if condition.Type == mcmv1alpha1.WorkProcessing {
				return true
			}
		}
	}
	return false
}

// createKlusterletResourceview - Creates resourceview to fetch klusterlet from managed cluster
func createKlusterletResourceview(
	r *ReconcileKlusterletConfig,
	cluster *clusterregistryv1alpha1.Cluster,
	klusterletConf *klusterletcfgv1beta1.KlusterletConfig) (*mcmv1alpha1.ResourceView, error) {

	resourceView := &mcmv1alpha1.ResourceView{
		TypeMeta: metav1.TypeMeta{
			APIVersion: mcmv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ResourceView",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-get-klusterlet",
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
				Resource:     "klusterlet.agent.open-cluster-management.io",
				ResourceName: "klusterlet",
				NameSpace:    clusterimport.KlusterletNamespace,
			},
		},
	}

	if err := controllerutil.SetControllerReference(klusterletConf, resourceView, r.scheme); err != nil {
		return nil, err
	}

	err := r.client.Create(context.TODO(), resourceView)
	if err != nil {
		return nil, err
	}

	time.Sleep(3 * time.Second)
	return resourceView, nil
}

// getKlusterletFromResourceView - Fetch the klusterlet from managed cluster
func getKlusterletFromResourceView(
	r *ReconcileKlusterletConfig,
	cluster *clusterregistryv1alpha1.Cluster,
	resourceView *mcmv1alpha1.ResourceView) (*klusterletv1beta1.Klusterlet, error) {

	resourceNamespace := types.NamespacedName{
		Name:      resourceView.Name,
		Namespace: cluster.Namespace,
	}

	completedResourceView := &mcmv1alpha1.ResourceView{}
	if err := r.apireader.Get(context.TODO(), resourceNamespace, completedResourceView); err != nil {
		return nil, err
	}

	klusterlet := &klusterletv1beta1.Klusterlet{}
	err := json.Unmarshal(completedResourceView.Status.Results[cluster.Name].Raw, klusterlet)
	if err != nil {
		return nil, err
	}

	return klusterlet, nil
}
