// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// TODO add unit test for the following functions, right now they are only covered in e2e
package helpers

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkSelector func(string, workv1.ManifestWork) bool

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

	recorder.Eventf("ManagedClusterFinalizerAdded",
		"The managed cluster %s manifestwork finalizer is added", cluster.Name)
	return nil
}

// ForceDeleteAllManifestWorks delete all manifestworks forcefully
func ForceDeleteAllManifestWorks(ctx context.Context, workClient workclient.Interface, recorder events.Recorder,
	manifestWorks []workv1.ManifestWork) error {
	for _, item := range manifestWorks {
		if err := ForceDeleteManifestWork(ctx, workClient, recorder, item.Namespace, item.Name); err != nil {
			return err
		}
	}
	return nil
}

// ForceDeleteManifestWork will delete the manifestwork regardless of finalizers.
func ForceDeleteManifestWork(ctx context.Context, workClient workclient.Interface, recorder events.Recorder,
	namespace, name string) error {
	_, err := workClient.WorkV1().ManifestWorks(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	err = workClient.WorkV1().ManifestWorks(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// reload the manifest work
	manifestWork, err := workClient.WorkV1().ManifestWorks(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// if the manifest work is not deleted, force remove its finalizers
	if len(manifestWork.Finalizers) != 0 {
		patch := "{\"metadata\": {\"finalizers\":[]}}"
		if _, err = workClient.WorkV1().ManifestWorks(namespace).Patch(ctx, name, types.MergePatchType,
			[]byte(patch), metav1.PatchOptions{}); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	recorder.Eventf("ManifestWorksForceDeleted",
		fmt.Sprintf("The manifest work %s/%s is force deleted", manifestWork.Namespace, manifestWork.Name))
	return nil
}

// ListManagedClusterAddons lists all managedclusteraddons for the managed cluster
func ListManagedClusterAddons(ctx context.Context, runtimeClient client.Client, clusterName string) (
	*addonv1alpha1.ManagedClusterAddOnList, error) {
	managedClusterAddons := &addonv1alpha1.ManagedClusterAddOnList{}
	if err := runtimeClient.List(ctx, managedClusterAddons, client.InNamespace(clusterName)); err != nil {
		return managedClusterAddons, err
	}
	return managedClusterAddons, nil
}

// GetWorkRoleBinding gets the work roleBinding in the cluster ns
func GetWorkRoleBinding(ctx context.Context, runtimeClient client.Client, clusterName string) (
	*rbacv1.RoleBinding, error) {
	workRoleBinding := &rbacv1.RoleBinding{}
	workRoleBindingName := fmt.Sprintf("open-cluster-management:managedcluster:%s:work", clusterName)
	err := runtimeClient.Get(ctx, types.NamespacedName{Name: workRoleBindingName, Namespace: clusterName}, workRoleBinding)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return workRoleBinding, nil
}

// ForceDeleteWorkRoleBinding gets the work roleBinding in the cluster ns
func ForceDeleteWorkRoleBinding(ctx context.Context, kubeClient kubernetes.Interface, clusterName string,
	recorder events.Recorder) error {
	workRoleBindingName := fmt.Sprintf("open-cluster-management:managedcluster:%s:work", clusterName)
	workRoleBinding, err := kubeClient.RbacV1().RoleBindings(clusterName).Get(ctx, workRoleBindingName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if workRoleBinding.DeletionTimestamp.IsZero() {
		if err = kubeClient.RbacV1().RoleBindings(clusterName).
			Delete(ctx, workRoleBindingName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	// reload the manifest work
	workRoleBinding, err = kubeClient.RbacV1().RoleBindings(clusterName).Get(ctx, workRoleBindingName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// if the manifest work is not deleted, force remove its finalizers
	if len(workRoleBinding.Finalizers) != 0 {
		patch := "{\"metadata\": {\"finalizers\":[]}}"
		if _, err = kubeClient.RbacV1().RoleBindings(clusterName).Patch(ctx, workRoleBindingName,
			types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	recorder.Eventf("workRoleBindingDeleted",
		fmt.Sprintf("The manifest work roleBinding %s/%s is force deleted", clusterName, workRoleBindingName))
	return nil
}

// IsClusterUnavailable checks whether the cluster is unavailable
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

func IsManifestWorksAvailable(ctx context.Context, client workclient.Interface,
	namespace string, names ...string) (bool, error) {
	for _, name := range names {
		work, err := client.WorkV1().ManifestWorks(namespace).Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		if !meta.IsStatusConditionTrue(work.Status.Conditions, workv1.WorkAvailable) {
			return false, nil
		}
	}
	return true, nil
}

func HostedKlusterletManifestWorkName(managedClusterName string) string {
	return fmt.Sprintf("%s-%s", managedClusterName, constants.HostedKlusterletManifestworkSuffix)
}

func HostedManagedKubeConfigManifestWorkName(managedClusterName string) string {
	return fmt.Sprintf("%s-%s", managedClusterName, constants.HostedManagedKubeconfigManifestworkSuffix)
}
