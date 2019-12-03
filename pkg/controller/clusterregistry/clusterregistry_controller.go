//Package clusterregistry contains common utility functions that gets call by many differerent packages
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
package clusterregistry

import (
	"context"
	"time"

	mcmv1alpha1 "github.ibm.com/IBMPrivateCloud/hcm-api/pkg/apis/mcm/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/clusterimport"
	"github.com/rh-ibm-synergy/multicloud-operators-cluster-controller/pkg/utils"
)

// PlatformAPIFinalizer is the constant for finalizer put on by platform-api
const PlatformAPIFinalizer = "platform-api.cluster"

// ClusterControllerFinalizer constant for finalizer put on by cluster-controller
const ClusterControllerFinalizer = "mcm-cluster-controller"

const selfDestructWorkName = "delete-multicluster-endpoint"
const selfDestructJobName = "delete-multicluster-endpoint"

const multiclusterEndpointNamespace = "multicluster-endpoint"
const multiclusterEndpointOperatorServiceAccountName = "ibm-multicluster-endpoint-operator"

var log = logf.Log.WithName("controller_clusterregistry")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Cluster Registry Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterregistry-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource clusterregistry.cluster
	err = c.Watch(&source.Kind{Type: &clusterregistryv1alpha1.Cluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner clusterregistry.cluster

	return nil
}

// blank assignment to verify that ReconcileCluster implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileCluster{}

// ReconcileCluster reconciles a Cluster object
type ReconcileCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Cluster object and makes changes based on the state read
// and what is in the Cluster.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterRegistry.Cluster")

	// Fetch the Cluster instance
	instance := &clusterregistryv1alpha1.Cluster{}

	if err := r.client.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	if isDeleting(instance) {
		if isOffline(instance) {
			// if offline remove finalizer
			instance.Finalizers = utils.RemoveFromStringSlice(instance.Finalizers, PlatformAPIFinalizer)
			instance.Finalizers = utils.RemoveFromStringSlice(instance.Finalizers, ClusterControllerFinalizer)

			// update instance
			if err := r.client.Update(context.TODO(), instance); err != nil {
				if !errors.IsConflict(err) {
					log.Error(err, "Fail to UPDATE ClusterRegistry.Cluster, requeueing")
					return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
				}

				return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
			}

			return reconcile.Result{}, nil
		}

		//create work if DNE
		if getSelfDestructWork(r.client, instance) == nil {
			newWork := newSelfDestructWork(r.client, instance)

			if err := controllerutil.SetControllerReference(instance, newWork, r.scheme); err != nil {
				log.Error(err, "Unable to SetControllerReference, requeueing")
				return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
			}

			if err := r.client.Create(context.TODO(), newWork); err != nil {
				log.Error(err, "Fail to create self destruct work, requeueing")
				return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
			}
		}

		//requeue to wait for self destruct to complete
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}

	finalizerExist := false

	for _, finalizer := range instance.Finalizers {
		if finalizer == ClusterControllerFinalizer {
			finalizerExist = true
		}
	}

	if !finalizerExist {
		instance.Finalizers = append(instance.Finalizers, ClusterControllerFinalizer)

		if err := r.client.Update(context.TODO(), instance); err != nil {
			if !errors.IsConflict(err) {
				log.Error(err, "Fail to UPDATE ClusterRegistry.Cluster, requeueing")
				return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
			}

			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
		}
	}

	return reconcile.Result{}, nil
}

func isDeleting(instance *clusterregistryv1alpha1.Cluster) bool {
	return instance.GetDeletionTimestamp() != nil
}

func isOffline(instance *clusterregistryv1alpha1.Cluster) bool {
	conds := instance.Status.Conditions
	for _, condition := range conds {
		if condition.Type == clusterregistryv1alpha1.ClusterOK {
			return false
		}
	}

	return true
}

func getSelfDestructWork(client client.Client, instance *clusterregistryv1alpha1.Cluster) *mcmv1alpha1.Work {
	workNamespacedName := types.NamespacedName{
		Name:      selfDestructWorkName,
		Namespace: instance.Namespace,
	}

	foundWork := &mcmv1alpha1.Work{}

	if err := client.Get(context.TODO(), workNamespacedName, foundWork); err != nil {
		return nil
	}

	return foundWork
}

func newSelfDestructWork(client client.Client, instance *clusterregistryv1alpha1.Cluster) *mcmv1alpha1.Work {
	return &mcmv1alpha1.Work{
		ObjectMeta: metav1.ObjectMeta{
			Name:      selfDestructWorkName,
			Namespace: instance.Namespace,
		},
		Spec: mcmv1alpha1.WorkSpec{
			Cluster: corev1.LocalObjectReference{
				Name: instance.Name,
			},
			ActionType: mcmv1alpha1.CreateActionType,
			Type:       mcmv1alpha1.ActionWorkType,
			KubeWork: &mcmv1alpha1.KubeWorkSpec{
				Resource:  "job",
				Name:      selfDestructWorkName,
				Namespace: multiclusterEndpointNamespace,
				ObjectTemplate: runtime.RawExtension{
					Object: newSelfDestructJob(client, instance),
				},
			},
		},
	}
}

func newSelfDestructJob(client client.Client, instance *clusterregistryv1alpha1.Cluster) *batchv1.Job {
	jobBackoff := int32(0) // 0 = no retries before the job fails

	clusterSecret := getClusterSecret(client, instance)
	configBytes := []byte{}

	if clusterSecret != nil {
		configBytes = clusterSecret.Data["config.yaml"]
	}

	clusterConfig := clusterimport.NewConfig(configBytes)

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: batchv1.SchemeGroupVersion.String(),
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      selfDestructJobName,
			Namespace: multiclusterEndpointNamespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &jobBackoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: multiclusterEndpointOperatorServiceAccountName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "self-destruct",
							Image:   clusterConfig.OperatorImage,
							Command: []string{"self-destruct.sh"},
						},
					},
				},
			},
		},
	}

	if clusterConfig.RegistryEnabled {
		job.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: clusterConfig.ImagePullSecret,
			},
		}
	}

	return job
}

func getClusterSecret(client client.Client, instance *clusterregistryv1alpha1.Cluster) *corev1.Secret {
	clusterSecretNamespaceName := types.NamespacedName{
		Name:      instance.Name + "-secret",
		Namespace: instance.Namespace,
	}

	foundClusterSecret := &corev1.Secret{}

	if err := client.Get(context.TODO(), clusterSecretNamespaceName, foundClusterSecret); err != nil {
		return nil
	}

	return foundClusterSecret
}
