//Package clusterdeployment ...
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package clusterdeployment

import (
	"context"
	"time"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

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
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, err
	}

	// create cluster namespace if does not exist
	reqLogger.V(5).Info("getClusterRegistryNamespace")
	if _, err := getClusterRegistryNamespace(r.client, instance); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("createClusterRegistryNamespace")
			if _, err = createClusterRegistryNamespace(r.client, instance); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, err
	}

	// create cluster registry cluster if does not exist
	reqLogger.V(5).Info("getClusterRegistryCluster")
	crc, err := getClusterRegistryCluster(r.client, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("createClusterRegistryCluster")
			if _, err = createClusterRegistryCluster(r.client, instance); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, err
	}

	for _, condition := range crc.Status.Conditions {
		if condition.Type == clusterregistryv1alpha1.ClusterOK {
			//cluster already imported and online, so do nothing
			return reconcile.Result{}, nil
		}
	}

	// requeue until EndpointConfig is created for the cluster
	reqLogger.V(5).Info("getEndpointConfig")
	endpointConfig, err := getEndpointConfig(r.client, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("EndPointConfig Not found")
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}
		return reconcile.Result{}, err
	}

	// create syncset if does not exist
	reqLogger.V(5).Info("clusterimport.GetSyncSet")
	if _, err := clusterimport.GetSyncSet(r.client, endpointConfig, instance); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.V(5).Info("clusterimport.CreateSyncSet")
			if _, err := clusterimport.CreateSyncSet(r.client, endpointConfig, instance); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
