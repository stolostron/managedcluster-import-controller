// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ghodss/yaml"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

func newSyncSets(
	client client.Client,
	managedCluster *clusterv1.ManagedCluster,
) (*hivev1.SyncSet, *hivev1.SyncSet, error) {

	crds, yamls, err := generateImportYAMLs(client, managedCluster, []string{})
	if err != nil {
		return nil, nil, err
	}

	runtimeRawExtensionsCRDs, err := convertToRawExtensions(crds)
	if err != nil {
		return nil, nil, err
	}

	runtimeRawExtensionsYAMLs, err := convertToRawExtensions(yamls)
	if err != nil {
		return nil, nil, err
	}

	ssNsN, err := syncSetNsN(managedCluster)
	if err != nil {
		return nil, nil, err
	}

	crdsSyncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssNsN.Name + syncsetCRDSPostfix,
			Namespace: ssNsN.Namespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				Resources:         runtimeRawExtensionsCRDs,
				ResourceApplyMode: hivev1.SyncResourceApplyMode,
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: managedCluster.Name,
				},
			},
		},
	}

	yamlsSyncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssNsN.Name,
			Namespace: ssNsN.Namespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				Resources:         runtimeRawExtensionsYAMLs,
				ResourceApplyMode: hivev1.SyncResourceApplyMode,
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: managedCluster.Name,
				},
			},
		},
	}

	return crdsSyncSet, yamlsSyncSet, nil
}

func convertToRawExtensions(data [][]byte) (runtimeRawExtensions []runtime.RawExtension, err error) {
	for _, d := range data {
		j, err := yaml.YAMLToJSON(d)
		if err != nil {
			return nil, err
		}
		runtimeRawExtensions = append(runtimeRawExtensions, runtime.RawExtension{Raw: j})
	}
	return runtimeRawExtensions, nil
}

// createOrUpdateSyncSets create the syncset use for installing klusterlet
func createOrUpdateSyncSets(
	client client.Client,
	scheme *runtime.Scheme,
	managedCluster *clusterv1.ManagedCluster,
) (*hivev1.SyncSet, *hivev1.SyncSet, error) {
	crds, yamls, err := newSyncSets(client, managedCluster)
	if err != nil {
		return nil, nil, err
	}

	crds, err = createOrUpdateSyncSet(client, scheme, managedCluster, crds)
	if err != nil {
		return nil, nil, err
	}

	yamls, err = createOrUpdateSyncSet(client, scheme, managedCluster, yamls)
	if err != nil {
		return nil, nil, err
	}

	return crds, yamls, nil
}

func createOrUpdateSyncSet(
	client client.Client,
	scheme *runtime.Scheme,
	managedCluster *clusterv1.ManagedCluster,
	ss *hivev1.SyncSet,
) (*hivev1.SyncSet, error) {
	// set ownerReference to klusterletconfig
	if err := controllerutil.SetControllerReference(managedCluster, ss, scheme); err != nil {
		return nil, err
	}
	log.Info("Create/update of Import syncset", "name", ss.Name, "namespace", ss.Namespace)
	oldSyncSet := &hivev1.SyncSet{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: ss.Name, Namespace: ss.Namespace}, oldSyncSet)
	if err != nil {
		if errors.IsNotFound(err) {
			err := client.Create(context.TODO(), ss)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		if !reflect.DeepEqual(oldSyncSet.Spec, ss.Spec) {
			log.Info("Exist then Update of Import syncset", "name", ss.Name, "namespace", ss.Namespace)
			oldSyncSet.Spec = ss.Spec
			if err := client.Update(context.TODO(), oldSyncSet); err != nil {
				return nil, err
			}
		} else {
			log.Info("Synset identical then no update", "name", ss.Name, "namespace", ss.Namespace)
		}
	}
	return ss, nil
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
	ss := &hivev1.SyncSet{}
	errCRDs := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      ssNsN.Name + syncsetCRDSPostfix,
			Namespace: ssNsN.Namespace},
		ss)
	if errCRDs == nil {
		err := client.Delete(context.TODO(), ss)
		if err != nil {
			return err
		}
	} else if !errors.IsNotFound(errCRDs) {
		return err
	}

	//Delete the YAML syncset
	ss = &hivev1.SyncSet{}
	errYAMLs := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      ssNsN.Name,
			Namespace: ssNsN.Namespace},
		ss)
	if errYAMLs == nil {
		err := client.Delete(context.TODO(), ss)
		if err != nil {
			return err
		}
	} else if !errors.IsNotFound(errYAMLs) {
		return err
	}

	return nil
}
