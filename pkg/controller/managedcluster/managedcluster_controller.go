// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"

	libgometav1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1"
	"github.com/open-cluster-management/library-go/pkg/applier"
	"github.com/open-cluster-management/library-go/pkg/templateprocessor"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/bindata"
)

// constants for delete work and finalizer
const (
	managedClusterFinalizer = "managedcluster-import-controller.open-cluster-management.io/cleanup"
	registrationFinalizer   = "cluster.open-cluster-management.io/api-resource-cleanup"
)

const clusterLabel = "cluster.open-cluster-management.io/managedCluster"
const selfManagedLabel = "local-cluster"

var log = logf.Log.WithName("controller_managedcluster")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// customClient will do get secret without cache, other operations are like normal cache client
type customClient struct {
	client.Client
	APIReader client.Reader
}

// newCustomClient creates custom client to do get secret without cache
func newCustomClient(client client.Client, apiReader client.Reader) client.Client {
	return customClient{
		Client:    client,
		APIReader: apiReader,
	}
}

func (cc customClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	if _, ok := obj.(*corev1.Secret); ok {
		return cc.APIReader.Get(ctx, key, obj)
	}
	return cc.Client.Get(ctx, key, obj)
}

func newManifestWorkSpecPredicate() predicate.Predicate {
	return predicate.Predicate(predicate.Funcs{
		GenericFunc: func(e event.GenericEvent) bool { return false },
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.MetaOld == nil {
				log.Error(nil, "Update event has no old metadata", "event", e)
				return false
			}
			if e.ObjectOld == nil {
				log.Error(nil, "Update event has no old runtime object to update", "event", e)
				return false
			}
			if e.ObjectNew == nil {
				log.Error(nil, "Update event has no new runtime object for update", "event", e)
				return false
			}
			if e.MetaNew == nil {
				log.Error(nil, "Update event has no new metadata", "event", e)
				return false
			}
			newManifestWork, okNew := e.ObjectNew.(*workv1.ManifestWork)
			oldManifestWork, okOld := e.ObjectOld.(*workv1.ManifestWork)
			if okNew && okOld {
				return !reflect.DeepEqual(newManifestWork.Spec, oldManifestWork.Spec)
			}
			return false
		},
	})
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

	if err := r.client.Get(
		context.TODO(),
		types.NamespacedName{Namespace: "", Name: request.Name},
		instance,
	); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info(fmt.Sprintf("deleteNamespace: %s", request.Name))
			err = r.deleteNamespace(request.Name)
			if err != nil {
				reqLogger.Error(err, "Failed to delete namespace")
				return reconcile.Result{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
			}

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return r.managedClusterDeletion(instance)
	}

	reqLogger.Info(fmt.Sprintf("AddFinalizer to instance: %s", instance.Name))
	libgometav1.AddFinalizer(instance, managedClusterFinalizer)

	instanceLabels := instance.GetLabels()
	if instanceLabels == nil {
		instanceLabels = make(map[string]string)
	}

	if _, ok := instanceLabels["name"]; !ok {
		instanceLabels["name"] = instance.Name
		instance.SetLabels(instanceLabels)
	}

	if err := r.client.Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	//Add clusterLabel on ns if missing
	ns := &corev1.Namespace{}
	if err := r.client.Get(
		context.TODO(),
		types.NamespacedName{Namespace: "", Name: instance.Name},
		ns); err != nil {
		return reconcile.Result{}, err
	}

	labels := ns.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	if _, ok := labels[clusterLabel]; !ok {
		labels[clusterLabel] = instance.Name
		ns.SetLabels(labels)
		if err := r.client.Update(context.TODO(), ns); err != nil {
			return reconcile.Result{}, err
		}
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

	a, err := applier.NewApplier(
		bindata.NewBindataReader(),
		nil,
		r.client,
		instance,
		r.scheme,
		applier.DefaultKubernetesMerger,
		nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	sa := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      instance.Name + bootstrapServiceAccountNamePostfix,
			Namespace: instance.Name,
		},
		sa); err != nil && errors.IsNotFound(err) {
		reqLogger.Info(
			fmt.Sprintf("Create hub/managedcluster/manifests/managedcluster-service-account.yaml: %s",
				instance.Name))
		err = a.CreateResource(
			"hub/managedcluster/manifests/managedcluster-service-account.yaml",
			config,
		)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info(fmt.Sprintf("CreateOrUpdateInPath hub/managedcluster/manifests except sa: %s", instance.Name))
	err = a.CreateOrUpdateInPath(
		"hub/managedcluster/manifests",
		[]string{"hub/managedcluster/manifests/managedcluster-service-account.yaml"},
		false,
		config,
	)

	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("createOrUpdateImportSecret: %s", instance.Name))
	_, err = createOrUpdateImportSecret(r.client, r.scheme, instance)
	if err != nil {
		reqLogger.Error(err, "create ManagedCluster Import Secret")
		return reconcile.Result{}, err
	}

	//If self managed then apply the crds/yamls
	if v, ok := instance.GetLabels()[selfManagedLabel]; ok {
		managed, err := strconv.ParseBool(v)
		if err != nil {
			managed = false
		}
		if managed {
			if err := r.selfManaged(instance); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
			}
		}
	}

	clusterDeployment := &hivev1.ClusterDeployment{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Name},
		clusterDeployment)
	if err == nil {
		reqLogger.Info(fmt.Sprintf("createOrUpdateSyncSets: %s", instance.Name))
		_, _, err := createOrUpdateSyncSets(r.client, r.scheme, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if !checkOffLine(instance) {
			reqLogger.Info(fmt.Sprintf("createOrUpdateManifestWorks: %s", instance.Name))
			_, _, err = createOrUpdateManifestWorks(r.client, r.scheme, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileManagedCluster) managedClusterDeletion(instance *clusterv1.ManagedCluster) (reconcile.Result, error) {
	reqLogger := log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	reqLogger.Info(fmt.Sprintf("Instance in Terminating: %s", instance.Name))
	if len(filterFinalizers(instance, []string{managedClusterFinalizer, registrationFinalizer})) != 0 {
		return reconcile.Result{Requeue: true}, nil
	}

	offLine := checkOffLine(instance)
	reqLogger.Info(fmt.Sprintf("deleteAllOtherManifestWork: %s", instance.Name))
	err := deleteAllOtherManifestWork(r.client, instance)
	if err != nil {
		if !offLine {
			return reconcile.Result{}, err
		}
	}

	if offLine {
		reqLogger.Info(fmt.Sprintf("evictAllOtherManifestWork: %s", instance.Name))
		err = evictAllOtherManifestWork(r.client, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	clusterDeployment := &hivev1.ClusterDeployment{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Name},
		clusterDeployment)
	if err == nil {
		reqLogger.Info(fmt.Sprintf("deleteKlusterletSyncSets: %s", instance.Name))
		err = deleteKlusterletSyncSets(r.client, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if errors.IsNotFound(err) {
			reqLogger.Info(fmt.Sprintf("deleteKlusterletManifestWorks: %s", instance.Name))
			err = deleteKlusterletManifestWorks(r.client, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		} else {
			return reconcile.Result{}, err
		}
	}

	if !offLine {
		return reconcile.Result{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
	}

	reqLogger.Info(fmt.Sprintf("evictKlusterletManifestWorks: %s", instance.Name))
	err = evictKlusterletManifestWorks(r.client, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("Remove all finalizer: %s", instance.Name))
	instance.ObjectMeta.Finalizers = nil
	if err := r.client.Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
}

func filterFinalizers(managedCluster *clusterv1.ManagedCluster, finalizers []string) []string {
	results := make([]string, 0)
	clusterFinalizers := managedCluster.GetFinalizers()
	for _, cf := range clusterFinalizers {
		found := false
		for _, f := range finalizers {
			if cf == f {
				found = true
				break
			}
		}
		if !found {
			results = append(results, cf)
		}
	}
	return results
}

func checkOffLine(managedCluster *clusterv1.ManagedCluster) bool {
	for _, sc := range managedCluster.Status.Conditions {
		if sc.Type == clusterv1.ManagedClusterConditionAvailable {
			return sc.Status == metav1.ConditionUnknown || sc.Status == metav1.ConditionFalse
		}
	}
	return true
}

func (r *ReconcileManagedCluster) deleteNamespace(namespaceName string) error {
	ns := &corev1.Namespace{}
	err := r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name: namespaceName,
		},
		ns,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Namespace " + namespaceName + " not found")
			return nil
		}
		log.Error(err, "Failed to get namespace")
		return err
	}
	if ns.DeletionTimestamp != nil {
		log.Info("Already in deletion")
		return nil
	}

	clusterDeployment := &hivev1.ClusterDeployment{}
	err = r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      namespaceName,
			Namespace: namespaceName,
		},
		clusterDeployment,
	)
	tobeDeleted := false
	if err != nil {
		if errors.IsNotFound(err) {
			tobeDeleted = true
		} else {
			log.Error(err, "Failed to get cluster deployment")
			return err
		}
	} else {
		return fmt.Errorf(
			"can not delete namespace %s as ClusterDeployment %s still exist",
			namespaceName,
			namespaceName,
		)
	}
	if tobeDeleted {
		err = r.client.Delete(context.TODO(), ns)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete namespace")
			return err
		}
	}

	return nil
}

func (r *ReconcileManagedCluster) selfManaged(managedCluster *clusterv1.ManagedCluster) error {
	mwNSN, err := manifestWorkNsN(managedCluster)
	if err != nil {
		return err
	}
	mw := &workv1.ManifestWork{}
	err = r.client.Get(context.TODO(), mwNSN, mw)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	yamlSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{Namespace: managedCluster.Name, Name: managedCluster.Name + "-import"},
		yamlSecret)
	if err != nil {
		return err
	}
	excluded := make([]string, 0)
	sa := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      "klusterlet",
			Namespace: klusterletNamespace,
		}, sa); err == nil {
		excluded = append(excluded, "klusterlet/service_account.yaml")
	}
	crds, yamls, err := generateImportYAMLs(r.client, managedCluster, excluded)
	if err != nil {
		return err
	}

	a, err := applier.NewApplier(
		templateprocessor.NewYamlStringReader(templateprocessor.ConvertArrayOfBytesToString(crds),
			templateprocessor.KubernetesYamlsDelimiter),
		nil,
		r.client,
		nil,
		nil,
		applier.DefaultKubernetesMerger, nil)
	if err != nil {
		return err
	}

	err = a.CreateOrUpdateInPath(".", nil, false, nil)
	if err != nil {
		return err
	}

	a, err = applier.NewApplier(
		templateprocessor.NewYamlStringReader(templateprocessor.ConvertArrayOfBytesToString(yamls),
			templateprocessor.KubernetesYamlsDelimiter),
		nil,
		r.client,
		nil,
		nil,
		applier.DefaultKubernetesMerger,
		nil)
	if err != nil {
		return err
	}

	err = a.CreateOrUpdateInPath(".", excluded, false, nil)
	if err != nil {
		return err
	}
	return nil
}
