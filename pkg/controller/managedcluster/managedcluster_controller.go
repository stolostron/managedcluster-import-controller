// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"strings"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const clusterNameLabel = "name"

const ClusterLabel = "cluster.open-cluster-management.io/managedCluster"

const (
	createdViaOther = "other"
)

var log = logf.Log.WithName(ControllerName)

// ReconcileManagedCluster reconciles a ManagedCluster object
type ReconcileManagedCluster struct {
	client     client.Client
	recorder   events.Recorder
	mcRecorder kevents.EventRecorder
}

// NewReconcileManagedCluster creates a new ReconcileManagedCluster
func NewReconcileManagedCluster(
	client client.Client,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
) *ReconcileManagedCluster {
	return &ReconcileManagedCluster{
		client:     client,
		recorder:   recorder,
		mcRecorder: mcRecorder,
	}
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

	reqLogger.V(5).Info("Reconciling the managed cluster meta object")

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}
	if err := r.ensureManagedClusterMetaObj(ctx, managedCluster); err != nil {
		return reconcile.Result{}, err
	}

	// set cluster label on the managed cluster namespace
	ns := &corev1.Namespace{}
	err = r.client.Get(ctx, types.NamespacedName{Name: managedCluster.Name}, ns)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	modified := resourcemerge.BoolPtr(false)
	// TODO: use one cluster label to filter the cluster ns.
	// in ocm we use open-cluster-management.io/cluster-name label to filter cluster ns,
	// but in acm we use cluster.open-cluster-management.io/managedCluster to filter cluster ns.
	// to make sure the cluster ns can be filtered in some cases, add the 2 labels to the cluster ns here.
	resourcemerge.MergeMap(modified, &ns.Labels, map[string]string{ClusterLabel: managedCluster.Name,
		clusterv1.ClusterNameLabelKey: managedCluster.Name})

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

func ensureCreateViaAnnotation(modified *bool, cluster *clusterv1.ManagedCluster) {
	createViaOtherAnnotation := map[string]string{constants.CreatedViaAnnotation: createdViaOther}
	viaAnnotation, ok := cluster.Annotations[constants.CreatedViaAnnotation]
	if !ok {
		// no created-via annotation, set it with default annotation (other)
		resourcemerge.MergeMap(modified, &cluster.Annotations, createViaOtherAnnotation)
		return
	}

	// Define a set of valid created-via values
	validCreatedViaValues := map[string]bool{
		constants.CreatedViaAI:         true,
		constants.CreatedViaHive:       true,
		constants.CreatedViaDiscovery:  true,
		constants.CreatedViaHypershift: true,
	}

	// If the annotation value is not in the valid set, set it to the default value (other)
	if !validCreatedViaValues[viaAnnotation] {
		resourcemerge.MergeMap(modified, &cluster.Annotations, createViaOtherAnnotation)
	}
}
