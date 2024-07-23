// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/library-go/pkg/operator/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/managedcluster-import-controller/pkg/bootstrap"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	listerklusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/listers/klusterletconfig/v1alpha1"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
)

var log = logf.Log.WithName(controllerName)

// ReconcileImportConfig reconciles a managed cluster to prepare its import secret
type ReconcileImportConfig struct {
	clientHolder           *helpers.ClientHolder
	klusterletconfigLister listerklusterletconfigv1alpha1.KlusterletConfigLister
	scheme                 *runtime.Scheme
	recorder               events.Recorder
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

	reqLogger.Info("Reconciling managed cluster")

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

	// get the previous bootstrap kubeconfig and expiration
	bootstrapKubeconfigData, expiration, err := getBootstrapKubeConfigDataFromImportSecret(ctx, r.clientHolder, managedCluster.Name, mergedKlusterletConfig)
	if err != nil {
		return reconcile.Result{}, err
	}

	// if bootstrapKubeconfig not exist or expired, create a new one
	if bootstrapKubeconfigData == nil {
		var token []byte

		token, expiration, err = bootstrap.GetBootstrapToken(ctx, r.clientHolder.KubeClient,
			bootstrap.GetBootstrapSAName(managedCluster.Name),
			managedCluster.Name, constants.DefaultSecretTokenExpirationSecond)
		if err != nil {
			return reconcile.Result{}, err
		}

		bootstrapKubeconfigData, err = bootstrap.CreateBootstrapKubeConfig(ctx, r.clientHolder,
			managedCluster.Name, token, mergedKlusterletConfig)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	var yamlcontent, crdsV1YAML, crdsV1beta1YAML []byte
	var secretAnnotations map[string]string
	switch mode {
	case operatorv1.InstallModeDefault, operatorv1.InstallModeSingleton:
		supportPriorityClass, err := helpers.SupportPriorityClass(managedCluster)
		if err != nil {
			return reconcile.Result{}, err
		}
		var priorityClassName string
		if supportPriorityClass {
			priorityClassName = constants.DefaultKlusterletPriorityClassName
		}
		config := bootstrap.NewKlusterletManifestsConfig(
			mode,
			managedCluster.Name,
			bootstrapKubeconfigData).
			WithManagedClusterAnnotations(managedCluster.GetAnnotations()).
			WithKlusterletConfig(mergedKlusterletConfig).
			WithPriorityClassName(priorityClassName)
		yamlcontent, err = config.Generate(ctx, r.clientHolder)
		if err != nil {
			return reconcile.Result{}, err
		}

		crdsV1beta1YAML, err = config.GenerateKlusterletCRDsV1Beta1()
		if err != nil {
			return reconcile.Result{}, err
		}

		crdsV1YAML, err = config.GenerateKlusterletCRDsV1()
		if err != nil {
			return reconcile.Result{}, err
		}
	case operatorv1.InstallModeHosted, operatorv1.InstallModeSingletonHosted:
		yamlcontent, err = bootstrap.NewKlusterletManifestsConfig(
			mode,
			managedCluster.Name,
			bootstrapKubeconfigData).
			WithManagedClusterAnnotations(managedCluster.GetAnnotations()).
			WithImagePullSecretGenerate(false).
			// the hosting cluster should support PriorityClass API and have
			// already had the default PriorityClass
			WithPriorityClassName(constants.DefaultKlusterletPriorityClassName).
			WithKlusterletConfig(mergedKlusterletConfig).
			Generate(ctx, r.clientHolder)
		if err != nil {
			return reconcile.Result{}, err
		}

		secretAnnotations = map[string]string{
			constants.KlusterletDeployModeAnnotation: string(operatorv1.InstallModeHosted),
		}
	default:
		return reconcile.Result{}, fmt.Errorf("klusterlet deploy mode %s not supportted", mode)
	}

	// generate import secret
	importSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.ImportSecretNameSuffix),
			Namespace: managedCluster.Name,
			Labels: map[string]string{
				constants.ClusterImportSecretLabel: "",
			},
			Annotations: secretAnnotations,
		},
		Data: map[string][]byte{
			constants.ImportSecretImportYamlKey:      yamlcontent,
			constants.ImportSecretCRDSYamlKey:        crdsV1YAML,
			constants.ImportSecretCRDSV1YamlKey:      crdsV1YAML,
			constants.ImportSecretCRDSV1beta1YamlKey: crdsV1beta1YAML,
		},
	}
	if len(expiration) != 0 {
		importSecret.Data[constants.ImportSecretTokenExpiration] = expiration
	}

	if _, err := helpers.ApplyResources(
		r.clientHolder, r.recorder, r.scheme, managedCluster, importSecret); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
