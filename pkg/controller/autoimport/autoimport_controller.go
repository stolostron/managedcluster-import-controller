// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"fmt"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/openshift/library-go/pkg/operator/events"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(controllerName)

// ReconcileAutoImport reconciles the managed cluster auto import secret to import the managed cluster
type ReconcileAutoImport struct {
	client     client.Client
	kubeClient kubernetes.Interface
	recorder   events.Recorder
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

	// TODO: we will use list instead of get to reduce the request in the future
	autoImportSecret, err := r.kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, constants.AutoImportSecretName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// the auto import secret could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if helpers.DetermineKlusterletMode(managedCluster) != constants.KlusterletDeployModeDefault {
		return reconcile.Result{}, nil
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

	importCondition := metav1.Condition{
		Type:    "ManagedClusterImportSucceeded",
		Status:  metav1.ConditionTrue,
		Message: "Import succeeded",
		Reason:  "ManagedClusterImported",
	}

	importClient, restMapper, importErr := helpers.GenerateClientFromSecret(autoImportSecret)
	switch {
	case importErr != nil:
		// failed to generate import client with auto-import sercet, will reduce the auto-import secret retry times and reconcile again
	case importErr == nil:
		importErr = helpers.ImportManagedClusterFromSecret(importClient, restMapper, r.recorder, importSecret)
	}

	if importErr != nil {
		importCondition.Status = metav1.ConditionFalse
		importCondition.Message = fmt.Sprintf("Unable to import managed cluster %s with auto-import-secret: %s", managedClusterName, importErr.Error())
		importCondition.Reason = "ManagedClusterNotImported"

		if err := helpers.UpdateManagedClusterStatus(r.client, r.recorder, managedClusterName, importCondition); err != nil {
			return reconcile.Result{}, err
		}

		// failed to apply the import secrect, reduce the retry times and reconcile again
		return reconcile.Result{}, helpers.UpdateAutoImportRetryTimes(ctx, r.kubeClient, r.recorder, autoImportSecret.DeepCopy())
	}

	// TODO enhancment: check klusterlet status from managed cluster

	if err := helpers.UpdateManagedClusterStatus(r.client, r.recorder, managedClusterName, importCondition); err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.DeleteAutoImportSecret(ctx, r.kubeClient, autoImportSecret); err != nil {
		return reconcile.Result{}, err
	}

	r.recorder.Eventf("AutoImportSecretDeleted",
		fmt.Sprintf("The managed cluster %s is imported, delete its auto import secret", managedClusterName))
	return reconcile.Result{}, nil
}
