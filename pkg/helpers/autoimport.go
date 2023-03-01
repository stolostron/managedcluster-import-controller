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
	"k8s.io/client-go/kubernetes"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

func ClusterNotNeedToApplyImportResources(mc *clusterv1.ManagedCluster) bool {
	return meta.IsStatusConditionTrue(mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded) &&
		meta.IsStatusConditionTrue(mc.Status.Conditions, clusterv1.ManagedClusterConditionAvailable)
}

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

// ApplyResourcesFunc is a function to apply resources to the manged cluster to import it to the hub
type ApplyResourcesFunc func(backupRestore bool, client *ClientHolder, restMapper meta.RESTMapper,
	recorder events.Recorder, importSecret *corev1.Secret) error

// GenerateClientHolderFunc is a function to generate the managed cluster client holder which is
// used to import cluster(apply resources to the managed cluster)
type GenerateClientHolderFunc func(secret *corev1.Secret) (*ClientHolder, meta.RESTMapper, error)

// ImportHelper is used to helper controller to import managed cluster
type ImportHelper struct {
	informerHolder *source.InformerHolder
	recorder       events.Recorder
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
	recorder events.Recorder, log logr.Logger) *ImportHelper {
	return &ImportHelper{
		informerHolder: informerHolder,
		recorder:       recorder,
		log:            log,

		generateClientHolderFunc: GenerateClientFromSecret,
		applyResourcesFunc:       defaultapplyResourcesFunc,
	}
}

func defaultapplyResourcesFunc(backupRestore bool,
	client *ClientHolder, restMapper meta.RESTMapper, recorder events.Recorder, importSecret *corev1.Secret) error {
	if backupRestore {
		// only update the bootstrap secret on the managed cluster with the auto-import-secret
		return UpdateManagedClusterBootstrapSecret(client, importSecret, recorder)
	}
	return ImportManagedClusterFromSecret(client, restMapper, recorder, importSecret)
}

// Import uses the managedClusterKubeClientSecret to generate a managed cluster client,
// then use this client to import the managed cluster, return condition when finished
// apply
func (i *ImportHelper) Import(backupRestore bool, clusterName string,
	managedClusterKubeClientSecret *corev1.Secret, lastRetry, totalRetry int) (
	reconcile.Result, metav1.Condition, int, error) {

	reqLogger := i.log.WithValues("Request.Name", clusterName)
	currentRetry := lastRetry

	clientHolder, restMapper, err := i.generateClientHolderFunc(managedClusterKubeClientSecret)
	if err != nil {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonAutoImportSecretInvalid,
				fmt.Sprintf("Try to import managed cluster, generate kube client by secret error: %v", err),
			), currentRetry, err
	}

	importSecretName := fmt.Sprintf("%s-%s", clusterName, constants.ImportSecretNameSuffix)
	importSecret, err := i.informerHolder.ImportSecretLister.Secrets(clusterName).Get(importSecretName)
	if errors.IsNotFound(err) {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonImportSecretNotReady,
				"Import secret does not exist",
			), currentRetry, nil
	}
	if err != nil {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonImportSecretNotReady,
				fmt.Sprintf("Get import secret failed: %v", err),
			), currentRetry, err
	}

	// ensure the klusterlet manifest works exist
	workSelector := labels.SelectorFromSet(map[string]string{constants.KlusterletWorksLabel: "true"})
	manifestWorks, err := i.informerHolder.KlusterletWorkLister.ManifestWorks(clusterName).List(workSelector)
	if err != nil {
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonKlusterletManifestWorkNotReady,
				fmt.Sprintf("Get klusterlet manifestwork failed: %v", err),
			), currentRetry, err
	}
	if len(manifestWorks) != 2 {
		reqLogger.Info(fmt.Sprintf("Waiting for klusterlet manifest works for managed cluster %s", clusterName))
		return reconcile.Result{},
			NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonKlusterletManifestWorkNotReady,
				fmt.Sprintf("Expect 2 manifestwork, but got %v", len(manifestWorks)),
			), currentRetry, nil
	}

	currentRetry++
	err = i.applyResourcesFunc(backupRestore, clientHolder, restMapper, i.recorder, importSecret)
	if err != nil {
		if ContainAuthError(err) {
			// return an invalid auto import secret reason so that the user knows that
			// a correct secret needs to be re-provided
			return reconcile.Result{RequeueAfter: 10 * time.Second},
				NewManagedClusterImportSucceededCondition(
					metav1.ConditionFalse,
					constants.ConditionReasonAutoImportSecretInvalid,
					fmt.Sprintf("Try to import managed cluster, retry times: %d/%d, error: %v",
						currentRetry, totalRetry, err),
				), currentRetry, err
		}

		// maybe some network issue, retry after 10 second
		condition := NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonApplyResourcesFailed,
			fmt.Sprintf("Try to import managed cluster, retry times: %d/%d, error: %v",
				currentRetry, totalRetry, err),
		)
		if currentRetry >= totalRetry {
			// stop retrying
			condition.Reason = constants.ConditionReasonManagedClusterImportFailed
			return reconcile.Result{}, condition, currentRetry, nil
		}
		return reconcile.Result{RequeueAfter: 10 * time.Second},
			condition, currentRetry, nil
	}

	return reconcile.Result{},
		NewManagedClusterImportSucceededCondition(
			metav1.ConditionFalse,
			constants.ConditionReasonManagedClusterImporting,
			fmt.Sprintf("Try to import managed cluster, retry times: %d/%d", currentRetry, totalRetry),
		), currentRetry, nil
}
