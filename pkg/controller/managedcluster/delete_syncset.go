// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"fmt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
) error {
	ssNsN, err := syncSetNsN(managedCluster)
	if err != nil {
		return err
	}

	//Delete the CRD syncset
	err = deleteKlusterletSyncSet(client, ssNsN.Name+syncsetCRDSPostfix, ssNsN.Namespace)
	if err != nil {
		return err
	}

	//Delete the YAML syncset
	return deleteKlusterletSyncSet(client, ssNsN.Name, ssNsN.Namespace)
}

func deleteKlusterletSyncSet(
	client client.Client,
	name string,
	namespace string,
) error {
	oldSyncSet := &hivev1.SyncSet{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
		oldSyncSet)
	if err == nil {
		klog.Infof("SyncSet %s found, will delete it with upsert mode", oldSyncSet.GetName())
		//Update the syncset to set upsert mode.
		oldSyncSet.Spec.ResourceApplyMode = hivev1.UpsertResourceApplyMode
		err := client.Update(context.TODO(), oldSyncSet)
		if err != nil {
			return err
		}
		//Now delete syncset.
		err = client.Delete(context.TODO(), oldSyncSet)
		if err != nil {
			return err
		}
		klog.Infof("SyncSet %s deleted", oldSyncSet.GetName())
	}
	return nil
}
