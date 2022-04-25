// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

// AssertManifestWorkFinalizer add/remove manifest finalizer for a managed cluster,
// this func will send request to api server to update managed cluster.
func AssertManifestWorkFinalizer(ctx context.Context, runtimeClient client.Client, recorder events.Recorder,
	cluster *clusterv1.ManagedCluster, works int) error {
	if works == 0 {
		// there are no manifest works, remove the manifest work finalizer
		err := RemoveManagedClusterFinalizer(ctx, runtimeClient, recorder, cluster, constants.ManifestWorkFinalizer)
		if err != nil {
			return err
		}
		return nil
	}

	if !cluster.DeletionTimestamp.IsZero() {
		// cluster is deleting, do nothing
		return nil
	}

	// there are manifest works in the managed cluster namespace, make sure the managed cluster has the manifest work finalizer
	patch := client.MergeFrom(cluster.DeepCopy())
	modified := resourcemerge.BoolPtr(false)
	AddManagedClusterFinalizer(modified, cluster, constants.ManifestWorkFinalizer)
	if !*modified {
		return nil
	}

	if err := runtimeClient.Patch(ctx, cluster, patch); err != nil {
		return err
	}

	recorder.Eventf("ManagedClusterMetaObjModified",
		"The managed cluster %s meta data is modified: manifestwork finalizer is added", cluster.Name)
	return nil
}

// ForceDeleteAllManifestWorks delete all manifestworks forcefully
func ForceDeleteAllManifestWorks(ctx context.Context, runtimeClient client.Client, recorder events.Recorder,
	manifestWorks []workv1.ManifestWork) error {
	for _, item := range manifestWorks {
		if err := ForceDeleteManifestWork(ctx, runtimeClient, recorder, item.Namespace, item.Name); err != nil {
			return err
		}
	}
	return nil
}

// ForceDeleteManifestWork will delete the manifestwork regardless of finalizers.
func ForceDeleteManifestWork(ctx context.Context, runtimeClient client.Client, recorder events.Recorder,
	namespace, name string) error {
	manifestWorkKey := types.NamespacedName{Namespace: namespace, Name: name}
	manifestWork := &workv1.ManifestWork{}
	err := runtimeClient.Get(ctx, manifestWorkKey, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := runtimeClient.Delete(ctx, manifestWork); err != nil {
		return err
	}

	// reload the manifest work
	err = runtimeClient.Get(ctx, manifestWorkKey, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// if the manifest work is not deleted, force remove its finalizers
	if len(manifestWork.Finalizers) != 0 {
		patch := client.MergeFrom(manifestWork.DeepCopy())
		manifestWork.Finalizers = []string{}
		if err := runtimeClient.Patch(ctx, manifestWork, patch); err != nil {
			return err
		}
	}

	recorder.Eventf("ManifestWorksForceDeleted",
		fmt.Sprintf("The manifest work %s/%s is force deleted", manifestWork.Namespace, manifestWork.Name))
	return nil
}

// DeleteManifestWork triggers the deletion action of the manifestwork
func DeleteManifestWork(ctx context.Context, runtimeClient client.Client, recorder events.Recorder,
	namespace, name string) error {
	manifestWork := &workv1.ManifestWork{}
	err := runtimeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, manifestWork)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if !manifestWork.DeletionTimestamp.IsZero() {
		// the manifest work is deleting, do nothing
		return nil
	}

	if err := runtimeClient.Delete(ctx, manifestWork); err != nil {
		return err
	}

	recorder.Eventf("ManifestWorksDeleted", fmt.Sprintf("The manifest work %s/%s is deleted", namespace, name))
	return nil
}

// NoPendingManifestWorks checks whether there are pending manifestworks for the managed cluster
func NoPendingManifestWorks(ctx context.Context, runtimeClient client.Client, log logr.Logger, clusterName string,
	ignoredSelector func(clusterName string, manifestWork workv1.ManifestWork) bool) (bool, error) {
	listOpts := &client.ListOptions{Namespace: clusterName}
	manifestWorks := &workv1.ManifestWorkList{}
	if err := runtimeClient.List(ctx, manifestWorks, listOpts); err != nil {
		return false, err
	}

	manifestWorkNames := []string{}
	ignoredManifestWorkNames := []string{}
	for _, manifestWork := range manifestWorks.Items {
		if ignoredSelector(clusterName, manifestWork) {
			ignoredManifestWorkNames = append(ignoredManifestWorkNames, manifestWork.GetName())
		} else {
			manifestWorkNames = append(manifestWorkNames, manifestWork.GetName())
		}
	}

	if len(manifestWorkNames) != 0 {
		log.Info(fmt.Sprintf("In addition to ignored manifest works %s, there are also have %s",
			strings.Join(ignoredManifestWorkNames, ","), strings.Join(manifestWorkNames, ",")))
		return false, nil
	}

	return true, nil
}

// ListManagedClusterAddons lists all managedclusteraddons for the managed cluster
func ListManagedClusterAddons(ctx context.Context, runtimeClient client.Client, clusterName string) (
	*addonv1alpha1.ManagedClusterAddOnList, error) {
	managedClusterAddons := &addonv1alpha1.ManagedClusterAddOnList{}
	if err := runtimeClient.List(ctx, managedClusterAddons, client.InNamespace(clusterName)); err != nil {
		return nil, err
	}
	return managedClusterAddons, nil
}

// // NoManagedClusterAddons checks whether there are managedclusteraddons for the managed cluster
func NoManagedClusterAddons(ctx context.Context, runtimeClient client.Client, clusterName string) (bool, error) {
	managedclusteraddons, err := ListManagedClusterAddons(ctx, runtimeClient, clusterName)
	if err != nil {
		return false, err
	}

	return len(managedclusteraddons.Items) == 0, nil
}

// DeleteManagedClusterAddons deletes all managedclusteraddons for the managed cluster
func DeleteManagedClusterAddons(
	ctx context.Context,
	runtimeClient client.Client,
	recorder events.Recorder,
	cluster *clusterv1.ManagedCluster) error {
	if IsClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all managed cluster addons
		return ForceDeleteAllManagedClusterAddons(ctx, runtimeClient, recorder, cluster.GetName())
	}

	return runtimeClient.DeleteAllOf(ctx, &addonv1alpha1.ManagedClusterAddOn{}, client.InNamespace(cluster.GetName()))
}

// DeleteManifestWorkWithSelector deletes manifestworks but ignores the ignoredSelector selected manifestworks
func DeleteManifestWorkWithSelector(ctx context.Context, runtimeClient client.Client, recorder events.Recorder,
	cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork,
	ignoredSelector func(clusterName string, manifestWork workv1.ManifestWork) bool) error {

	for _, manifestWork := range works {
		if ignoredSelector(cluster.GetName(), manifestWork) {
			continue
		}

		annotations := manifestWork.GetAnnotations()
		if _, ok := annotations[constants.PostponeDeletionAnnotation]; ok {
			if time.Since(cluster.DeletionTimestamp.Time) < constants.ManifestWorkPostponeDeleteTime {
				continue
			}
		}
		if err := DeleteManifestWork(ctx, runtimeClient, recorder, manifestWork.Namespace, manifestWork.Name); err != nil {
			return err
		}
	}

	return nil
}

// IsClusterUnavailable checks whether the cluster is unavilable
func IsClusterUnavailable(cluster *clusterv1.ManagedCluster) bool {
	if meta.IsStatusConditionFalse(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
		return true
	}
	if meta.IsStatusConditionPresentAndEqual(
		cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
		return true
	}

	return false
}
