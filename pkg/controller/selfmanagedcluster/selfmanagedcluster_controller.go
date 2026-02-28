// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package selfmanagedcluster

import (
	"context"
	"strings"

	"github.com/openshift/library-go/pkg/operator/events"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

var log = logf.Log.WithName(controllerName)

// ReconcileLocalCluster reconciles the import secret of a self managed cluster to import the managed cluster
type ReconcileLocalCluster struct {
	clientHolder   *helpers.ClientHolder
	restMapper     meta.RESTMapper
	informerHolder *source.InformerHolder
	recorder       events.Recorder
	mcRecorder     kevents.EventRecorder
	importHelper   *helpers.ImportHelper
}

func NewReconcileLocalCluster(
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	restMapper meta.RESTMapper,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
) *ReconcileLocalCluster {

	return &ReconcileLocalCluster{
		clientHolder:   clientHolder,
		restMapper:     restMapper,
		informerHolder: informerHolder,
		recorder:       recorder,
		mcRecorder:     mcRecorder,
		importHelper: helpers.NewImportHelper(informerHolder, recorder, log).WithGenerateClientHolderFunc(
			func(secret *v1.Secret) (reconcile.Result, *helpers.ClientHolder, meta.RESTMapper, error) {
				return reconcile.Result{}, clientHolder, restMapper, nil
			},
		),
	}
}

// blank assignment to verify that ReconcileLocalCluster implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileLocalCluster{}

// Reconcile reconciles the import secret to of a self managed cluster to import the managed cluster
// A a self managed cluster must have self managed label and the label value must be true
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileLocalCluster) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	if selfManaged, ok := managedCluster.Labels[constants.SelfManagedLabel]; !ok ||
		!strings.EqualFold(selfManaged, "true") {
		return reconcile.Result{}, nil
	}

	if _, autoImportDisabled := managedCluster.Annotations[apiconstants.DisableAutoImportAnnotation]; autoImportDisabled {
		// skip if auto import is disabled
		return reconcile.Result{}, nil
	}

	reqLogger.Info("Reconciling self managed cluster")

	// if there is an auto import secret in the managed cluster namespace, we will use the auto import secret to import
	// the cluster
	_, err = r.informerHolder.AutoImportSecretLister.Secrets(request.Name).Get(constants.AutoImportSecretName)
	if err == nil {
		return reconcile.Result{}, nil
	}
	if !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	result, condition, modified, _, iErr := r.importHelper.Import(false, managedCluster, nil, 0, 1)
	// if resources are applied but NOT modified, will not update the condition, keep the original condition.
	// This check is to prevent the current controller and import status controller from modifying the
	// ManagedClusterImportSucceeded condition of the managed cluster in a loop
	if !helpers.ImportingResourcesApplied(&condition) || modified {
		if err := helpers.UpdateManagedClusterImportCondition(
			r.clientHolder.RuntimeClient,
			managedCluster,
			condition,
			r.mcRecorder,
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	return result, iErr
}
