// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"
	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"

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

const clusterLabel = "cluster.open-cluster-management.io/managedCluster"

const (
	createdViaOther = "other"
)

const (
	curatorJobPrefix  string = "curator-job"
	postHookJobPrefix string = "posthookjob"
	preHookJobPrefix  string = "prehookjob"
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
// - When a new managed cluster is created, we will add the required meta data to the managed cluster
// - When a managed cluster is deleting, we will wait the other components to delete their finalizers, after
//   there is only the import finalizer on managed cluster, we will delete the managed cluster namespace.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileManagedCluster) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling the managed cluster meta object")

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	if errors.IsNotFound(err) {
		// the managed cluster could have been deleted, do nothing
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

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
		resourcemerge.MergeMap(modified, &ns.Labels, map[string]string{clusterLabel: managedCluster.Name})

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
		// managed cluster import finalizer is missed, this should not be happened,
		// if happened, we ask user to handle this manually
		r.recorder.Warningf("ManagedClusterImportFinalizerMissed",
			"The namespace of managed cluster %s will not be deleted due to import finalizer is missed", managedCluster.Name)
		return reconcile.Result{}, nil
	}

	// managed cluster is deleting, remove its namespace
	if err = r.deleteManagedClusterNamespace(ctx, managedCluster); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, helpers.RemoveManagedClusterFinalizer(ctx, r.client, r.recorder, managedCluster, constants.ImportFinalizer)
}

func (r *ReconcileManagedCluster) ensureManagedClusterMetaObj(ctx context.Context, managedCluster *clusterv1.ManagedCluster) error {
	patch := client.MergeFrom(managedCluster.DeepCopy())
	modified := resourcemerge.BoolPtr(false)
	msgs := []string{}

	// if there is no cluster name label ensure the cluster name label
	// TODO we should ensure only update the name label in one place
	if name, ok := managedCluster.Labels[clusterNameLabel]; !ok || name == "" {
		resourcemerge.MergeMap(modified, &managedCluster.Labels, map[string]string{clusterNameLabel: managedCluster.Name})
		if *modified {
			msgs = append(msgs, "cluster name label is added")
		}
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

func (r *ReconcileManagedCluster) deleteManagedClusterNamespace(
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
	if ns.DeletionTimestamp != nil {
		log.Info(fmt.Sprintf("namespace %s is already in deletion", clusterName))
		return nil
	}

	clusterDeployment := &hivev1.ClusterDeployment{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: clusterName, Name: clusterName}, clusterDeployment)
	if err == nil && clusterDeployment.DeletionTimestamp.IsZero() {
		// there is a clusterdeployment in the managed cluster namespace and the clusterdeployment is not in deletion
		// the managed cluster is detached, we need to keep the managed cluster namespace
		r.recorder.Warningf("ManagedClusterNamespaceRemained", "There is a clusterdeployment in namespace %s", clusterName)
		return nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	infraEnvList := &asv1beta1.InfraEnvList{}
	err = r.client.List(ctx, infraEnvList, client.InNamespace(clusterName))
	if err == nil && len(infraEnvList.Items) != 0 {
		// there are infraEnvs in the managed cluster namespace.
		// the managed cluster is deleted, we need to keep the managed cluster namespace.
		// TODO: find a proper way to hand the deletion of the managed cluster namespace.
		r.recorder.Warningf("ManagedClusterNamespaceRemained", "There are infraEnvs in namespace %s", clusterName)
		return nil
	}
	if err != nil && !errors.IsNotFound(err) && !strings.Contains(err.Error(), "no matches for kind \"InfraEnv\"") {
		return err
	}

	pods := &corev1.PodList{}
	if err := r.client.List(ctx, pods, client.InNamespace(clusterName)); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if !strings.HasPrefix(pod.Name, curatorJobPrefix) &&
			!strings.HasPrefix(pod.Name, postHookJobPrefix) &&
			!strings.HasPrefix(pod.Name, preHookJobPrefix) {
			r.recorder.Warningf("ManagedClusterNamespaceRemained", "There are non curator pods in namespace %s", clusterName)
			return nil
		}
	}

	err = r.client.Delete(ctx, ns)
	if err != nil {
		return err
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
