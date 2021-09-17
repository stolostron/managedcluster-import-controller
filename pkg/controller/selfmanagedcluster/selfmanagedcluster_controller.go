// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package selfmanagedcluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/openshift/library-go/pkg/operator/events"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(controllerName)

// ReconcileLocalCluster reconciles the import secret of a self managed cluster to import the managed cluster
type ReconcileLocalCluster struct {
	clientHolder *helpers.ClientHolder
	restMapper   meta.RESTMapper
	scheme       *runtime.Scheme
	recorder     events.Recorder
}

// Reconcile reconciles the import secret to of a self managed cluster to import the managed cluster
// A a self managed cluster must have self managed label and the label value must be true
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
//func (r *ReconcileLocalCluster) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
func (r *ReconcileLocalCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling self managed cluster")

	ctx := context.TODO()
	managedCluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if selfManaged, ok := managedCluster.Labels[constants.SelfManagedLabel]; !ok || !strings.EqualFold(selfManaged, "true") {
		log.Info(fmt.Sprintf("The managed cluster %s is not self managed cluster", request.Name))
		return reconcile.Result{}, nil
	}

	// if there is an auto import secret in the managed cluster namespce, we will use the auto import secret to import
	// the cluster
	err = r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{
		Namespace: request.Name,
		Name:      constants.AutoImportSecretName,
	}, &corev1.Secret{})
	if err == nil {
		log.Info(fmt.Sprintf("The self managed cluster %s has auto import secret, skipped", request.Name))
		return reconcile.Result{}, nil
	}
	if !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	importSecretName := fmt.Sprintf("%s-%s", request.Name, constants.ImportSecretNameSuffix)
	importSecretKey := types.NamespacedName{Namespace: request.Name, Name: importSecretName}
	importSecret := &corev1.Secret{}
	err = r.clientHolder.RuntimeClient.Get(ctx, importSecretKey, importSecret)
	if errors.IsNotFound(err) {
		// the import secret could have been deleted, do nothing
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

	errs := []error{}
	err = helpers.ImportManagedClusterFromSecret(r.clientHolder, r.restMapper, r.recorder, importSecret)
	if err != nil {
		errs = append(errs, err)

		importCondition.Status = metav1.ConditionFalse
		importCondition.Message = fmt.Sprintf("Unable to import %s: %s", request.Name, err.Error())
		importCondition.Reason = "ManagedClusterNotImported"
	}

	err = helpers.UpdateManagedClusterStatus(r.clientHolder.RuntimeClient, r.recorder, request.Name, importCondition)
	if err != nil {
		errs = append(errs, err)
	}

	return reconcile.Result{}, utilerrors.NewAggregate(errs)
}
