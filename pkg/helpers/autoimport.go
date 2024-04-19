// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

// DeleteAutoImportSecret delete the auto-import-secret if the secret does not have the keeping annotation
func DeleteAutoImportSecret(ctx context.Context, kubeClient kubernetes.Interface,
	secret *corev1.Secret, recorder events.Recorder) error {
	if _, ok := secret.Annotations[constants.AnnotationKeepingAutoImportSecret]; ok {
		recorder.Eventf("AutoImportSecretSaved",
			fmt.Sprintf("The auto import secret %s/%s is saved", secret.Namespace, secret.Name))
		return nil
	}

	if err := kubeClient.CoreV1().Secrets(secret.Namespace).Delete(
		ctx, secret.Name, metav1.DeleteOptions{}); err != nil {
		return err
	}

	recorder.Eventf("AutoImportSecretDeleted",
		fmt.Sprintf("The auto import secret %s/%s is deleted", secret.Namespace, secret.Name))
	return nil
}

func NewManagedClusterImportSucceededCondition(s metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    constants.ConditionManagedClusterImportSucceeded,
		Status:  s,
		Reason:  reason,
		Message: message,
	}
}

func ContainAuthError(err error) bool {
	if errors.IsUnauthorized(err) || errors.IsForbidden(err) {
		return true
	}
	if errs, ok := err.(utilerrors.Aggregate); ok {
		for _, e := range errs.Errors() {
			if errors.IsUnauthorized(e) || errors.IsForbidden(e) {
				return true
			}
		}
	}
	return false
}

func ContainInternalServerError(err error) bool {
	// There should not be a NotFound error, because when applying for a resource,
	// the existence of the resource has been checked before all create requests
	if errors.IsAlreadyExists(err) || errors.IsConflict(err) ||
		errors.IsInternalError(err) || errors.IsTooManyRequests(err) {
		return true
	}
	if errs, ok := err.(utilerrors.Aggregate); ok {
		for _, e := range errs.Errors() {
			if errors.IsAlreadyExists(e) || errors.IsConflict(e) ||
				errors.IsInternalError(e) || errors.IsTooManyRequests(e) {
				return true
			}
		}
	}
	return false
}

// ApplyResourcesFunc is a function to apply resources to the manged cluster to import it to the hub
type ApplyResourcesFunc func(backupRestore bool, client *ClientHolder, restMapper meta.RESTMapper,
	recorder events.Recorder, importSecret *corev1.Secret) (bool, error)

// GenerateClientHolderFunc is a function to generate the managed cluster client holder which is
// used to import cluster(apply resources to the managed cluster)
type GenerateClientHolderFunc func(secret *corev1.Secret) (reconcile.Result, *ClientHolder, meta.RESTMapper, error)

// ImportHelper is used to helper controller to import managed cluster
type ImportHelper struct {
	informerHolder *source.InformerHolder
	globalRecorder events.Recorder
	log            logr.Logger

	generateClientHolderFunc GenerateClientHolderFunc
	applyResourcesFunc       ApplyResourcesFunc
}

func (i *ImportHelper) WithApplyResourcesFunc(f ApplyResourcesFunc) *ImportHelper {
	i.applyResourcesFunc = f
	return i
}

func (i *ImportHelper) WithGenerateClientHolderFunc(f GenerateClientHolderFunc) *ImportHelper {
	i.generateClientHolderFunc = f
	return i
}

func NewImportHelper(informerHolder *source.InformerHolder,
	recorder events.Recorder,
	log logr.Logger) *ImportHelper {
	return &ImportHelper{
		informerHolder:     informerHolder,
		log:                log,
		globalRecorder:     recorder,
		applyResourcesFunc: defaultApplyResourcesFunc,
	}
}

func defaultApplyResourcesFunc(backupRestore bool, client *ClientHolder,
	restMapper meta.RESTMapper, recorder events.Recorder, importSecret *corev1.Secret) (bool, error) {
	if backupRestore {
		// only update the bootstrap secret on the managed cluster with the auto-import-secret
		return UpdateManagedClusterBootstrapSecret(client, importSecret, recorder)
	}
	return ImportManagedClusterFromSecret(client, restMapper, recorder, importSecret)
}

// Import uses the managedClusterKubeClientSecret to generate a managed cluster client,
// then use this client to import the managed cluster, return managed cluster import condition
// when finished apply
func (i *ImportHelper) Import(backupRestore bool, cluster *clusterv1.ManagedCluster,
	managedClusterKubeClientSecret *corev1.Secret, lastRetry, totalRetry int) (
	reconcile.Result, metav1.Condition, bool, int, error) {
	if i.generateClientHolderFunc == nil {
		// if generateClientHolderFunc is nil, panic here
		utilruntime.Must(fmt.Errorf("the generateClientHolderFunc in the ImportHelper is nil"))
	}

	clusterName := cluster.Name
	reqLogger := i.log.WithValues("Request.Name", clusterName)
	currentRetry := lastRetry

	// move to importing state for the condition
	ic := meta.FindStatusCondition(cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if ic == nil || ic.Reason == constants.ConditionReasonManagedClusterWaitForImporting {
		return reconcile.Result{}, NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImporting,
			"Start to import managed cluster",
		), false, currentRetry, nil
	}

	// ensure the klusterlet manifest works exist
	workSelector := labels.SelectorFromSet(map[string]string{constants.KlusterletWorksLabel: "true"})
	manifestWorks, err := i.informerHolder.KlusterletWorkLister.ManifestWorks(clusterName).List(workSelector)
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Get klusterlet manifestwork failed: %v. Will retry", err),
			), false, currentRetry, err
	}
	if errors.IsNotFound(err) || len(manifestWorks) != 2 {
		reqLogger.Info(fmt.Sprintf("Waiting for klusterlet manifest works for managed cluster %s", clusterName))
		return reconcile.Result{RequeueAfter: 3 * time.Second},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Expect 2 manifestworks, but got %v. Will retry", len(manifestWorks)),
			), false, currentRetry, nil
	}

	// build import client with managed cluster kube client secret
	result, clientHolder, restMapper, err := i.generateClientHolderFunc(managedClusterKubeClientSecret)
	if err != nil {
		if result.Requeue {
			return result,
				NewManagedClusterImportSucceededCondition(
					metav1.ConditionFalse,
					constants.ConditionReasonManagedClusterImporting,
					failureMessageOfKubeClientGerneration(managedClusterKubeClientSecret, err),
				), false, currentRetry, nil
		}

		return result,
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImportFailed,
				failureMessageOfInvalidAutoImportSecret(managedClusterKubeClientSecret, err),
			), false, currentRetry, nil
	}

	importSecretName := fmt.Sprintf("%s-%s", clusterName, constants.ImportSecretNameSuffix)
	importSecret, err := i.informerHolder.ImportSecretLister.Secrets(clusterName).Get(importSecretName)
	if errors.IsNotFound(err) {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				"Wait for import secret",
			), false, currentRetry, nil
	}
	if err != nil {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				fmt.Sprintf("Get import secret failed: %v. Will retry", err),
			), false, currentRetry, err
	}

	currentRetry++
	modified, err := i.applyResourcesFunc(backupRestore, clientHolder, restMapper, i.globalRecorder, importSecret)
	if err != nil {
		condition := NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImportFailed,
			fmt.Sprintf("Try to import managed cluster, retry times: %d/%d, error: %v",
				currentRetry, totalRetry, err),
		)

		if ContainAuthError(err) {
			// return message reflects the auto import secret is invalid, so the user knows that
			// a correct secret needs to be re-provided
			condition.Message = failureMessageOfInvalidAutoImportSecretPrivileges(
				managedClusterKubeClientSecret, err)
			return reconcile.Result{}, condition, modified, currentRetry, nil
		}

		if ContainInternalServerError(err) {
			// might be some internal server error, does not take up retry times
			condition.Reason = constants.ConditionReasonManagedClusterImporting
			condition.Message = fmt.Sprintf(
				"Try to import managed cluster, apply resources error: %v. Will Retry", err)
			return reconcile.Result{}, condition, modified, lastRetry, err
		}

		if currentRetry < totalRetry {
			// maybe some network issue, retry after 10 second
			condition.Reason = constants.ConditionReasonManagedClusterImporting
			return reconcile.Result{RequeueAfter: 10 * time.Second},
				condition, modified, currentRetry, nil
		}

		return reconcile.Result{},
			condition, modified, currentRetry, nil
	}

	return reconcile.Result{},
		NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImporting,
			conditionMessageImportingResourcesApplied,
		), modified, currentRetry, nil
}

func failureMessageOfKubeClientGerneration(managedClusterKubeClientSecret *corev1.Secret,
	err error) string {
	if managedClusterKubeClientSecret != nil {
		return fmt.Sprintf("Generate kube client by secret %s/%s, %v",
			managedClusterKubeClientSecret.Namespace, managedClusterKubeClientSecret.Name, err)
	}
	return fmt.Sprintf("Generate kube client error: %v", err)
}

func failureMessageOfInvalidAutoImportSecret(managedClusterKubeClientSecret *corev1.Secret,
	err error) string {
	if managedClusterKubeClientSecret != nil {
		return fmt.Sprintf("AutoImportSecretInvalid %s/%s; generate kube client by secret error: %v",
			managedClusterKubeClientSecret.Namespace, managedClusterKubeClientSecret.Name, err)
	}
	return fmt.Sprintf("Generate kube client error: %v", err)
}

func failureMessageOfInvalidAutoImportSecretPrivileges(managedClusterKubeClientSecret *corev1.Secret,
	err error) string {
	if managedClusterKubeClientSecret != nil {
		return fmt.Sprintf(
			"AutoImportSecretInvalid %s/%s; please check its permission, apply resources error: %v",
			managedClusterKubeClientSecret.Namespace, managedClusterKubeClientSecret.Name, err)
	}
	return fmt.Sprintf(
		"Apply resources error, please check kube client permission: %v", err)
}

const (
	conditionMessageImportingResourcesApplied = "Importing resources are applied, wait for resources be available"
)

func ImportingResourcesApplied(condition *metav1.Condition) bool {
	if condition != nil && condition.Type == constants.ConditionManagedClusterImportSucceeded &&
		condition.Reason == constants.ConditionReasonManagedClusterImporting &&
		condition.Message == conditionMessageImportingResourcesApplied {
		return true
	}
	return false
}
