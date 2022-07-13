// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusternamespacedeletion

import (
	"context"
	"strings"
	"time"

	"github.com/openshift/library-go/pkg/operator/events"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	clustercontroller "github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(controllerName)

const (
	curatorJobPrefix  string = "curator-job"
	postHookJobPrefix string = "posthookjob"
	preHookJobPrefix  string = "prehookjob"

	deletionGracePeriod = 10 * time.Minute
)

// ReconcileClusterNamespaceDeletion delete cluster namespace when
// 1. the namespace is a cluster namespace
// 2. no clusterdeployment in the ns
// 3. no infraenv in the ns
// 4. no active jobs in the ns
type ReconcileClusterNamespaceDeletion struct {
	client   client.Client
	recorder events.Recorder
}

// blank assignment to verify that ReconcileManagedCluster implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileClusterNamespaceDeletion{}

func (r *ReconcileClusterNamespaceDeletion) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling the managed cluster namespace deletion")

	ns := &corev1.Namespace{}
	err := r.client.Get(ctx, types.NamespacedName{Name: request.Name}, ns)
	if err != nil {
		// the namespace could have been deleted, do nothing
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// not interested in non-cluster namespace
	if _, ok := ns.Labels[clustercontroller.ClusterLabel]; !ok {
		return reconcile.Result{}, nil
	}
	if !ns.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	managedCluster := &clusterv1.ManagedCluster{}
	err = r.client.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	// Do nothing if the cluster is not deleting or deleted
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	if err == nil {
		if managedCluster.DeletionTimestamp.IsZero() {
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
	}

	addons := &addonv1alpha1.ManagedClusterAddOnList{}
	if err := r.client.List(ctx, addons, client.InNamespace(ns.Name)); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if len(addons.Items) > 0 {
		klog.Warningf("ManagedClusterNamespaceRemained", "There are %d addon in namespace %s", len(addons.Items), ns.Name)
		return reconcile.Result{}, nil
	}

	clusterDeploymentList := &hivev1.ClusterDeploymentList{}
	if err := r.client.List(ctx, clusterDeploymentList, client.InNamespace(ns.Name)); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if len(clusterDeploymentList.Items) != 0 {
		// there are clusterDeployments in the managed cluster namespace.
		// the managed cluster is deleted, we need to keep the managed cluster namespace.
		klog.Warningf("ManagedClusterNamespaceRemained", "There are %d clusterDeployement in namespace %s", len(clusterDeploymentList.Items), ns.Name)
		return reconcile.Result{}, nil
	}

	infraEnvList := &asv1beta1.InfraEnvList{}
	if err := r.client.List(ctx, infraEnvList, client.InNamespace(ns.Name)); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if len(infraEnvList.Items) != 0 {
		// there are infraEnvs in the managed cluster namespace.
		// the managed cluster is deleted, we need to keep the managed cluster namespace.
		klog.Warningf("ManagedClusterNamespaceRemained", "There are %d infraEnvs in namespace %s", len(infraEnvList.Items), ns.Name)
		return reconcile.Result{}, nil
	}

	pods := &corev1.PodList{}
	if err := r.client.List(ctx, pods, client.InNamespace(ns.Name)); err != nil {
		return reconcile.Result{}, err
	}
	validPods := filterPods(pods.Items, ns.Name)
	if len(validPods) > 0 {
		klog.Warningf("ManagedClusterNamespaceRemained", "There are some pods remaining in namespace %s", ns.Name)
		return reconcile.Result{RequeueAfter: deletionGracePeriod}, nil
	}

	err = r.client.Delete(ctx, ns)
	r.recorder.Eventf("ClusterNamespaceDeletion", "cluster namespace %s is deleted", ns.Name)

	return reconcile.Result{}, err
}

func filterPods(pods []corev1.Pod, namespace string) []corev1.Pod {
	validPods := []corev1.Pod{}

	for _, pod := range pods {
		// this is weird, no idea why it is needed.
		if !strings.HasPrefix(pod.Name, curatorJobPrefix) &&
			!strings.HasPrefix(pod.Name, postHookJobPrefix) &&
			!strings.HasPrefix(pod.Name, preHookJobPrefix) {
			validPods = append(validPods, pod)
		}

		// this is weird code from curator code
		if pod.Status.Phase != "" && pod.Status.Phase == "Running" {
			if !strings.Contains(pod.Name, namespace+"-uninstall") {
				validPods = append(validPods, pod)
			}
		}
	}

	return validPods
}
