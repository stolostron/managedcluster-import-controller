// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	listerklusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/listers/klusterletconfig/v1alpha1"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
)

var log = logf.Log.WithName(ControllerName)

// ReconcileImportConfig reconciles a managed cluster to prepare its import secret
type ReconcileImportConfig struct {
	clientHolder           *helpers.ClientHolder
	klusterletconfigLister listerklusterletconfigv1alpha1.KlusterletConfigLister
	scheme                 *runtime.Scheme
	recorder               events.Recorder
	importControllerConfig *helpers.ImportControllerConfig
}

// blank assignment to verify that ReconcileImportConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImportConfig{}

// Reconcile one managed cluster to prepare its import secret.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImportConfig) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.V(5).Info("Reconciling managed cluster")

	mode := helpers.DetermineKlusterletMode(managedCluster)
	if err := helpers.ValidateKlusterletMode(mode); err != nil {
		reqLogger.Info(err.Error())
		return reconcile.Result{}, nil
	}

	// make sure the managed cluster clusterrole, clusterrolebinding and bootstrap sa are updated
	objects, err := bootstrap.GenerateHubBootstrapRBACObjects(managedCluster.Name)
	if err != nil {
		return reconcile.Result{}, err
	}
	if _, err := helpers.ApplyResources(
		r.clientHolder, r.recorder, r.scheme, managedCluster, objects...); err != nil {
		return reconcile.Result{}, err
	}

	klusterletconfigName := managedCluster.GetAnnotations()[apiconstants.AnnotationKlusterletConfig]

	// Get the merged KlusterletConfig, it merges the user assigned KlusterletConfig with the global KlusterletConfig.
	mergedKlusterletConfig, err := helpers.GetMergedKlusterletConfigWithGlobal(klusterletconfigName, r.klusterletconfigLister)
	if err != nil {
		return reconcile.Result{}, err
	}

	// build the bootstrap kubeconfig
	bootstrapKubeconfigData, tokenCreation, tokenExpiration, err := buildBootstrapKubeconfigData(ctx, r.clientHolder,
		managedCluster, mergedKlusterletConfig)
	if err != nil {
		return reconcile.Result{}, err
	}

	// rebuild the import secret and save it if it is modified
	importSecret, configSecret, err := buildImportSecret(ctx, r.clientHolder, managedCluster, mode, mergedKlusterletConfig,
		bootstrapKubeconfigData, tokenCreation, tokenExpiration)
	if err != nil {
		return reconcile.Result{}, err
	}

	if _, err := helpers.ApplyResources(
		r.clientHolder, r.recorder, r.scheme, managedCluster, importSecret); err != nil {
		return reconcile.Result{}, err
	}

	generateConfigSecret, err := r.importControllerConfig.GenerateImportConfig()
	if err != nil || !generateConfigSecret {
		return reconcile.Result{}, err
	}
	if _, err := helpers.ApplyResources(
		r.clientHolder, r.recorder, r.scheme, managedCluster, configSecret); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
