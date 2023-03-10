// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

var log = logf.Log.WithName(controllerName)

// ReconcileAutoImport reconciles the managed cluster auto import secret to import the managed cluster
type ReconcileAutoImport struct {
	client         client.Client
	kubeClient     kubernetes.Interface
	informerHolder *source.InformerHolder
	recorder       events.Recorder
	importHelper   *helpers.ImportHelper
}

func NewReconcileAutoImport(
	client client.Client,
	kubeClient kubernetes.Interface,
	informerHolder *source.InformerHolder,
	recorder events.Recorder,
) *ReconcileAutoImport {

	return &ReconcileAutoImport{
		client:         client,
		kubeClient:     kubeClient,
		informerHolder: informerHolder,
		recorder:       recorder,
		importHelper:   helpers.NewImportHelper(informerHolder, recorder, log),
	}
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

	if !managedCluster.DeletionTimestamp.IsZero() {
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

	reqLogger.V(5).Info("Reconciling auto import secret")

	lastRetry := 0
	totalRetry := 1
	if len(autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry]) != 0 {
		lastRetry, err = strconv.Atoi(autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry])
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if len(autoImportSecret.Data[constants.AutoImportRetryName]) != 0 {
		totalRetry, err = strconv.Atoi(string(autoImportSecret.Data[constants.AutoImportRetryName]))
		if err != nil {
			if err := helpers.UpdateManagedClusterStatus(
				r.client,
				r.recorder,
				managedClusterName,
				helpers.NewManagedClusterImportSucceededCondition(
					metav1.ConditionFalse,
					constants.ConditionReasonManagedClusterImportFailed,
					fmt.Sprintf("AutoImportSecretInvalid %s/%s; please check the value %s",
						autoImportSecret.Namespace, autoImportSecret.Name, constants.AutoImportRetryName),
				),
			); err != nil {
				return reconcile.Result{}, err
			}
			// auto import secret invalid, stop retrying
			return reconcile.Result{}, nil
		}
	}

	backupRestore := false
	if v, ok := autoImportSecret.Labels[constants.LabelAutoImportRestore]; ok && strings.EqualFold(v, "true") {
		backupRestore = true
	}

	result, condition, modified, currentRetry, iErr := r.importHelper.Import(
		backupRestore, managedClusterName, autoImportSecret, lastRetry, totalRetry)
	// if resources are applied but NOT modified, will not update the condition, keep the original condition.
	// This check is to prevent the current controller and import status controller from modifying the
	// ManagedClusterImportSucceeded condition of the managed cluster in a loop
	if !helpers.ImportingResourcesApplied(&condition) || modified {
		if err := helpers.UpdateManagedClusterStatus(
			r.client,
			r.recorder,
			managedClusterName,
			condition,
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.V(5).Info("Import result", "importError", iErr, "condition", condition,
		"current", currentRetry, "result", result, "modified", modified)

	if condition.Reason == constants.ConditionReasonManagedClusterImportFailed ||
		helpers.ImportingResourcesApplied(&condition) {
		// delete secret
		if err := helpers.DeleteAutoImportSecret(ctx, r.kubeClient, autoImportSecret, r.recorder); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if lastRetry < currentRetry && currentRetry < totalRetry {
		// update secret
		if autoImportSecret.Annotations == nil {
			autoImportSecret.Annotations = make(map[string]string)
		}
		autoImportSecret.Annotations[constants.AnnotationAutoImportCurrentRetry] = strconv.Itoa(currentRetry)

		if _, err := r.kubeClient.CoreV1().Secrets(managedClusterName).Update(
			ctx, autoImportSecret, metav1.UpdateOptions{},
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	return result, iErr
}
