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

package endpointconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

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

	multicloudv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/multicloud/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	multicloudv1alpha1 "github.com/open-cluster-management/rcm-controller/pkg/apis/multicloud/v1alpha1"
	"github.com/open-cluster-management/rcm-controller/pkg/controller/clusterregistry"
)

var log = logf.Log.WithName("controller_endpointconfig")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new EndpointConfig Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileEndpointConfig{client: mgr.GetClient(), scheme: mgr.GetScheme(), apireader: mgr.GetAPIReader()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("endpointconfig-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource EndpointConfig
	err = c.Watch(
		&source.Kind{Type: &multicloudv1alpha1.EndpointConfig{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	// Watch to enqueue request to resourceview update
	err = c.Watch(
		&source.Kind{Type: &mcmv1alpha1.ResourceView{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	err = c.Watch(
		&source.Kind{Type: &clusterregistryv1alpha1.Cluster{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: &clusterReconcileMapper{}},
	)
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner EndpointConfig
	return nil
}

// blank assignment to verify that ReconcileEndpointConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileEndpointConfig{}

// ReconcileEndpointConfig reconciles a EndpointConfig object
type ReconcileEndpointConfig struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	scheme    *runtime.Scheme
	apireader client.Reader
}

// Reconcile reads that state of the cluster for a EndpointConfig object and makes changes based on the state read
// and what is in the EndpointConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileEndpointConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling EndpointConfig")

	// Fetch the EndpointConfig instance
	instance := &multicloudv1alpha1.EndpointConfig{}

	if err := r.client.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	if instance.Spec.ClusterNamespace != instance.Namespace {
		// invalid EndpointConfig and should be prevented with ValidatingAdmissionWebhook
		return reconcile.Result{}, fmt.Errorf("invalid EndpointConfig")
	}

	cluster, err := getClusterRegistryCluster(r.client, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// when the ClusterRegistry.Cluster reconcile request for this controller will be enqueued
			// all maybe we use endpointconfig information to create the cluster?
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	if _, err := getImportSecret(r.client, instance); err != nil {
		if errors.IsNotFound(err) {
			if _, err = createImportSecret(r.client, r.scheme, instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// Update endpoint on managed cluster
	if clusterregistry.IsClusterOnline(cluster) && cluster.DeletionTimestamp == nil {
		if err := updateEndpoint(r, cluster, instance); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func updateEndpoint(r *ReconcileEndpointConfig, cluster *clusterregistryv1alpha1.Cluster, endpointconfig *multicloudv1alpha1.EndpointConfig) error {
	endpointResourceView, err := getEndpointResourceView(r.client, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			if _, err := createEndpointResourceview(r, cluster, endpointconfig); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	if !IsEndpointResourceviewCompleted(endpointResourceView) {
		return nil
	}
	endpoint, err := GetEndpoint(r, cluster, endpointResourceView)
	if err != nil {
		return err
	}

	endpointWork, err := getEndpointUpdateWork(r, endpointconfig)
	if !reflect.DeepEqual(endpoint.Spec, endpointconfig.Spec) {
		if err != nil && errors.IsNotFound(err) {
			if err := createEndpointUpdateWork(r, endpointconfig, endpoint); err != nil {
				return err
			}
			return nil
		}

		updatedEndpointwork := &multicloudv1beta1.Endpoint{}
		_ = json.Unmarshal(endpointWork.Status.Result.Raw, updatedEndpointwork)
		if reflect.DeepEqual(updatedEndpointwork.Spec, endpoint.Spec) {
			if err := deleteEndpointUpdateWork(r, endpointWork); err != nil {
				return err
			}
		}

		return nil
	}

	if endpointWork != nil {
		if err := deleteEndpointUpdateWork(r, endpointWork); err != nil {
			return err
		}
	}

	return nil
}
