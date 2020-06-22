// Copyright (c) 2020 Red Hat, Inc.

package managedcluster

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"

	"github.com/open-cluster-management/library-go/pkg/applier"
	"github.com/open-cluster-management/rcm-controller/pkg/bindata"
	"github.com/open-cluster-management/rcm-controller/pkg/utils"
)

// constants for delete work and finalizer
const (
	managedClusterFinalizer = "managedcluster-import-controller.managedcluster"
)

var log = logf.Log.WithName("controller_managedcluster")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ManagedCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileManagedCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("managedcluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ManagedCluster
	err = c.Watch(
		&source.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner ManagedCluster
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRole{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &clusterv1.ManagedCluster{},
	})
	if err != nil {
		log.Error(err, "Fail to add Watch for ClusterRole to controller")
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &clusterv1.ManagedCluster{},
	})
	if err != nil {
		log.Error(err, "Fail to add Watch for ServiceAccount to controller")
		return err
	}

	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &clusterv1.ManagedCluster{},
	})
	if err != nil {
		log.Error(err, "Fail to add Watch for ClusterDeployment to controller")
		return err
	}

	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &clusterv1.ManagedCluster{},
	})
	if err != nil {
		log.Error(err, "Fail to add Watch for ClusterRoleBinding to controller")
		return err
	}

	err = c.Watch(&source.Kind{Type: &hivev1.SyncSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &clusterv1.ManagedCluster{},
	})
	if err != nil {
		log.Error(err, "Fail to add Watch for SyncSet to controller")
		return err
	}

	err = c.Watch(&source.Kind{Type: &workv1.ManifestWork{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &clusterv1.ManagedCluster{},
	})
	if err != nil {
		log.Error(err, "Fail to add Watch for ManifestWork to controller")
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileManagedCluster implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileManagedCluster{}

// ReconcileManagedCluster reconciles a ManagedCluster object
type ReconcileManagedCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

var merger applier.Merger = func(current,
	new *unstructured.Unstructured,
) (
	future *unstructured.Unstructured,
	update bool,
) {
	if spec, ok := new.Object["spec"]; ok &&
		!reflect.DeepEqual(spec, current.Object["spec"]) {
		update = true
		current.Object["spec"] = spec
	}
	if rules, ok := new.Object["rules"]; ok &&
		!reflect.DeepEqual(rules, current.Object["rules"]) {
		update = true
		current.Object["rules"] = rules
	}
	if roleRef, ok := new.Object["roleRef"]; ok &&
		!reflect.DeepEqual(roleRef, current.Object["roleRef"]) {
		update = true
		current.Object["roleRef"] = roleRef
	}
	if subjects, ok := new.Object["subjects"]; ok &&
		!reflect.DeepEqual(subjects, current.Object["subjects"]) {
		update = true
		current.Object["subjects"] = subjects
	}
	return current, update
}

// Reconcile reads that state of the cluster for a ManagedCluster object and makes changes based on the state read
// and what is in the ManagedCluster.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileManagedCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ManagedCluster")

	// Fetch the ManagedCluster instance
	instance := &clusterv1.ManagedCluster{}

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
		hasFinalizers := checkOtherFinalizers(instance)
		if hasFinalizers {
			return reconcile.Result{Requeue: true}, nil
		}
		err := deleteSyncSetAndManifestWork(r.client, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !checkOffLine(instance) {
			return reconcile.Result{Requeue: true}, nil
		}
		err = evictManifestWorks(r.client, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
		utils.RemoveFinalizer(instance, managedClusterFinalizer)
		if err := r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	utils.AddFinalizer(instance, managedClusterFinalizer)

	if err := r.client.Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	//Create the values for the yamls
	config := struct {
		ManagedClusterName          string
		ManagedClusterNamespace     string
		BootstrapServiceAccountName string
	}{
		ManagedClusterName:          instance.Name,
		ManagedClusterNamespace:     instance.Name,
		BootstrapServiceAccountName: instance.Name + bootstrapServiceAccountNamePostfix,
	}

	tp, err := applier.NewTemplateProcessor(bindata.NewBindataReader(), nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	a, err := applier.NewApplier(tp, r.client, instance, r.scheme, merger)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = a.CreateOrUpdateInPath(
		"hub/managedcluster/manifests",
		nil,
		false,
		config,
	)

	if err != nil {
		return reconcile.Result{}, err
	}

	_, err = createOrUpdateImportSecret(r.client, r.scheme, instance)
	if err != nil {
		log.Error(err, "create ManagedCluster Import Secret")
		return reconcile.Result{}, err
	}

	clusterDeployment := &hivev1.ClusterDeployment{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Name},
		clusterDeployment)
	if err == nil {
		_, _, err := createOrUpdateSyncSets(r.client, r.scheme, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if !checkOffLine(instance) {
			_, _, err = createOrUpdateManifestWorks(r.client, r.scheme, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return reconcile.Result{}, nil
}

//checkOtherFinalizer checks if other finalizers left
func checkOtherFinalizers(managedCluster *clusterv1.ManagedCluster) bool {
	finalizers := managedCluster.GetFinalizers()
	if len(finalizers) > 1 {
		return true
	}
	return len(finalizers) != 0 && finalizers[0] != managedClusterFinalizer
}

func checkOffLine(managedCluster *clusterv1.ManagedCluster) bool {
	for _, sc := range managedCluster.Status.Conditions {
		if sc.Type == clusterv1.ManagedClusterConditionAvailable {
			return sc.Status == metav1.ConditionUnknown || sc.Status == metav1.ConditionFalse
		}
	}
	return true
}

func deleteSyncSetAndManifestWork(client client.Client, instance *clusterv1.ManagedCluster) error {
	clusterDeployment := &hivev1.ClusterDeployment{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Name},
		clusterDeployment)
	if err == nil {
		err := deleteSyncSets(client, instance)
		if err != nil {
			return err
		}
	} else {
		return deleteManifestWorks(client, instance)
	}
	return nil
}
