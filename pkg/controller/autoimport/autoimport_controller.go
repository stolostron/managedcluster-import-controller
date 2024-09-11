// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"fmt"
	"strconv"
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

var log = logf.Log.WithName(controllerName)

// ReconcileAutoImport reconciles the managed cluster auto import secret to import the managed cluster
type ReconcileAutoImport struct {
	client                client.Client
	kubeClient            kubernetes.Interface
	informerHolder        *source.InformerHolder
	recorder              events.Recorder
	mcRecorder            kevents.EventRecorder
	importHelper          *helpers.ImportHelper
	rosaKubeConfigGetters map[string]*helpers.RosaKubeConfigGetter
}

func NewReconcileAutoImport(
	client client.Client,
	kubeClient kubernetes.Interface,
	informerHolder *source.InformerHolder,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
) *ReconcileAutoImport {
	return &ReconcileAutoImport{
		client:                client,
		kubeClient:            kubeClient,
		informerHolder:        informerHolder,
		recorder:              recorder,
		mcRecorder:            mcRecorder,
		importHelper:          helpers.NewImportHelper(informerHolder, recorder, log),
		rosaKubeConfigGetters: make(map[string]*helpers.RosaKubeConfigGetter),
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
		reqLogger.V(5).Info("Auto import is disabled", "managedCluster", managedCluster.Name)
		return reconcile.Result{}, nil
	} else {
		reqLogger.V(5).Info("Auto import is enabled", "managedCluster", managedCluster.Name)
	}

	autoImportSecret, err := r.informerHolder.AutoImportSecretLister.Secrets(managedClusterName).Get(constants.AutoImportSecretName)
	if errors.IsNotFound(err) {
		// the auto import secret could have been deleted, do nothing
		reqLogger.V(5).Info("Auto import secret not found", "managedCluster", managedCluster.Name)
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

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
			if err := helpers.UpdateManagedClusterImportCondition(
				r.client,
				managedCluster,
				helpers.NewManagedClusterImportSucceededCondition(
					metav1.ConditionFalse,
					constants.ConditionReasonManagedClusterImportFailed,
					fmt.Sprintf("AutoImportSecretInvalid %s/%s; please check the value %s",
						autoImportSecret.Namespace, autoImportSecret.Name, constants.AutoImportRetryName),
				),
				r.mcRecorder,
			); err != nil {
				return reconcile.Result{}, err
			}
			// auto import secret invalid, stop retrying
			reqLogger.V(5).Info("Auto import secret invalid", "managedCluster", managedCluster.Name)
			return reconcile.Result{}, nil
		}
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
		// auto import secret invalid, stop retrying
		reqLogger.V(5).Info("Auto import secret invalid", "managedCluster", managedCluster.Name)
		return reconcile.Result{}, nil
	}

	r.importHelper = r.importHelper.WithGenerateClientHolderFunc(generateClientHolderFunc)
	result, condition, modified, currentRetry, iErr := r.importHelper.Import(
		backupRestore, managedCluster, autoImportSecret, lastRetry, totalRetry)
	// if resources are applied but NOT modified, will not update the condition, keep the original condition.
	// This check is to prevent the current controller and import status controller from modifying the
	// ManagedClusterImportSucceeded condition of the managed cluster in a loop
	if !helpers.ImportingResourcesApplied(&condition) || modified {
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
		"current", currentRetry, "result", result, "modified", modified)

	if condition.Reason == constants.ConditionReasonManagedClusterImportFailed ||
		helpers.ImportingResourcesApplied(&condition) {
		// clean up the import user when current cluster is rosa
		if getter, ok := r.rosaKubeConfigGetters[managedClusterName]; ok {
			if err := getter.Cleanup(); err != nil {
				return reconcile.Result{}, err
			}

			delete(r.rosaKubeConfigGetters, managedClusterName)
		}

		// update the cluster URL before the auto secret is deleted if the importing resources are applied
		if helpers.ImportingResourcesApplied(&condition) {
			if err := updateClusterURL(ctx, r.client, managedCluster, autoImportSecret); err != nil {
				reqLogger.Error(err, "Failed to update clusterURL")
				return reconcile.Result{}, err
			}
		}

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
