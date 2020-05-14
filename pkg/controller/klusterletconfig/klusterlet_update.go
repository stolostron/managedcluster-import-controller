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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
	"github.com/open-cluster-management/rcm-controller/pkg/clusterimport"
)

// constants for update klusterlet on managed cluster
const (
	KlusterletUpdateWork = "update-klusterlet"
)

// getKlusterletUpdateWork - fetch the klusterlet update work
func getKlusterletUpdateWork(r *ReconcileKlusterletConfig, klusterletConfig *klusterletcfgv1beta1.KlusterletConfig) (*mcmv1alpha1.Work, error) {
	ncNsN := types.NamespacedName{
		Name:      klusterletConfig.Name + "-update-klusterlet",
		Namespace: klusterletConfig.Namespace,
	}
	klusterletWork := &mcmv1alpha1.Work{}
	if err := r.client.Get(context.TODO(), ncNsN, klusterletWork); err != nil {
		return nil, err
	}

	return klusterletWork, nil
}

// createKlusterletUpdateWork - creates work to update klusterlet on the managed cluster
func createKlusterletUpdateWork(r *ReconcileKlusterletConfig,
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
	klusterlet *klusterletv1beta1.Klusterlet) error {

	klusterlet.Spec = klusterletConfig.Spec
	work := &mcmv1alpha1.Work{
		ObjectMeta: metav1.ObjectMeta{
			Name:      klusterletConfig.Name + "-update-klusterlet",
			Namespace: klusterletConfig.Namespace,
		},
		Spec: mcmv1alpha1.WorkSpec{
			Cluster: corev1.LocalObjectReference{
				Name: klusterletConfig.Name,
			},
			ActionType: mcmv1alpha1.UpdateActionType,
			Type:       mcmv1alpha1.ActionWorkType,
			KubeWork: &mcmv1alpha1.KubeWorkSpec{
				Resource:  "klusterlets.agent.open-cluster-management.io",
				Name:      KlusterletUpdateWork,
				Namespace: clusterimport.KlusterletNamespace,
				ObjectTemplate: runtime.RawExtension{
					Object: klusterlet,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(klusterletConfig, work, r.scheme); err != nil {
		return err
	}

	if err := r.client.Create(context.TODO(), work); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)

	return nil
}

// deleteKlusterletUpdateWork - delete klusterlet update work
func deleteKlusterletUpdateWork(r *ReconcileKlusterletConfig, work *mcmv1alpha1.Work) error {
	if err := r.client.Delete(context.TODO(), work); err != nil {
		return err
	}

	return nil
}
