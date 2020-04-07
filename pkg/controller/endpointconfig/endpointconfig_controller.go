// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package endpointconfig

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	err = c.Watch(
		&source.Kind{Type: &clusterregistryv1alpha1.Cluster{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: &clusterReconcileMapper{}},
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

	// invalid EndpointConfig and should be prevented with ValidatingAdmissionWebhook
	if instance.Spec.ClusterNamespace != instance.Namespace {
		return reconcile.Result{}, fmt.Errorf("invalid EndpointConfig")
	}

	// preprocessing of instance.Spec these changes will not be saved into the EndpointConfig instance

	// if clusterNamespace is not set it should be configured to instance namespace
	if instance.Spec.ClusterNamespace == "" {
		instance.Spec.ClusterNamespace = instance.Namespace
	}

	if instance.Spec.ImagePullSecret == "" {
		instance.Spec.ImagePullSecret = os.Getenv("DEFAULT_IMAGE_PULL_SECRET")
	}

	if instance.Spec.ImageRegistry == "" {
		instance.Spec.ImageRegistry = os.Getenv("DEFAULT_IMAGE_REGISTRY")
	}

	cluster, err := getClusterRegistryCluster(r.client, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// when the ClusterRegistry.Cluster reconcile request for this controller will be enqueued
			// all maybe we use endpointconfig information to create the cluster?
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		return reconcile.Result{}, err
	}

	// always update the import secret
	oldImportSecret, err := getImportSecret(r.client, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			if _, err = createImportSecret(r.client, r.scheme, cluster, instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		if _, err = updateImportSecret(r.client, instance, oldImportSecret); err != nil {
			return reconcile.Result{}, err
		}
	}
	// add controller reference if doesn't have one
	if controllerRef := metav1.GetControllerOf(instance); controllerRef == nil {
		reqLogger.V(5).Info("Setting controller reference")
		if err := controllerutil.SetControllerReference(cluster, instance, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		reqLogger.V(5).Info("Updating endpointconfig to add controller reference")
		if err := r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	endpointWork, _ := getEndpointUpdateWork(r, instance)
	if endpointWork != nil {
		if endpointWork.Status.Type == mcmv1alpha1.WorkFailed || endpointWork.Status.Type == mcmv1alpha1.WorkCompleted {
			if err := deleteEndpointUpdateWork(r, endpointWork); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Update endpoint on managed cluster
	if clusterregistry.IsClusterOnline(cluster) && cluster.DeletionTimestamp == nil {
		endpointResourceView, err := getEndpointResourceView(r.client, cluster)
		if err != nil {
			if errors.IsNotFound(err) {
				reqLogger.V(5).Info("Creating resourceview to fetch endpoint for cluster ")
				if _, err := createEndpointResourceview(r, cluster, instance); err != nil {
					return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
				}
				return reconcile.Result{}, nil
			}
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		if !isEndpointResourceviewProcessing(endpointResourceView) {
			return reconcile.Result{}, nil
		}

		endpoint, err := getEndpointFromResourceView(r, cluster, endpointResourceView)
		if err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		if err := compareAndUpdateEndpoint(r, instance, endpoint); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func compareAndUpdateEndpoint(r *ReconcileEndpointConfig, endpointc *multicloudv1alpha1.EndpointConfig, endpoint *multicloudv1beta1.Endpoint) error {
	//Bootstrapconfig and cluster labels should not get updated
	endpointc.Spec.ClusterLabels = endpoint.Spec.ClusterLabels
	endpointc.Spec.BootStrapConfig = endpoint.Spec.BootStrapConfig
	endpointc.Spec.ClusterName = endpoint.Spec.ClusterName
	endpointc.Spec.ClusterNamespace = endpoint.Spec.ClusterNamespace

	// Create work if endpoinfconfig is not same as endpoint
	if !reflect.DeepEqual(endpoint.Spec, endpointc.Spec) {
		work, err := getEndpointUpdateWork(r, endpointc)
		if err == nil && work != nil {
			if work.Status.Type == mcmv1alpha1.WorkCompleted || work.Status.Type == mcmv1alpha1.WorkFailed {
				if err := deleteEndpointUpdateWork(r, work); err != nil {
					return err
				}
				return nil
			}
		}

		if err != nil && errors.IsNotFound(err) {
			if err := createEndpointUpdateWork(r, endpointc, endpoint); err != nil {
				return err
			}
			return nil
		}
		return nil
	}
	return nil
}
