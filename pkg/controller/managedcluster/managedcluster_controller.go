// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const clusterNameLabel = "name"

const ClusterLabel = "cluster.open-cluster-management.io/managedCluster"

const (
	createdViaOther = "other"
)

var log = logf.Log.WithName(controllerName)

// ReconcileManagedCluster reconciles a ManagedCluster object
type ReconcileManagedCluster struct {
	client   client.Client
	recorder events.Recorder
}

// blank assignment to verify that ReconcileManagedCluster implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileManagedCluster{}

// Reconcile the ManagedCluster.
//   - When a new managed cluster is created, we will add the required meta data to the managed cluster
//   - When a managed cluster is deleting, we will wait the other components to delete their finalizers, after
//     there is only the import finalizer on managed cluster, we will delete the managed cluster namespace.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileManagedCluster) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Reconciling the managed cluster meta object")

	if managedCluster.DeletionTimestamp.IsZero() {
		if err := r.ensureManagedClusterMetaObj(ctx, managedCluster); err != nil {
			return reconcile.Result{}, err
		}

		// set cluster label on the managed cluster namespace
		ns := &corev1.Namespace{}
		err := r.client.Get(ctx, types.NamespacedName{Name: managedCluster.Name}, ns)
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		if err != nil {
			return reconcile.Result{}, err
		}

		modified := resourcemerge.BoolPtr(false)
		resourcemerge.MergeMap(modified, &ns.Labels, map[string]string{ClusterLabel: managedCluster.Name})

		if !*modified {
			return reconcile.Result{}, nil
		}

		if err := r.client.Update(ctx, ns); err != nil {
			return reconcile.Result{}, err
		}

		r.recorder.Eventf("ManagedClusterNamespaceLabelUpdated",
			"The managed cluster %s namespace label is added", managedCluster.Name)
		return reconcile.Result{}, nil
	}

	if len(managedCluster.Finalizers) > 1 {
		// managed cluster is deleting, but other components finalizers are remaining,
		// wait for other components to remove their finalizers
		return reconcile.Result{}, nil
	}

	if len(managedCluster.Finalizers) == 0 || managedCluster.Finalizers[0] != constants.ImportFinalizer {
		return reconcile.Result{}, nil
	}

	// managed cluster is deleting, remove all addons
	if err = r.deleteManagedClusterAddon(ctx, managedCluster); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, helpers.RemoveManagedClusterFinalizer(ctx, r.client, r.recorder, managedCluster, constants.ImportFinalizer)
}

func (r *ReconcileManagedCluster) ensureManagedClusterMetaObj(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	patch := client.MergeFrom(managedCluster.DeepCopy())
	modified := resourcemerge.BoolPtr(false)
	msgs := []string{}

	// ensure the cluster name label, the value of this label should be cluster name
	resourcemerge.MergeMap(modified, &managedCluster.Labels, map[string]string{clusterNameLabel: managedCluster.Name})
	if *modified {
		msgs = append(msgs, "cluster name label is added")
	}

	// ensure cluster create-via annotation
	ensureCreateViaAnnotation(modified, managedCluster)
	if *modified {
		msgs = append(msgs, "created-via annotaion is added")
	}

	// ensure cluster import finalizer
	helpers.AddManagedClusterFinalizer(modified, managedCluster, constants.ImportFinalizer)
	if *modified {
		msgs = append(msgs, "import finalizer is added")
	}

	if !*modified {
		// no changed, return
		return nil
	}

	// using patch method to avoid error: "the object has been modified; please apply your changes to the
	// latest version and try again", see:
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1509
	// https://github.com/kubernetes-sigs/controller-runtime/issues/741
	if err := r.client.Patch(ctx, managedCluster, patch); err != nil {
		return err
	}
	r.recorder.Eventf("ManagedClusterMetaObjModified", "The managed cluster %s meta data is modified: %s",
		managedCluster.Name, strings.Join(msgs, ","))
	return nil
}

func (r *ReconcileManagedCluster) deleteManagedClusterAddon(
	ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	clusterName := managedCluster.Name
	ns := &corev1.Namespace{}
	err := r.client.Get(ctx, types.NamespacedName{Name: clusterName}, ns)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// force delete addons before delete cluster namespace in this case the addon is in deleting with finalizer.
	// otherwise, the deleting addon may prevent the cluster namespace from being deleted.
	// TODO: consider to delete this since addons should be deleted by the manifestwork controller.
	addons := &v1alpha1.ManagedClusterAddOnList{}
	if err := r.client.List(ctx, addons, client.InNamespace(clusterName)); err != nil {
		return err
	}
	for _, addon := range addons.Items {
		if err = helpers.ForceDeleteManagedClusterAddon(ctx, r.client, r.recorder, addon); err != nil {
			return err
		}
	}

	r.recorder.Eventf("ManagedClusterNamespaceDeleted", "The managed cluster %s namespace is deleted", managedCluster.Name)
	return nil
}

func ensureCreateViaAnnotation(modified *bool, cluster *clusterv1.ManagedCluster) {
	createViaOtherAnnotation := map[string]string{constants.CreatedViaAnnotation: createdViaOther}
	viaAnnotation, ok := cluster.Annotations[constants.CreatedViaAnnotation]
	if !ok {
		// no created-via annotation, set it with default annotation (other)
		resourcemerge.MergeMap(modified, &cluster.Annotations, createViaOtherAnnotation)
		return
	}

	// there is a created-via annotation and the annotation is not created by hive, we ensue that the
	// created-via annotation is default annotation
	if viaAnnotation != constants.CreatedViaAI &&
		viaAnnotation != constants.CreatedViaHive &&
		viaAnnotation != constants.CreatedViaDiscovery {
		resourcemerge.MergeMap(modified, &cluster.Annotations, createViaOtherAnnotation)
	}
}
