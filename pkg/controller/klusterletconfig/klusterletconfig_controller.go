// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

package klusterletconfig

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

	klusterletv1beta1 "github.com/open-cluster-management/endpoint-operator/pkg/apis/agent/v1beta1"
	mcmv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/mcm/v1alpha1"
	klusterletcfgv1beta1 "github.com/open-cluster-management/rcm-controller/pkg/apis/agent/v1beta1"
	"github.com/open-cluster-management/rcm-controller/pkg/controller/clusterregistry"
)

var log = logf.Log.WithName("controller_klusterletconfig")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new KlusterletConfig Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileKlusterletConfig{client: mgr.GetClient(), scheme: mgr.GetScheme(), apireader: mgr.GetAPIReader()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("klusterletconfig-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KlusterletConfig
	err = c.Watch(
		&source.Kind{Type: &klusterletcfgv1beta1.KlusterletConfig{}},
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

	// Watch for changes to secondary resource Pods and requeue the owner KlusterletConfig
	return nil
}

// blank assignment to verify that ReconcileKlusterletConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKlusterletConfig{}

// ReconcileKlusterletConfig reconciles a KlusterletConfig object
type ReconcileKlusterletConfig struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	scheme    *runtime.Scheme
	apireader client.Reader
}

// Reconcile reads that state of the cluster for a KlusterletConfig object and makes changes based on the state read
// and what is in the KlusterletConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKlusterletConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KlusterletConfig")

	// Fetch the KlusterletConfig instance
	instance := &klusterletcfgv1beta1.KlusterletConfig{}

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

	// invalid KlusterletConfig and should be prevented with ValidatingAdmissionWebhook
	if instance.Spec.ClusterNamespace != instance.Namespace {
		return reconcile.Result{}, fmt.Errorf("invalid KlusterletConfig")
	}

	// preprocessing of instance.Spec these changes will not be saved into the KlusterletConfig instance

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
			// all maybe we use klusterletconfig information to create the cluster?
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
		reqLogger.V(5).Info("Updating klusterletconfig to add controller reference")
		if err := r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	klusterletWork, _ := getKlusterletUpdateWork(r, instance)
	if klusterletWork != nil {
		if klusterletWork.Status.Type == mcmv1alpha1.WorkFailed || klusterletWork.Status.Type == mcmv1alpha1.WorkCompleted {
			if err := deleteKlusterletUpdateWork(r, klusterletWork); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Update klusterlet on managed cluster
	if clusterregistry.IsClusterOnline(cluster) && cluster.DeletionTimestamp == nil {
		klusterletResourceView, err := getKlusterletResourceView(r.client, cluster)
		if err != nil {
			if errors.IsNotFound(err) {
				reqLogger.V(5).Info("Creating resourceview to fetch klusterlet for cluster ")
				if _, err := createKlusterletResourceview(r, cluster, instance); err != nil {
					return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
				}
				return reconcile.Result{}, nil
			}
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		if !isKlusterletResourceviewProcessing(klusterletResourceView) {
			return reconcile.Result{}, nil
		}

		klusterlet, err := getKlusterletFromResourceView(r, cluster, klusterletResourceView)
		if err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		if err := compareAndUpdateKlusterlet(r, instance, klusterlet); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func compareAndUpdateKlusterlet(r *ReconcileKlusterletConfig,
	klusterletConfig *klusterletcfgv1beta1.KlusterletConfig,
	klusterlet *klusterletv1beta1.Klusterlet) error {

	//Bootstrapconfig and cluster labels should not get updated
	klusterletConfig.Spec.ClusterLabels = klusterlet.Spec.ClusterLabels
	klusterletConfig.Spec.BootStrapConfig = klusterlet.Spec.BootStrapConfig
	klusterletConfig.Spec.ClusterName = klusterlet.Spec.ClusterName
	klusterletConfig.Spec.ClusterNamespace = klusterlet.Spec.ClusterNamespace

	// Create work if klusterletconfig is not same as klusterlet
	if !reflect.DeepEqual(klusterlet.Spec, klusterletConfig.Spec) {
		work, err := getKlusterletUpdateWork(r, klusterletConfig)
		if err == nil && work != nil {
			if work.Status.Type == mcmv1alpha1.WorkCompleted || work.Status.Type == mcmv1alpha1.WorkFailed {
				if err := deleteKlusterletUpdateWork(r, work); err != nil {
					return err
				}
				return nil
			}
		}

		if err != nil && errors.IsNotFound(err) {
			if err := createKlusterletUpdateWork(r, klusterletConfig, klusterlet); err != nil {
				return err
			}
			return nil
		}
		return nil
	}
	return nil
}
