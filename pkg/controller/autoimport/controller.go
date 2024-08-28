package autoimport

import (
	"context"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	kevents "k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName(controllerName)

type importSyncer interface {
	sync(ctx context.Context, cluster *clusterv1.ManagedCluster, autoImportSecret *corev1.Secret) (reconcile.Result, error)
}

// ReconcileAutoImport reconciles the managed cluster auto import secret to import the managed cluster
type ReconcileAutoImport struct {
	client                client.Client
	kubeClient            kubernetes.Interface
	informerHolder        *source.InformerHolder
	recorder              events.Recorder
	mcRecorder            kevents.EventRecorder
	importHelper          *helpers.ImportHelper
	rosaKubeConfigGetters map[string]*helpers.RosaKubeConfigGetter
}

func NewReconcileAutoImport(
	client client.Client,
	kubeClient kubernetes.Interface,
	informerHolder *source.InformerHolder,
	recorder events.Recorder,
	mcRecorder kevents.EventRecorder,
) *ReconcileAutoImport {
	return &ReconcileAutoImport{
		client:                client,
		kubeClient:            kubeClient,
		informerHolder:        informerHolder,
		recorder:              recorder,
		mcRecorder:            mcRecorder,
		importHelper:          helpers.NewImportHelper(informerHolder, recorder, log),
		rosaKubeConfigGetters: make(map[string]*helpers.RosaKubeConfigGetter),
	}
}

// blank assignment to verify that ReconcileAutoImport implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAutoImport{}

// Reconcile the managed cluster auto import secret to import the managed cluster.
// Once the managed cluster is imported, the auto import secret will be deleted
func (r *ReconcileAutoImport) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	managedClusterName := request.Name
	klog.V(2).InfoS("Reconciling auto import controller", "managedCluster", managedClusterName)

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	autoImportSecret, err := r.informerHolder.AutoImportSecretLister.
		Secrets(managedCluster.Name).Get(constants.AutoImportSecretName)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	syncers := []importSyncer{
		&apiServerSyncer{
			client:       r.client,
			recorder:     r.recorder,
			mcRecorder:   r.mcRecorder,
			importHelper: r.importHelper,
		},
		&autoImportSyncer{
			client:                r.client,
			kubeClient:            r.kubeClient,
			informerHolder:        r.informerHolder,
			recorder:              r.recorder,
			mcRecorder:            r.mcRecorder,
			importHelper:          r.importHelper,
			rosaKubeConfigGetters: r.rosaKubeConfigGetters,
		},
	}

	var errs []error
	for _, s := range syncers {
		result, err := s.sync(ctx, managedCluster, autoImportSecret)
		if result.Requeue || result.RequeueAfter > 0 {
			return result, nil
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	return reconcile.Result{}, utilerrors.NewAggregate(errs)
}
