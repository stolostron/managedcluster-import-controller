// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterdeployment ...
package clusterdeployment

import (
	"context"
	"os"
	"time"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
	"github.com/open-cluster-management/rcm-controller/pkg/clusterimport"
)

var log = logf.Log.WithName("controller_clusterdeployment")

// Add creates a new ClusterDeployment Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeployment{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner ClusterDeployment
	err = c.Watch(
		&source.Kind{Type: &klusterletcfgv1beta1.KlusterletConfig{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      obj.Meta.GetName(),
						Namespace: obj.Meta.GetNamespace(),
					},
				},
			}
		})},
	)
	if err != nil {
		return err
	}
	return nil
}

// blank assignment to verify that ReconcileClusterDeployment implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileClusterDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterDeployment")

	// Fetch the ClusterDeployment instance
	instance := &hivev1.ClusterDeployment{}

	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	reqLogger.V(5).Info("clusterimport.GetSelectorSyncset")
	// create selector syncset if it does not exist
	if _, err := clusterimport.GetSelectorSyncset(r.client); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("clusterimport.CreateSelectorSyncset")
			if _, err = clusterimport.CreateSelectorSyncset(r.client); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
			}
		}
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	// create cluster namespace if does not exist
	reqLogger.V(5).Info("getClusterRegistryNamespace")
	if _, err := getClusterRegistryNamespace(r.client, instance); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("Cluster Namespace Not found")
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	// create cluster registry cluster if does not exist
	reqLogger.V(5).Info("getClusterRegistryCluster")
	cluster, err := getClusterRegistryCluster(r.client, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// remove labels for selectorsyncset
			if clusterimport.HasClusterManagedLabels(instance) {
				newInstance := clusterimport.RemoveClusterManagedLabels(instance)
				err := r.client.Patch(context.TODO(), newInstance, client.MergeFrom(instance))
				if err != nil {
					reqLogger.Error(err, "Failed to add labels")
				}
			}
			reqLogger.V(5).Info("Cluster Not found")
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	needUpdates := false
	newInstance := instance
	// remove labels for selectorsyncset if cluster detached
	if cluster.DeletionTimestamp != nil && clusterimport.HasClusterManagedLabels(instance) {
		newInstance = clusterimport.RemoveClusterManagedLabels(instance)
		needUpdates = true
	}
	// add labels for selectorsyncset if cluster ready to get imported
	if cluster.DeletionTimestamp == nil && !clusterimport.HasClusterManagedLabels(instance) {
		newInstance = clusterimport.AddClusterManagedLabels(instance)
		needUpdates = true
	}
	if needUpdates {
		reqLogger.V(5).Info("Update clusterdeployment for labels")
		if err := r.client.Patch(context.TODO(), newInstance, client.MergeFrom(instance)); err != nil {
			reqLogger.V(5).Info("Failed to update labels")
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}
	}

	// requeue until KlusterletConfig is created for the cluster
	reqLogger.V(5).Info("getKlusterletConfig")
	klusterletConfig, err := getKlusterletConfig(r.client, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("KlusterletConfig Not found")
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	// if clusterNamespace is not set it should be configured to klusterletConfig namespace
	if klusterletConfig.Spec.ClusterNamespace == "" {
		klusterletConfig.Spec.ClusterNamespace = klusterletConfig.Namespace
	}

	if klusterletConfig.Spec.ImagePullSecret == "" {
		klusterletConfig.Spec.ImagePullSecret = os.Getenv("DEFAULT_IMAGE_PULL_SECRET")
	}

	if klusterletConfig.Spec.ImageRegistry == "" {
		klusterletConfig.Spec.ImageRegistry = os.Getenv("DEFAULT_IMAGE_REGISTRY")
	}

	// create syncset if does not exist
	reqLogger.V(5).Info("clusterimport.GetSyncSet")
	syncSet, err := clusterimport.GetSyncSet(r.client, klusterletConfig, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("clusterimport.CreateSyncSet")
			if _, err := clusterimport.CreateSyncSet(r.client, r.scheme, cluster, klusterletConfig, instance); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
			}
		}
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	reqLogger.V(5).Info("clusterimport.UpdateSyncSet")
	if _, err := clusterimport.UpdateSyncSet(r.client, klusterletConfig, instance, syncSet); err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	return reconcile.Result{}, nil
}
