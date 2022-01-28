// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
)

type autoImportWorker interface {
	addFinalizer(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error
	removeImportFinalizer(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error

	autoImport(managedCluster *clusterv1.ManagedCluster, autoImportSecret *corev1.Secret, importSecret *corev1.Secret) (
		importCondition metav1.Condition, updateRetryTimes bool, err error)
}

type defaultWorker struct {
	recorder events.Recorder
}

var _ autoImportWorker = &defaultWorker{}

func (w *defaultWorker) addFinalizer(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	return nil
}

func (w *defaultWorker) removeImportFinalizer(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	return nil
}

func (w *defaultWorker) autoImport(managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret, importSecret *corev1.Secret) (metav1.Condition, bool, error) {
	importCondition := metav1.Condition{}

	importClient, restMapper, err := helpers.GenerateClientFromSecret(autoImportSecret)
	if err != nil {
		return importCondition, false, err
	}

	importCondition = metav1.Condition{
		Type:    "ManagedClusterImportSucceeded",
		Status:  metav1.ConditionTrue,
		Message: "Import succeeded",
		Reason:  "ManagedClusterImported",
	}

	err = helpers.ImportManagedClusterFromSecret(importClient, restMapper, w.recorder, importSecret)
	if err != nil {
		importCondition.Status = metav1.ConditionFalse
		importCondition.Message = fmt.Sprintf("Unable to import %s: %s", managedCluster.Name, err.Error())
		importCondition.Reason = "ManagedClusterNotImported"
	}

	return importCondition, true, err
}

type hypershiftDetachedWorker struct {
	recorder     events.Recorder
	client       client.Client
	clientHolder *helpers.ClientHolder
	scheme       *runtime.Scheme
}

var _ autoImportWorker = &hypershiftDetachedWorker{}

func (w *hypershiftDetachedWorker) addFinalizer(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	patch := client.MergeFrom(managedCluster.DeepCopy())
	for i := range managedCluster.Finalizers {
		if managedCluster.Finalizers[i] == finalizerExternalManagedSecret {
			return nil
		}
	}

	managedCluster.Finalizers = append(managedCluster.Finalizers, finalizerExternalManagedSecret)
	if err := w.client.Patch(ctx, managedCluster, patch); err != nil {
		return err
	}

	w.recorder.Eventf("ManagedClusterFinalizerAdded",
		"The managedcluster %s finalizer %s is added", managedCluster.Name, finalizerExternalManagedSecret)
	return nil
}

func (w *hypershiftDetachedWorker) removeImportFinalizer(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	hasFinalizer := false

	for _, finalizer := range managedCluster.Finalizers {
		if finalizer == finalizerExternalManagedSecret {
			hasFinalizer = true
			break
		}
	}

	if !hasFinalizer {
		log.Info(fmt.Sprintf("the managedCluster %s does not have external managed secret finalizer, skip it", managedCluster.Name))
		return nil
	}

	managementCluster, err := helpers.GetManagementCluster(managedCluster)
	if err != nil {
		return err
	}
	mwName := externalManagedKubeconfigManifestWorkName(managedCluster.Name)
	err = w.deleteManifestWork(ctx, managementCluster, mwName)
	if err != nil {
		return err
	}

	w.recorder.Eventf("ExternalManagedKubeconfigManifestWorkDeleted",
		"The external managed kubeconfig manifest work %s/%s for managed cluster %s is deleted",
		managementCluster, mwName, managedCluster.Name)

	patch := client.MergeFrom(managedCluster.DeepCopy())
	finalizer := []string{}
	for _, v := range managedCluster.Finalizers {
		if v != finalizerExternalManagedSecret {
			finalizer = append(finalizer, v)
		}
	}
	managedCluster.Finalizers = finalizer
	if err := w.client.Patch(ctx, managedCluster, patch); err != nil {
		return err
	}

	w.recorder.Eventf("ManagedClusterFinalizerRemoved",
		"The managedcluster %s finalizer %s is removed", managedCluster.Name, finalizerExternalManagedSecret)
	return nil
}

func (w *hypershiftDetachedWorker) deleteManifestWork(ctx context.Context, namespace, name string) error {
	manifestWork := &workv1.ManifestWork{}
	err := w.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, manifestWork)
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

	if err := w.clientHolder.RuntimeClient.Delete(ctx, manifestWork); err != nil {
		return err
	}

	w.recorder.Eventf("ManifestWorksDeleted", fmt.Sprintf("The manifest work %s/%s is deleted", namespace, name))
	return nil
}

func (w *hypershiftDetachedWorker) autoImport(managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret, importSecret *corev1.Secret) (metav1.Condition, bool, error) {
	importCondition := metav1.Condition{}
	managementCluster, err := helpers.GetManagementCluster(managedCluster)
	if err != nil {
		return importCondition, false, err
	}
	manifestWork, err := w.createManagedKubeconfigManifestWork(managedCluster.Name, autoImportSecret, managementCluster)
	if err != nil {
		return importCondition, false, err
	}

	importCondition = metav1.Condition{
		Type:    "ExternalManagedKubeconfigCreatedSucceeded",
		Status:  metav1.ConditionTrue,
		Message: "Created succeeded",
		Reason:  "ExternalManagedKubeconfigCreated",
	}
	if err := helpers.ApplyResources(
		w.clientHolder,
		w.recorder,
		w.scheme,
		managedCluster,
		manifestWork,
	); err != nil {
		importCondition.Status = metav1.ConditionFalse
		importCondition.Message = fmt.Sprintf("Unable to create external managed kubeconfig for %s: %s", managedCluster.Name, err.Error())
		importCondition.Reason = "ExternalManagedKubeconfigNotCreated"
	}

	return importCondition, true, err
}

func (w *hypershiftDetachedWorker) createManagedKubeconfigManifestWork(managedClusterName string, importSecret *corev1.Secret,
	manifestWorkNamespace string) (*workv1.ManifestWork, error) {
	kubeconfig := importSecret.Data["kubeconfig"]
	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("import secret invalid, the field kubeconfig must exist in the secret for detached mode")
	}

	config := struct {
		KlusterletNamespace       string
		ExternalManagedKubeconfig string
	}{
		KlusterletNamespace:       fmt.Sprintf("klusterlet-%s", managedClusterName),
		ExternalManagedKubeconfig: base64.StdEncoding.EncodeToString(kubeconfig),
	}

	template, err := manifestFiles.ReadFile(klusterletDetachedExternalKubeconfig)
	if err != nil {
		return nil, err
	}
	externalKubeYAML := helpers.MustCreateAssetFromTemplate(klusterletDetachedExternalKubeconfig, template, config)
	externalKubeJSON, err := yaml.YAMLToJSON(externalKubeYAML)
	if err != nil {
		return nil, err
	}

	manifests := []workv1.Manifest{
		{
			RawExtension: runtime.RawExtension{Raw: externalKubeJSON},
		},
	}

	mw := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalManagedKubeconfigManifestWorkName(managedClusterName),
			Namespace: manifestWorkNamespace,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				// For detached mode, we will not delete the "external-managed-kubeconfig" since
				// klusterlet operator will use this secret to clean resources on the managed
				// cluster, after the cleanup finished, the klusterlet operator will delete the
				// secret.
				PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			},
		},
	}

	return mw, nil
}
