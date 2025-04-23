// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"fmt"

	"github.com/ghodss/yaml"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(ControllerName)

// ReconcileManifestWork reconciles the ManagedClusters of the ManifestWorks object
type ReconcileManifestWork struct {
	clientHolder   *helpers.ClientHolder
	informerHolder *source.InformerHolder
	scheme         *runtime.Scheme
	recorder       events.Recorder
	mcRecorder     kevents.EventRecorder
}

func NewReconcileManifestWork(
	clientHolder *helpers.ClientHolder,
	informerHolder *source.InformerHolder,
	scheme *runtime.Scheme,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
) *ReconcileManifestWork {
	return &ReconcileManifestWork{
		clientHolder:   clientHolder,
		informerHolder: informerHolder,
		scheme:         scheme,
		recorder:       recorder,
		mcRecorder:     mcRecorder,
	}
}

// blank assignment to verify that ReconcileManifestWork implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileManifestWork{}

// Reconcile the ManagedClusters of the ManifestWorks.
//   - When the manifester works are created in one managed cluster namespace, we will add a manifest work
//     finalizer to the managed cluster
//   - When a managed cluster is deleting, we delete the manifest works and remove the manifest work
//     finalizer from the managed cluster
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileManifestWork) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	managedClusterName := request.Name

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if helpers.IsHostedCluster(managedCluster) {
		return reconcile.Result{}, nil
	}

	reqLogger.V(5).Info("Reconciling the manifest works of the managed cluster")

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	workSelector := labels.SelectorFromSet(map[string]string{constants.KlusterletWorksLabel: "true"})
	manifestWorks, err := r.informerHolder.KlusterletWorkLister.ManifestWorks(managedClusterName).List(workSelector)
	if err != nil {
		return reconcile.Result{}, err
	}

	// after the klusterlet works are created, make sure the managed cluster has manifest work finalizer
	if err := helpers.AssertManifestWorkFinalizer(ctx, r.clientHolder.RuntimeClient, r.recorder,
		managedCluster, len(manifestWorks)); err != nil {
		return reconcile.Result{}, err
	}

	// apply klusterlet manifest works from import secret
	// Note: create the klusterlet manifest works before importing cluster to avoid the klusterlet applied manifest
	// works are deleted from managed cluster if the restored hub has same host with the backup hub in the
	// backup-restore case.
	importSecretName := fmt.Sprintf("%s-%s", managedClusterName, constants.ImportSecretNameSuffix)
	importSecret, err := r.informerHolder.ImportSecretLister.Secrets(managedClusterName).Get(importSecretName)
	if errors.IsNotFound(err) {
		reqLogger.V(5).Info("The import secret is not found", "importSecret", importSecretName)
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ValidateImportSecret(importSecret); err != nil {
		return reconcile.Result{}, err
	}

	_, err = helpers.ApplyResources(
		r.clientHolder,
		r.recorder,
		r.scheme,
		managedCluster,
		createManifestWorks(managedCluster, importSecret)...,
	)
	return reconcile.Result{}, err
}

func createManifestWorks(
	managedCluster *clusterv1.ManagedCluster,
	importSecret *corev1.Secret) []runtime.Object {
	var works []runtime.Object

	// create crd work if it contains in the secret.
	crdYaml := importSecret.Data[constants.ImportSecretCRDSYamlKey]
	if len(crdYaml) > 0 {
		jsonData, err := yaml.YAMLToJSON(crdYaml)
		if err != nil {
			panic(err)
		}

		crdWork := &workv1.ManifestWork{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.KlusterletCRDsSuffix),
				Namespace: managedCluster.Name,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
				Annotations: map[string]string{
					// make sure the crd manifestWork is the last to be deleted.
					clusterv1.CleanupPriorityAnnotationKey: "100",
				},
			},
			Spec: workv1.ManifestWorkSpec{
				Workload: workv1.ManifestsTemplate{
					Manifests: []workv1.Manifest{
						{RawExtension: runtime.RawExtension{Raw: jsonData}},
					},
				},
			},
		}

		works = append(works, crdWork)
	}

	manifests := []workv1.Manifest{}
	importYaml := importSecret.Data[constants.ImportSecretImportYamlKey]
	for _, yamlData := range helpers.SplitYamls(importYaml) {
		jsonData, err := yaml.YAMLToJSON(yamlData)
		if err != nil {
			panic(err)
		}
		manifests = append(manifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: jsonData},
		})
	}

	klwork := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.KlusterletSuffix),
			Namespace: managedCluster.Name,
			Labels: map[string]string{
				constants.KlusterletWorksLabel: "true",
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			},
		},
	}

	works = append(works, klwork)

	// if crd is not set, we only apply klusterlet only, and the deleteOption
	// should be foreground.
	if len(crdYaml) == 0 {
		klwork.SetAnnotations(map[string]string{
			// make sure the klusterlet manifestWork is the last to be deleted if there is no crd manifestWork.
			clusterv1.CleanupPriorityAnnotationKey: "100",
		})
		klwork.Spec.DeleteOption = &workv1.DeleteOption{
			PropagationPolicy: workv1.DeletePropagationPolicyTypeForeground,
		}
	}

	return works
}
