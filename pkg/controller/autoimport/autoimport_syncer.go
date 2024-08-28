// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openshift/library-go/pkg/operator/events"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kevents "k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type autoImportSyncer struct {
	client                client.Client
	kubeClient            kubernetes.Interface
	informerHolder        *source.InformerHolder
	recorder              events.Recorder
	mcRecorder            kevents.EventRecorder
	importHelper          *helpers.ImportHelper
	rosaKubeConfigGetters map[string]*helpers.RosaKubeConfigGetter
}

// sync imports the managed cluster via the auto import secret
func (s *autoImportSyncer) sync(ctx context.Context, managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret) (reconcile.Result, error) {
	if helpers.DetermineKlusterletMode(managedCluster) == operatorv1.InstallModeHosted {
		return reconcile.Result{}, nil
	}

	if _, autoImportDisabled := managedCluster.Annotations[apiconstants.DisableAutoImportAnnotation]; autoImportDisabled {
		// skip if auto import is disabled
		klog.InfoS("Auto import is disabled:", "clusterName", managedCluster.Name)
		return reconcile.Result{}, nil
	} else {
		klog.V(2).InfoS("Auto import is enabled:", "clusterName", managedCluster.Name)
	}

	lastRetry := 0
	totalRetry := 1
	var err error
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
				s.client,
				managedCluster,
				helpers.NewManagedClusterImportSucceededCondition(
					metav1.ConditionFalse,
					constants.ConditionReasonManagedClusterImportFailed,
					fmt.Sprintf("AutoImportSecretInvalid %s/%s; please check the value %s",
						autoImportSecret.Namespace, autoImportSecret.Name, constants.AutoImportRetryName),
				),
				s.mcRecorder,
			); err != nil {
				return reconcile.Result{}, err
			}
			// auto import secret invalid, stop retrying
			klog.Errorf("Failed to get autoImportRetry. stop retrying: cluster:%v", managedCluster.Name)
			return reconcile.Result{}, nil
		}
	}

	backupRestore := false
	if v, ok := autoImportSecret.Labels[constants.LabelAutoImportRestore]; ok && strings.EqualFold(v, "true") {
		backupRestore = true
	}

	generateClientHolderFunc, err := s.getGenerateClientHolderFuncFromAutoImportSecret(
		managedCluster.Name, autoImportSecret)
	if err != nil {
		if err := helpers.UpdateManagedClusterImportCondition(
			s.client,
			managedCluster,
			helpers.NewManagedClusterImportSucceededCondition(
				metav1.ConditionFalse,
				constants.ConditionReasonManagedClusterImportFailed,
				fmt.Sprintf("AutoImportSecretInvalid %s/%s; %s",
					autoImportSecret.Namespace, autoImportSecret.Name, err),
			),
			s.mcRecorder,
		); err != nil {
			return reconcile.Result{}, err
		}
		// auto import secret invalid, stop retrying
		klog.Errorf("Failed to generate clients, stop retrying. cluster:%v, error: %v", managedCluster.Name, err)
		return reconcile.Result{}, nil
	}

	s.importHelper = s.importHelper.WithGenerateClientHolderFunc(generateClientHolderFunc)
	result, condition, modified, currentRetry, iErr := s.importHelper.Import(
		backupRestore, managedCluster, autoImportSecret, lastRetry, totalRetry)
	// if resources are applied but NOT modified, will not update the condition, keep the original condition.
	// This check is to prevent the current controller and import status controller from modifying the
	// ManagedClusterImportSucceeded condition of the managed cluster in a loop
	if !helpers.ImportingResourcesApplied(&condition) || modified {
		if err := helpers.UpdateManagedClusterImportCondition(
			s.client,
			managedCluster,
			condition,
			s.mcRecorder,
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	if iErr != nil {
		klog.Errorf("cluster %s import failed: %v. condition:%v, currentRetry:%v, result:%v, modified:%v",
			managedCluster.Name, iErr, condition, currentRetry, result, modified)
	}

	if condition.Reason == constants.ConditionReasonManagedClusterImportFailed ||
		helpers.ImportingResourcesApplied(&condition) {
		// clean up the import user when current cluster is rosa
		if getter, ok := s.rosaKubeConfigGetters[managedCluster.Name]; ok {
			if err := getter.Cleanup(); err != nil {
				return reconcile.Result{}, err
			}
			delete(s.rosaKubeConfigGetters, managedCluster.Name)
		}

		// delete secret
		if err := helpers.DeleteAutoImportSecret(ctx, s.kubeClient, autoImportSecret, s.recorder); err != nil {
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

		if _, err := s.kubeClient.CoreV1().Secrets(managedCluster.Name).Update(
			ctx, autoImportSecret, metav1.UpdateOptions{},
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	return result, iErr
}

func (s *autoImportSyncer) getGenerateClientHolderFuncFromAutoImportSecret(
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
		getter, ok := s.rosaKubeConfigGetters[clusterName]
		if !ok {
			getter = helpers.NewRosaKubeConfigGetter()
			s.rosaKubeConfigGetters[clusterName] = getter
		}

		return func(secret *corev1.Secret) (reconcile.Result, *helpers.ClientHolder, meta.RESTMapper, error) {
			return helpers.GenerateImportClientFromRosaCluster(getter, secret)
		}, nil
	default:
		return nil, fmt.Errorf("unsupported secret type %s", secret.Type)
	}
}
