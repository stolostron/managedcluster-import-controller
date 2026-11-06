// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/library-go/pkg/operator/events"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kevents "k8s.io/client-go/tools/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

var log = logf.Log.WithName(ControllerName)

// ReconcileAutoImport reconciles the managed cluster auto import secret to import the managed cluster
type ReconcileAutoImport struct {
	client                 client.Client
	kubeClient             kubernetes.Interface
	informerHolder         *source.InformerHolder
	recorder               events.Recorder
	mcRecorder             kevents.EventRecorder
	importHelper           *helpers.ImportHelper
	rosaKubeConfigGetters  map[string]*helpers.RosaKubeConfigGetter
	importControllerConfig *helpers.ImportControllerConfig
}

func NewReconcileAutoImport(
	client client.Client,
	kubeClient kubernetes.Interface,
	informerHolder *source.InformerHolder,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
	autoImportStrategyGetter *helpers.ImportControllerConfig,
) *ReconcileAutoImport {
	return &ReconcileAutoImport{
		client:                 client,
		kubeClient:             kubeClient,
		informerHolder:         informerHolder,
		recorder:               recorder,
		mcRecorder:             mcRecorder,
		importHelper:           helpers.NewImportHelper(informerHolder, recorder, log),
		rosaKubeConfigGetters:  make(map[string]*helpers.RosaKubeConfigGetter),
		importControllerConfig: autoImportStrategyGetter,
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
	reqLogger := log.WithValues("Request.Name", request.Name)

	managedClusterName := request.Name
	reqLogger.V(5).Info("Reconciling managedcluster", "managedCluster", managedClusterName)

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if helpers.DetermineKlusterletMode(managedCluster) == operatorv1.InstallModeHosted {
		return reconcile.Result{}, nil
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	if _, autoImportDisabled := managedCluster.Annotations[apiconstants.DisableAutoImportAnnotation]; autoImportDisabled {
		// skip if auto import is disabled
		reqLogger.Info("Auto import is disabled", "managedCluster", managedCluster.Name)
		return reconcile.Result{}, nil
	} else {
		reqLogger.V(5).Info("Auto import is enabled", "managedCluster", managedCluster.Name)
	}

	autoImportSecret, err := r.informerHolder.AutoImportSecretLister.Secrets(managedClusterName).Get(constants.AutoImportSecretName)
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	immediateImport := helpers.IsImmediateImport(managedCluster.Annotations)
	autoImportStrategy, err := r.importControllerConfig.GetAutoImportStrategy()
	if err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Auto import strategy is fetched", "managedCluster", managedCluster.Name, "AutoImportStrategy", autoImportStrategy)
	importSucceeded := meta.IsStatusConditionTrue(managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	retriggeredImport := false
	if !immediateImport && autoImportStrategy == apiconstants.AutoImportStrategyImportOnly && importSucceeded {
		// A managed cluster that is deleted and recreated while the old klusterlet agent is
		// still running on the spoke can get a stale import success condition: the old agent
		// reconnects to the recreated managed cluster (the agent identity is scoped to the
		// cluster name only) and completes the import handshake before the spoke is
		// reinstalled. An auto import secret created after the import success was recorded
		// is an explicit request to import the cluster again, so in that case the import
		// success is stale and must not stop the fresh cluster from being imported.
		var autoImportSecretCreationTimestamp *metav1.Time
		if autoImportSecret != nil {
			autoImportSecretCreationTimestamp = &autoImportSecret.CreationTimestamp
		}
		_, staleImportSuccess := managedCluster.Annotations[constants.AnnotationStaleImportSuccess]
		// If the managed cluster import state success is not stale and the auto import secret is created
		// after the import success, skip the import (ImportOnly strategy)
		if !staleImportSuccess && !helpers.ImportSuccessPredates(managedCluster, autoImportSecretCreationTimestamp) {
			reqLogger.Info("Auto import is skipped due to the auto import strategy",
				"managedCluster", managedCluster.Name,
				"autoImportStrategy", autoImportStrategy,
				"importSucceeded", importSucceeded,
			)
			return reconcile.Result{}, nil
		}
		retriggeredImport = true
		if !staleImportSuccess {
			// Persist that the current ImportSucceeded condition is stale. Without
			// this, the import status controller can set the ImportSucceeded condition
			// back to True from the previous incarnation's ManifestWork and refresh its
			// LastTransitionTime, which would make timestamp-based stale detection
			// fail and skip the re-import. Cleared after klusterlet resources are applied.
			if err := helpers.SetStaleImportSuccessAnnotation(ctx, r.client, managedCluster); err != nil {
				return reconcile.Result{}, err
			}
		}
		reqLogger.Info("Auto import is re-triggered since the auto import secret is created after the import success",
			"managedCluster", managedCluster.Name,
			"autoImportStrategy", autoImportStrategy,
			"importSucceeded", importSucceeded,
		)
	}

	if autoImportSecret == nil {
		// the auto import secret could have been deleted, do nothing
		reqLogger.Info("Auto import secret not found", "managedCluster", managedCluster.Name)

		// A re-triggered import may have applied resources and deleted the auto import
		// secret, but failed to remove the stale import success annotation afterwards
		// (e.g. the update conflicted): recover by removing the annotation now, so the
		// import status controller does not block the success status forever.
		if _, staleImportSuccess := managedCluster.Annotations[constants.AnnotationStaleImportSuccess]; staleImportSuccess {
			condition := meta.FindStatusCondition(
				managedCluster.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
			if condition != nil && helpers.ImportingResourcesApplied(condition) {
				if err := helpers.RemoveStaleImportSuccessAnnotation(ctx, r.client, managedCluster); err != nil {
					return reconcile.Result{}, err
				}
			}
		}
		return reconcile.Result{}, nil
	}

	backupRestore := false
	if v, ok := autoImportSecret.Labels[constants.LabelAutoImportRestore]; ok && strings.EqualFold(v, "true") {
		backupRestore = true
	}

	generateClientHolderFunc, err := r.getGenerateClientHolderFuncFromAutoImportSecret(managedClusterName, autoImportSecret)
	if err != nil {
		if err := helpers.UpdateManagedClusterImportCondition(
			r.client,
			managedCluster,
			helpers.NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImportFailed,
				fmt.Sprintf("AutoImportSecretInvalid %s/%s; %s",
					autoImportSecret.Namespace, autoImportSecret.Name, err),
			),
			r.mcRecorder,
		); err != nil {
			return reconcile.Result{}, err
		}
		// return error if the auto import secret invalid
		reqLogger.Info("Auto import secret invalid", "managedCluster", managedCluster.Name, "error", err)
		return reconcile.Result{}, err
	}

	r.importHelper = r.importHelper.WithGenerateClientHolderFunc(generateClientHolderFunc)
	result, condition, modified, iErr := r.importHelper.Import(
		backupRestore, managedCluster, autoImportSecret)
	// Skip the condition update when resources are already applied and unchanged, so
	// this controller and the import status controller do not fight over
	// ManagedClusterImportSucceeded. Re-triggered imports are an exception: always
	// write the condition so the cluster leaves the stale True state and shows the
	// current import progress (the stale-import-success annotation still keeps the
	// re-trigger active until apply finishes).
	if !helpers.ImportingResourcesApplied(&condition) || modified || retriggeredImport {
		if err := helpers.UpdateManagedClusterImportCondition(
			r.client,
			managedCluster,
			condition,
			r.mcRecorder,
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.V(5).Info("Import result", "importError", iErr, "condition", condition,
		"result", result, "modified", modified)

	if helpers.ImportingResourcesApplied(&condition) {
		// clean up the import user when current cluster is rosa
		if getter, ok := r.rosaKubeConfigGetters[managedClusterName]; ok {
			if err := getter.Cleanup(); err != nil {
				return reconcile.Result{}, err
			}

			delete(r.rosaKubeConfigGetters, managedClusterName)
		}

		// update the cluster URL before the auto secret is deleted if the importing resources are applied
		if err := updateClusterURL(ctx, r.client, managedCluster, autoImportSecret); err != nil {
			reqLogger.Error(err, "Failed to update clusterURL")
			return reconcile.Result{}, err
		}

		// delete secret
		if err := helpers.DeleteAutoImportSecret(ctx, r.kubeClient, autoImportSecret, r.recorder); err != nil {
			return reconcile.Result{}, err
		}

		// The import applied resources and the post-apply cleanup succeeded: remove the
		// stale import success annotation, so the import success reported from now on is
		// trusted again. Keep the annotation until the cleanup succeeds, otherwise the
		// import status controller can restore True from the stale ManifestWork
		// availability and the ImportOnly strategy skips the remaining work.
		if err := helpers.RemoveStaleImportSuccessAnnotation(ctx, r.client, managedCluster); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return result, iErr
}

func (r *ReconcileAutoImport) getGenerateClientHolderFuncFromAutoImportSecret(
	clusterName string, secret *corev1.Secret) (helpers.GenerateClientHolderFunc, error) {
	switch secret.Type {
	case corev1.SecretTypeOpaque:
		// for compatibility, we parse the secret felids to determine which generator should be used
		if _, hasKubeConfig := secret.Data[constants.AutoImportSecretKubeConfigKey]; hasKubeConfig {
			return helpers.GenerateImportClientFromKubeConfigSecret, nil
		}

		_, hasKubeAPIToken := secret.Data[constants.AutoImportSecretKubeTokenKey]
		_, hasKubeAPIServer := secret.Data[constants.AutoImportSecretKubeServerKey]
		if hasKubeAPIToken && hasKubeAPIServer {
			return helpers.GenerateImportClientFromKubeTokenSecret, nil
		}

		return nil, fmt.Errorf("kubeconfig or token/server pair is missing")
	case constants.AutoImportSecretKubeConfig:
		return helpers.GenerateImportClientFromKubeConfigSecret, nil
	case constants.AutoImportSecretKubeToken:
		return helpers.GenerateImportClientFromKubeTokenSecret, nil
	case constants.AutoImportSecretRosaConfig:
		getter, ok := r.rosaKubeConfigGetters[clusterName]
		if !ok {
			getter = helpers.NewRosaKubeConfigGetter()
			r.rosaKubeConfigGetters[clusterName] = getter
		}

		return func(secret *corev1.Secret) (reconcile.Result, *helpers.ClientHolder, meta.RESTMapper, error) {
			return helpers.GenerateImportClientFromRosaCluster(getter, secret)
		}, nil
	default:
		return nil, fmt.Errorf("unsupported secret type %s", secret.Type)
	}
}
