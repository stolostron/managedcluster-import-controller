// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"fmt"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
)

const syncsetNamePostfix = "-klusterlet"
const syncsetCRDSPostfix = "-crds"

func syncSetNsN(managedCluster *clusterv1.ManagedCluster) (types.NamespacedName, error) {
	if managedCluster == nil {
		return types.NamespacedName{}, fmt.Errorf("managedCluster is nil")
	} else if managedCluster.Name == "" {
		return types.NamespacedName{}, fmt.Errorf("managedCluster.Name is blank")
	}
	return types.NamespacedName{
		Name:      managedCluster.Name + syncsetNamePostfix,
		Namespace: managedCluster.Name,
	}, nil
}

func deleteKlusterletSyncSets(
	client client.Client,
	managedCluster *clusterv1.ManagedCluster,
) (res reconcile.Result, err error) {
	ssNsN, err := syncSetNsN(managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	//Delete the CRD syncset
	result, err := deleteKlusterletSyncSet(client, ssNsN.Name+syncsetCRDSPostfix, ssNsN.Namespace)
	if err != nil {
		return result, err
	}

	//Delete the YAML syncset
	return deleteKlusterletSyncSet(client, ssNsN.Name, ssNsN.Namespace)
}

func deleteKlusterletSyncSet(
	client client.Client,
	name string,
	namespace string,
) (res reconcile.Result, err error) {
	oldSyncSet := &hivev1.SyncSet{}
	err = client.Get(context.TODO(),
		types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
		oldSyncSet)
	if err == nil {
		klog.Infof("SyncSet %s found, will delete it with upsert mode", oldSyncSet.GetName())
		//Update the syncset to set upsert mode.
		if oldSyncSet.Spec.ResourceApplyMode != hivev1.UpsertResourceApplyMode {
			klog.Infof("SyncSet %s set with upsert mode", oldSyncSet.GetName())
			oldSyncSet.Spec.ResourceApplyMode = hivev1.UpsertResourceApplyMode
			err := client.Update(context.TODO(), oldSyncSet)
			if err != nil {
				return reconcile.Result{}, err
			}
			klog.Infof("SyncSet %s set with upsert mode, requeue to wait hive to process", oldSyncSet.GetName())
			return reconcile.Result{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
		}
		//Now delete syncset.
		klog.Infof("SyncSet %s will be deleted", oldSyncSet.GetName())
		err = client.Delete(context.TODO(), oldSyncSet)
		if err != nil {
			return reconcile.Result{}, err
		}
		klog.Infof("SyncSet %s deleted", oldSyncSet.GetName())
	}
	return reconcile.Result{}, nil
}
