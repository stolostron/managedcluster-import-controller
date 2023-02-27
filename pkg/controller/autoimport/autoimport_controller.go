// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/library-go/pkg/operator/events"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

const restoreLabel = "cluster.open-cluster-management.io/restore-auto-import-secret"

var log = logf.Log.WithName(controllerName)

// ReconcileAutoImport reconciles the managed cluster auto import secret to import the managed cluster
type ReconcileAutoImport struct {
	client         client.Client
	kubeClient     kubernetes.Interface
	workClient     workclient.Interface
	informerHolder *source.InformerHolder
	recorder       events.Recorder
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

	if helpers.DetermineKlusterletMode(managedCluster) != constants.KlusterletDeployModeDefault {
		return reconcile.Result{}, nil
	}

	autoImportSecret, err := r.informerHolder.AutoImportSecretLister.Secrets(managedClusterName).Get(constants.AutoImportSecretName)
	if errors.IsNotFound(err) {
		// the auto import secret could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	currentRetry := 0
	if len(autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry]) != 0 {
		currentRetry, err = strconv.Atoi(autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry])
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info("Reconciling auto import secret")

	result, condition, rc, iErr := r.autoImport(ctx, managedClusterName, autoImportSecret)
	if condition != nil {
		if err := helpers.UpdateManagedClusterStatus(
			r.client,
			r.recorder,
			managedClusterName,
			*condition,
		); err != nil {
			return reconcile.Result{}, err
		}
	}
	if condition.Status == metav1.ConditionFalse && currentRetry < rc.current && rc.current < rc.total {
		// update secret
		if autoImportSecret.Annotations == nil {
			autoImportSecret.Annotations = make(map[string]string)
		}
		autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry] = strconv.Itoa(rc.current)

		if _, err := r.kubeClient.CoreV1().Secrets(managedClusterName).Update(
			ctx, autoImportSecret, metav1.UpdateOptions{},
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	if iErr == nil {
		if condition.Status == metav1.ConditionTrue {
			reqLogger.Info(fmt.Sprintf("Managed cluster is imported, try to delete its auto import secret %s/%s",
				autoImportSecret.Namespace, autoImportSecret.Name))
			// delete secret
			if err := helpers.DeleteAutoImportSecret(ctx, r.kubeClient, autoImportSecret, r.recorder); err != nil {
				return reconcile.Result{}, err
			}
		}

		return result, nil
	}

	if rc.current >= rc.total {
		r.recorder.Eventf("RetryTimesExceeded",
			fmt.Sprintf("Exceed the retry times, delete the auto import secret %s/%s",
				managedClusterName, autoImportSecret.Name))
		// delete secret
		if err := helpers.DeleteAutoImportSecret(ctx, r.kubeClient, autoImportSecret, r.recorder); err != nil {
			return reconcile.Result{}, err
		}
	}
	return result, iErr
}

func newManagedClusterImportSucceededCondition(s metav1.ConditionStatus, reason, message string) *metav1.Condition {
	return &metav1.Condition{
		Type:    constants.ConditionManagedClusterImportSucceeded,
		Status:  s,
		Reason:  reason,
		Message: message,
	}
}

type importRetryCount struct {
	total   int
	current int
}

func (r *ReconcileAutoImport) autoImport(ctx context.Context, managedClusterName string,
	autoImportSecret *v1.Secret) (reconcile.Result, *metav1.Condition, importRetryCount, error) {

	reqLogger := log.WithValues("Request.Namespace", managedClusterName)

	var err error
	rc := importRetryCount{
		total:   1,
		current: 0,
	}
	if len(autoImportSecret.Data[constants.AutoImportRetryName]) != 0 {
		rc.total, err = strconv.Atoi(string(autoImportSecret.Data[constants.AutoImportRetryName]))
		if err != nil {
			r.recorder.Warningf("AutoImportRetryInvalid", "The value of autoImportRetry is invalid in auto-import-secret secret")
			return reconcile.Result{},
				newManagedClusterImportSucceededCondition(
					metav1.ConditionFalse,
					constants.ConditionReasonAutoImportSecretInvalid,
					fmt.Sprintf("Invalid value of %s", constants.AutoImportRetryName),
				), rc, nil
		}
	}

	if len(autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry]) != 0 {
		rc.current, err = strconv.Atoi(autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry])
		if err != nil {
			return reconcile.Result{}, nil, rc, err
		}
	}

	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.informerHolder.ImportSecretLister.Secrets(managedClusterName).Get(importSecretName)
	if errors.IsNotFound(err) {
		return reconcile.Result{},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonImportSecretNotReady,
				"Import secret does not exist",
			), rc, nil
	}
	if err != nil {
		return reconcile.Result{},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonImportSecretNotReady,
				fmt.Sprintf("Get import secret failed: %v", err),
			), rc, err
	}

	// ensure the klusterlet manifest works exist
	workSelector := labels.SelectorFromSet(map[string]string{constants.KlusterletWorksLabel: "true"})
	manifestWorks, err := r.informerHolder.KlusterletWorkLister.ManifestWorks(managedClusterName).List(workSelector)
	if err != nil {
		return reconcile.Result{},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonKlusterletManifestWorkNotReady,
				fmt.Sprintf("Get klusterlet manifestwork failed: %v", err),
			), rc, err
	}
	if len(manifestWorks) != 2 {
		reqLogger.Info(fmt.Sprintf("Waiting for klusterlet manifest works for managed cluster %s", managedClusterName))
		return reconcile.Result{},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonKlusterletManifestWorkNotReady,
				fmt.Sprintf("Expect 2 manifestwork, but got %v", len(manifestWorks)),
			), rc, nil
	}

	rc.current++
	r.recorder.Eventf("RetryToImportCluster", "Retry to import cluster %s, %d/%d",
		autoImportSecret.Namespace, rc.current, rc.total)

	importClient, restMapper, err := helpers.GenerateClientFromSecret(autoImportSecret)
	if err != nil {
		return reconcile.Result{},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonAutoImportSecretInvalid,
				fmt.Sprintf("Retry to import managed cluster, retry times: %d/%d, generate kube client by secret error: %v",
					rc.current, rc.total, err),
			), rc, err
	}

	if val, ok := autoImportSecret.Labels[restoreLabel]; ok && strings.EqualFold(val, "true") {
		// only update the bootstrap secret on the managed cluster with the auto-import-secret
		err = helpers.UpdateManagedClusterBootstrapSecret(importClient, importSecret, r.recorder)
	} else {
		err = helpers.ImportManagedClusterFromSecret(importClient, restMapper, r.recorder, importSecret)
	}
	if err != nil {
		if containAuthError(err) {
			// return an invalid auto import secret reason so that the user knows that
			// a correct secret needs to be re-provided
			return reconcile.Result{RequeueAfter: 10 * time.Second},
				newManagedClusterImportSucceededCondition(
					metav1.ConditionFalse,
					constants.ConditionReasonAutoImportSecretInvalid,
					fmt.Sprintf("Retry to import managed cluster, retry times: %d/%d, error: %v",
						rc.current, rc.total, err),
				), rc, err
		}

		// maybe some network issue, retry after 10 second
		return reconcile.Result{RequeueAfter: 10 * time.Second},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonImporting,
				fmt.Sprintf("Retry to import managed cluster, retry times: %d/%d, error: %v",
					rc.current, rc.total, err),
			), rc, nil
	}

	result, condition, err := r.checkManifestWorksAvailability(ctx, managedClusterName)
	return result, condition, rc, err
}

func (r *ReconcileAutoImport) checkManifestWorksAvailability(ctx context.Context, managedClusterName string) (
	reconcile.Result, *metav1.Condition, error) {

	available, err := helpers.IsManifestWorksAvailable(ctx, r.workClient, managedClusterName,
		fmt.Sprintf("%s-%s", managedClusterName, constants.KlusterletCRDsSuffix),
		fmt.Sprintf("%s-%s", managedClusterName, constants.KlusterletSuffix))
	if err != nil {
		return reconcile.Result{},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonKlusterletNotAvailable,
				fmt.Sprintf("Check klusterlet availability failed: %v", err),
			), err
	}
	if !available {
		return reconcile.Result{RequeueAfter: 10 * time.Second},
			newManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonKlusterletNotAvailable,
				"The klusterlet works are not available",
			), nil
	}

	return reconcile.Result{RequeueAfter: 10 * time.Second},
		newManagedClusterImportSucceededCondition(
			metav1.ConditionTrue,
			constants.ConditionReasonManagedClusterImported,
			"Import succeeded",
		), nil
}

func containAuthError(err error) bool {
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
