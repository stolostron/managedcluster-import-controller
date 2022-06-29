// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"embed"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
)

const bootstrapSASuffix = "bootstrap-sa"

/* #nosec */
const (
	registrationOperatorImageEnvVarName = "REGISTRATION_OPERATOR_IMAGE"
	registrationImageEnvVarName         = "REGISTRATION_IMAGE"
	workImageEnvVarName                 = "WORK_IMAGE"
	defaultImagePullSecretEnvVarName    = "DEFAULT_IMAGE_PULL_SECRET"
)

const defaultKlusterletNamespace = "open-cluster-management-agent"

const managedClusterImagePullSecretName = "open-cluster-management-image-pull-credentials"

const (
	klusterletCrdsV1File      = "manifests/klusterlet/crds/klusterlets.crd.v1.yaml"
	klusterletCrdsV1beta1File = "manifests/klusterlet/crds/klusterlets.crd.v1beta1.yaml"
)

var hubFiles = []string{
	"manifests/hub/managedcluster-service-account.yaml",
	"manifests/hub/managedcluster-clusterrole.yaml",
	"manifests/hub/managedcluster-clusterrolebinding.yaml",
}

var klusterletOperatorFiles = []string{
	"manifests/klusterlet/namespace.yaml",
	"manifests/klusterlet/service_account.yaml",
	"manifests/klusterlet/cluster_role.yaml",
	"manifests/klusterlet/clusterrole_aggregate.yaml",
	"manifests/klusterlet/cluster_role_binding.yaml",
	"manifests/klusterlet/operator.yaml",
}

var klusterletFiles = []string{
	"manifests/klusterlet/bootstrap_secret.yaml",
	"manifests/klusterlet/klusterlet.yaml",
}

var log = logf.Log.WithName(controllerName)

//go:embed manifests
var manifestFiles embed.FS

// ReconcileImportConfig reconciles a managed cluster to prepare its import secret
type ReconcileImportConfig struct {
	clientHolder *helpers.ClientHolder
	scheme       *runtime.Scheme
	recorder     events.Recorder

	workerFactory *workerFactory
}

// blank assignment to verify that ReconcileImportConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImportConfig{}

// Reconcile one managed cluster to prepare its import secret.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImportConfig) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling managed cluster import secret")

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	mode := helpers.DetermineKlusterletMode(managedCluster)
	worker, err := r.workerFactory.newWorker(mode)
	if err != nil {
		reqLogger.Info(err.Error())
		return reconcile.Result{}, nil
	}

	// make sure the managed cluster clusterrole, clusterrolebinding and bootstrap sa are updated
	config := struct {
		ManagedClusterName          string
		ManagedClusterNamespace     string
		BootstrapServiceAccountName string
	}{
		ManagedClusterName:          managedCluster.Name,
		ManagedClusterNamespace:     managedCluster.Name,
		BootstrapServiceAccountName: getBootstrapSAName(managedCluster.Name),
	}
	objects := []runtime.Object{}
	for _, file := range hubFiles {
		template, err := manifestFiles.ReadFile(file)
		if err != nil {
			// this should not happen, if happened, panic here
			panic(err)
		}

		objects = append(objects, helpers.MustCreateObjectFromTemplate(file, template, config))
	}

	if err := helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, objects...); err != nil {
		return reconcile.Result{}, err
	}

	// make sure the managed cluster import secret is updated
	importSecret, err := worker.generateImportSecret(ctx, managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, importSecret); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func klusterletNamespace(managedCluster *clusterv1.ManagedCluster) string {
	if klusterletNamespace, ok := managedCluster.Annotations[constants.KlusterletNamespaceAnnotation]; ok {
		return klusterletNamespace
	}

	return defaultKlusterletNamespace
}
