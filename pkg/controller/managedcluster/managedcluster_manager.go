// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"fmt"
	"os"
	"strconv"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	_v1APIExtensionKubeMinVersion = "v1.16.0"
)

var v1APIExtensionMinVersion = version.MustParseGeneric(_v1APIExtensionKubeMinVersion)

// Add creates a new ManagedCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	client := newCustomClient(mgr.GetClient(), mgr.GetAPIReader())
	return &ReconcileManagedCluster{client: client, scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	maxConcurrentReconciles := 1
	if os.Getenv("MAX_CONCURRENT_RECONCILES") != "" {
		var err error
		maxConcurrentReconciles, err = strconv.Atoi(os.Getenv("MAX_CONCURRENT_RECONCILES"))
		log.Info(fmt.Sprintf("MAX_CONCURRENT_RECONCILES=%d", maxConcurrentReconciles))
		if err != nil {
			return err
		}
	}

	// Create a new controller
	c, err := controller.New("managedcluster-controller",
		mgr,
		controller.Options{Reconciler: r,
			MaxConcurrentReconciles: maxConcurrentReconciles,
		})
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
	err = c.Watch(
		&source.Kind{Type: &rbacv1.ClusterRole{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
	)
	if err != nil {
		log.Error(err, "Fail to add Watch for ClusterRole to controller")
		return err
	}

	err = c.Watch(
		&source.Kind{Type: &corev1.ServiceAccount{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
	)
	if err != nil {
		log.Error(err, "Fail to add Watch for ServiceAccount to controller")
		return err
	}

	err = c.Watch(
		&source.Kind{Type: &hivev1.ClusterDeployment{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      obj.Meta.GetName(),
							Namespace: obj.Meta.GetNamespace(),
						},
					},
				}
			}),
		},
	)
	if err != nil {
		log.Error(err, "Fail to add Watch for ClusterDeployment to controller")
		return err
	}

	err = c.Watch(
		&source.Kind{Type: &rbacv1.ClusterRoleBinding{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
	)
	if err != nil {
		log.Error(err, "Fail to add Watch for ClusterRoleBinding to controller")
		return err
	}

	err = c.Watch(
		&source.Kind{Type: &workv1.ManifestWork{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &clusterv1.ManagedCluster{},
		},
		newManifestWorkSpecPredicate(),
	)
	if err != nil {
		log.Error(err, "Fail to add Watch for ManifestWork to controller")
		return err
	}
	return nil
}
