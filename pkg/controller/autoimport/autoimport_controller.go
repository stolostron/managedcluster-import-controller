// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	"github.com/openshift/library-go/pkg/operator/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

const autoImportRetryName string = "autoImportRetry"

const finalizerExternalManagedSecret string = "managedcluster-import-controller.open-cluster-management.io/cleanup-external-managed-kubeconfig"

//go:embed manifests
var manifestFiles embed.FS

var klusterletDetachedExternalKubeconfig = "manifests/external_managed_secret.yaml"

var log = logf.Log.WithName(controllerName)

// ReconcileAutoImport reconciles the managed cluster auto import secret to import the managed cluster
type ReconcileAutoImport struct {
	client     client.Client
	kubeClient kubernetes.Interface
	recorder   events.Recorder

	// useful in detached mode
	clientHolder *helpers.ClientHolder
	scheme       *runtime.Scheme
}

// blank assignment to verify that ReconcileAutoImport implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAutoImport{}

// Reconcile the managed cluster auto import secret to import the managed cluster
// Once the managed cluster is imported, the auto import secret will be deleted
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAutoImport) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace)
	reqLogger.Info("Reconciling auto import secret")

	managedClusterName := request.Namespace
	managedCluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	mode, management, err := helpers.DetermineKlusterletMode(managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	if mode == operatorv1.InstallModeDetached {
		if !managedCluster.DeletionTimestamp.IsZero() {
			return reconcile.Result{}, r.removeImportFinalizer(ctx, managedCluster, management)
		}

		err = r.addFinalizer(ctx, managedCluster)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// TODO: we will use list instead of get to reduce the request in the future
	autoImportSecret, err := r.kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, constants.AutoImportSecretName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// the auto import secret could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, importSecretName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// there is no import secret, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	importCondition := metav1.Condition{}
	errs := []error{}
	if mode == operatorv1.InstallModeDetached {
		manifestWork, err := createManagedKubeconfigManifestWork(managedCluster.Name, autoImportSecret, management)
		if err != nil {
			return reconcile.Result{}, err
		}

		importCondition = metav1.Condition{
			Type:    "ExternalManagedKubeconfigCreatedSucceeded",
			Status:  metav1.ConditionTrue,
			Message: "Created succeeded",
			Reason:  "ExternalManagedKubeconfigCreated",
		}
		if err := helpers.ApplyResources(
			r.clientHolder,
			r.recorder,
			r.scheme,
			managedCluster,
			manifestWork,
		); err != nil {
			importCondition.Status = metav1.ConditionFalse
			importCondition.Message = fmt.Sprintf("Unable to create external managed kubeconfig for %s: %s", managedClusterName, err.Error())
			importCondition.Reason = "ExternalManagedKubeconfigNotCreated"

			errs = append(errs, err, r.updateAutoImportRetryTimes(ctx, autoImportSecret.DeepCopy()))
		}
	} else {
		importClient, restMapper, err := helpers.GenerateClientFromSecret(autoImportSecret)
		if err != nil {
			return reconcile.Result{}, err
		}

		importCondition = metav1.Condition{
			Type:    "ManagedClusterImportSucceeded",
			Status:  metav1.ConditionTrue,
			Message: "Import succeeded",
			Reason:  "ManagedClusterImported",
		}

		err = helpers.ImportManagedClusterFromSecret(importClient, restMapper, r.recorder, importSecret)
		if err != nil {
			importCondition.Status = metav1.ConditionFalse
			importCondition.Message = fmt.Sprintf("Unable to import %s: %s", managedClusterName, err.Error())
			importCondition.Reason = "ManagedClusterNotImported"

			errs = append(errs, err, r.updateAutoImportRetryTimes(ctx, autoImportSecret.DeepCopy()))
		}
	}

	if len(errs) == 0 {
		err := r.kubeClient.CoreV1().Secrets(autoImportSecret.Namespace).Delete(ctx, autoImportSecret.Name, metav1.DeleteOptions{})
		if err != nil {
			errs = append(errs, err)
		}

		r.recorder.Eventf("AutoImportSecretDeleted",
			fmt.Sprintf("The managed cluster %s is imported, delete its auto import secret", managedClusterName))
	}

	if err := helpers.UpdateManagedClusterStatus(r.client, r.recorder, managedClusterName, importCondition); err != nil {
		errs = append(errs, err)
	}

	return reconcile.Result{}, utilerrors.NewAggregate(errs)
}

func (r *ReconcileAutoImport) updateAutoImportRetryTimes(ctx context.Context, secret *corev1.Secret) error {
	autoImportRetry, err := strconv.Atoi(string(secret.Data[autoImportRetryName]))
	if err != nil {
		r.recorder.Warningf("AutoImportRetryInvalid", "The value of autoImportRetry is invalid in auto-import-secret secret")
		return err
	}

	r.recorder.Eventf("RetryToImportCluster", "Retry to import cluster %s, %d", secret.Namespace, autoImportRetry)

	autoImportRetry--
	if autoImportRetry < 0 {
		// stop retry, delete the auto-import-secret
		err := r.kubeClient.CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		r.recorder.Eventf("AutoImportSecretDeleted",
			fmt.Sprintf("Exceed the retry times, delete the auto import secret %s/%s", secret.Namespace, secret.Name))
		return nil
	}

	secret.Data[autoImportRetryName] = []byte(strconv.Itoa(autoImportRetry))
	_, err = r.kubeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

func (r *ReconcileAutoImport) addFinalizer(
	ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	patch := client.MergeFrom(managedCluster.DeepCopy())
	for i := range managedCluster.Finalizers {
		if managedCluster.Finalizers[i] == finalizerExternalManagedSecret {
			return nil
		}
	}

	managedCluster.Finalizers = append(managedCluster.Finalizers, finalizerExternalManagedSecret)
	if err := r.client.Patch(ctx, managedCluster, patch); err != nil {
		return err
	}

	r.recorder.Eventf("ManagedClusterFinalizerAdded",
		"The managedcluster %s finalizer %s is added", managedCluster.Name, finalizerExternalManagedSecret)
	return nil
}

func (r *ReconcileAutoImport) removeImportFinalizer(ctx context.Context, managedCluster *clusterv1.ManagedCluster,
	management string) error {
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

	mwName := externalManagedKubeconfigManifestWorkName(managedCluster.Name)
	err := r.deleteManifestWork(ctx, management, mwName)
	if err != nil {
		return err
	}

	r.recorder.Eventf("ExternalManagedKubeconfigManifestWorkDeleted",
		"The external managed kubeconfig manifest work %s/%s for managed cluster %s is deleted",
		management, mwName, managedCluster.Name)

	patch := client.MergeFrom(managedCluster.DeepCopy())
	finalizer := []string{}
	for _, v := range managedCluster.Finalizers {
		if v != finalizerExternalManagedSecret {
			finalizer = append(finalizer, v)
		}
	}
	managedCluster.Finalizers = finalizer
	if err := r.client.Patch(ctx, managedCluster, patch); err != nil {
		return err
	}

	r.recorder.Eventf("ManagedClusterFinalizerRemoved",
		"The managedcluster %s finalizer %s is removed", managedCluster.Name, finalizerExternalManagedSecret)
	return nil
}

func (r *ReconcileAutoImport) deleteManifestWork(ctx context.Context, namespace, name string) error {
	manifestWork := &workv1.ManifestWork{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, manifestWork)
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

	if err := r.clientHolder.RuntimeClient.Delete(ctx, manifestWork); err != nil {
		return err
	}

	r.recorder.Eventf("ManifestWorksDeleted", fmt.Sprintf("The manifest work %s/%s is deleted", namespace, name))
	return nil
}

func createManagedKubeconfigManifestWork(managedClusterName string, importSecret *corev1.Secret,
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

func externalManagedKubeconfigManifestWorkName(managedClusterName string) string {
	return fmt.Sprintf("%s-klusterlet-managed-kubeconfig", managedClusterName)
}
