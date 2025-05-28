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
	corev1listers "k8s.io/client-go/listers/core/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
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

// GenerateClientHolderFunc is a function to generate the managed cluster client holder which is
// used to import cluster(apply resources to the managed cluster)
type GenerateClientHolderFunc func(secret *corev1.Secret) (reconcile.Result, *ClientHolder, meta.RESTMapper, error)

// ImportHelper is used to helper controller to import managed cluster
type ImportHelper struct {
	informerHolder *source.InformerHolder
	recorder       events.Recorder
	log            logr.Logger

	generateClientHolderFunc GenerateClientHolderFunc
}

func (i *ImportHelper) WithGenerateClientHolderFunc(f GenerateClientHolderFunc) *ImportHelper {
	i.generateClientHolderFunc = f
	return i
}

func NewImportHelper(informerHolder *source.InformerHolder,
	recorder events.Recorder,
	log logr.Logger) *ImportHelper {
	return &ImportHelper{
		informerHolder: informerHolder,
		log:            log,
		recorder:       recorder,
	}
}

func applyResourcesFunc(backupRestore bool, client *ClientHolder,
	restMapper meta.RESTMapper, recorder events.Recorder, importSecret *corev1.Secret) (bool, error) {
	if backupRestore {
		// only update the bootstrap secret on the managed cluster with the auto-import-secret
		return UpdateManagedClusterBootstrapSecret(client, importSecret, recorder)
	}
	return ImportManagedClusterFromSecret(client, restMapper, recorder, importSecret)
}

// Import uses the managedClusterKubeClientSecret to generate a managed cluster client,
// then use this client to import the managed cluster, return managed cluster import condition
// when finished apply.
func (i *ImportHelper) Import(backupRestore bool, cluster *clusterv1.ManagedCluster,
	managedClusterKubeClientSecret *corev1.Secret) (
	reconcile.Result, metav1.Condition, bool, error) {
	if i.generateClientHolderFunc == nil {
		// if generateClientHolderFunc is nil, panic here
		utilruntime.Must(fmt.Errorf("the generateClientHolderFunc in the ImportHelper is nil"))
	}

	clusterName := cluster.Name
	reqLogger := i.log.WithValues("Request.Name", clusterName)

	// move to importing state for the condition
	ic := meta.FindStatusCondition(cluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if ic == nil || ic.Reason == constants.ConditionReasonManagedClusterWaitForImporting {
		return reconcile.Result{Requeue: true},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				"Start to import managed cluster",
			), false, nil
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
			), false, err
	}
	// This check is to ensure when the hub is restored on another cluster, the klusterlet manifestworks are
	// created before calling the managed cluster to import. Otherwise, it is possible that klusterlet-agent
	// evicts the klusterlet manifestworks after it is imported to the new hub and klusterlet manifestworks
	// has not been created yet.
	// We do not need to check the specific number, since we do not know if there is crd manifestworks created.
	// It will not be created if installMode is noOperator.
	if errors.IsNotFound(err) || len(manifestWorks) == 0 {
		reqLogger.Info(fmt.Sprintf("Waiting for klusterlet manifest works for managed cluster %s", clusterName))
		return reconcile.Result{RequeueAfter: 3 * time.Second},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				"Expect klusterlet manifestworks created, but got none. Will retry",
			), false, nil
	}

	// build import client with managed cluster kube client secret
	result, clientHolder, restMapper, err := i.generateClientHolderFunc(managedClusterKubeClientSecret)
	if err != nil {
		return result,
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImportFailed,
				failureMessageOfInvalidAutoImportSecret(managedClusterKubeClientSecret, err),
			), false, err
	}

	importSecretName := fmt.Sprintf("%s-%s", clusterName, constants.ImportSecretNameSuffix)
	importSecret, err := i.informerHolder.ImportSecretLister.Secrets(clusterName).Get(importSecretName)
	if errors.IsNotFound(err) {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImporting,
				"Wait for import secret",
			), false, nil
	}
	if err != nil {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImportFailed,
				fmt.Sprintf("Get import secret failed: %v. Will retry", err),
			), false, err
	}

	modified, err := applyResourcesFunc(backupRestore, clientHolder, restMapper, i.recorder, importSecret)
	if err != nil {
		condition := NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImportFailed,
			fmt.Sprintf("Try to import managed cluster, error: %v", err),
		)

		if ContainAuthError(err) {
			// return message reflects the auto import secret is invalid, so the user knows that
			// a correct secret needs to be re-provided
			condition.Message = failureMessageOfInvalidAutoImportSecretPrivileges(
				managedClusterKubeClientSecret, err)
		}

		if ContainInternalServerError(err) {
			// might be some internal server error, does not take up retry times
			condition.Reason = constants.ConditionReasonManagedClusterImporting
			condition.Message = fmt.Sprintf(
				"Try to import managed cluster, apply resources error: %v. Will Retry", err)
		}

		return reconcile.Result{}, condition, modified, err
	}

	return reconcile.Result{},
		NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImporting,
			conditionMessageImportingResourcesApplied,
		), modified, nil
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

type AutoImportStrategyGetterFunc func() (strategy string, err error)

func AutoImportStrategyGetter(componentNamespace string, configMapLister corev1listers.ConfigMapLister, log logr.Logger) AutoImportStrategyGetterFunc {
	return func() (string, error) {
		cm, err := configMapLister.ConfigMaps(componentNamespace).Get(constants.ControllerConfigConfigMapName)
		if errors.IsNotFound(err) {
			return constants.DefaultAutoImportStrategy, nil
		}
		if err != nil {
			return "", err
		}

		strategy := cm.Data[constants.AutoImportStrategyKey]
		switch strategy {
		case apiconstants.AutoImportStrategyImportAndSync, apiconstants.AutoImportStrategyImportOnly:
			return strategy, nil
		case "":
			return constants.DefaultAutoImportStrategy, nil
		default:
			log.Info("Invalid config value found and use default instead.",
				"configmap", constants.ControllerConfigConfigMapName,
				constants.AutoImportStrategyKey, strategy,
				"default", constants.DefaultAutoImportStrategy)
			return constants.DefaultAutoImportStrategy, nil
		}
	}
}
