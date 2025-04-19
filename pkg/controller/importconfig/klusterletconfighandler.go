package importconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
)

var _ handler.EventHandler = &enqueueManagedClusterInKlusterletConfigAnnotation{}

type enqueueManagedClusterInKlusterletConfigAnnotation struct {
	managedclusterIndexer cache.Indexer
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Create(ctx context.Context,
	evt event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Update(ctx context.Context,
	evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.ObjectNew.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Delete(ctx context.Context,
	evt event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) Generic(ctx context.Context,
	evt event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterInKlusterletConfigAnnotation) enqueue(klusterletconfigName string,
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	objs, err := e.managedclusterIndexer.ByIndex(ManagedClusterKlusterletConfigAnnotationIndexKey, klusterletconfigName)
	if err != nil {
		klog.Error(err, "Failed to get managed clusters by klusterletconfig annotation by indexer", "klusterletconfig", klusterletconfigName)
		return
	}
	for _, obj := range objs {
		mc := obj.(*clusterv1.ManagedCluster)
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name: mc.GetName(),
		}})
	}
}

const (
	ManagedClusterKlusterletConfigAnnotationIndexKey = "annotation-klusterletconfig"
)

func IndexManagedClusterByKlusterletconfigAnnotation(obj interface{}) ([]string, error) {
	managedCluster, ok := obj.(*clusterv1.ManagedCluster)
	if !ok {
		return nil, fmt.Errorf("not a managedcluster object")
	}
	klusterletconfigs := []string{constants.GlobalKlusterletConfigName}
	klusterletconfig, ok := managedCluster.GetAnnotations()[apiconstants.AnnotationKlusterletConfig]
	if ok && klusterletconfig != "" {
		klusterletconfigs = append(klusterletconfigs, klusterletconfig)
	}
	return klusterletconfigs, nil
}

const (
	KlusterletConfigBootstrapKubeConfigSecretsIndexKey = "klusterletconfig-bootstrapkubeconfig-secrets"
)

var _ handler.EventHandler = &enqueueManagedClusterByBootstrapKubeConfigSecrets{}

// enqueueManagedClusterByBootstrapKubeConfigSecrets first finds the klusterletconfigs that using the secret, then
// finds the managedclusters that using the klusterletconfigs.
type enqueueManagedClusterByBootstrapKubeConfigSecrets struct {
	// index klusterletconfig by the spec bootstrap kubeconfig secrets
	klusterletconfigIndexer cache.Indexer

	// index managedcluster by the annotation
	managedclusterIndexer cache.Indexer
}

func (e *enqueueManagedClusterByBootstrapKubeConfigSecrets) Create(ctx context.Context,
	evt event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterByBootstrapKubeConfigSecrets) Update(ctx context.Context,
	evt event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.ObjectNew.GetName(), q)
}

func (e *enqueueManagedClusterByBootstrapKubeConfigSecrets) Delete(ctx context.Context,
	evt event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterByBootstrapKubeConfigSecrets) Generic(ctx context.Context,
	evt event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(evt.Object.GetName(), q)
}

func (e *enqueueManagedClusterByBootstrapKubeConfigSecrets) enqueue(secretName string, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	klusterletconfigObjs, err := e.klusterletconfigIndexer.ByIndex(KlusterletConfigBootstrapKubeConfigSecretsIndexKey, secretName)
	if err != nil {
		klog.Error(err, "Failed to get klusterletconfigs by bootstrap kubeconfig secrets by indexer", "secret", secretName)
		return
	}
	for _, kcObj := range klusterletconfigObjs {
		kc := kcObj.(*klusterletconfigv1alpha1.KlusterletConfig)
		managedclusterObjs, err := e.managedclusterIndexer.ByIndex(
			ManagedClusterKlusterletConfigAnnotationIndexKey, kc.GetName())
		if err != nil {
			klog.Error(err, "Failed to get managedclusters by klusterletconfig annotation by indexer",
				"klusterletconfig", kc.GetName())
			return
		}
		for _, mcObj := range managedclusterObjs {
			mc := mcObj.(*clusterv1.ManagedCluster)
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name: mc.GetName(),
			}})
		}
	}
}

func IndexKlusterletConfigByBootstrapKubeConfigSecrets() func(obj interface{}) ([]string, error) {
	return func(obj interface{}) ([]string, error) {
		kc, ok := obj.(*klusterletconfigv1alpha1.KlusterletConfig)
		if !ok {
			return nil, fmt.Errorf("not a klustereltconfig object")
		}

		var bootstrapKubeConfigSecrets []string
		if kc.Spec.BootstrapKubeConfigs.Type == operatorv1.LocalSecrets &&
			kc.Spec.BootstrapKubeConfigs.LocalSecrets != nil {
			for _, secret := range kc.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets {
				bootstrapKubeConfigSecrets = append(bootstrapKubeConfigSecrets, secret.Name)
			}
		}

		return bootstrapKubeConfigSecrets, nil
	}
}

const (
	KlusterletConfigCustomizedCAConfigmapsIndexKey = "klusterletconfig-customized-ca-configmaps"
)

var _ handler.EventHandler = &enqueueManagedClusterByCustomizedCAConfigmaps{}

// enqueueManagedClusterByCustomizedCAConfigmaps first finds the klusterletconfigs that using the configmap, then
// finds the managedclusters that using the klusterletconfigs.
type enqueueManagedClusterByCustomizedCAConfigmaps struct {
	// index klusterletconfig by the customized ca configmaps
	klusterletconfigIndexer cache.Indexer

	// index managedcluster by the annotation
	managedclusterIndexer cache.Indexer
}

func (e *enqueueManagedClusterByCustomizedCAConfigmaps) Create(ctx context.Context,
	evt event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(configmapKey(evt.Object.GetNamespace(), evt.Object.GetName()), q)
}

func (e *enqueueManagedClusterByCustomizedCAConfigmaps) Update(ctx context.Context,
	evt event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(configmapKey(evt.ObjectNew.GetNamespace(), evt.ObjectNew.GetName()), q)
}

func (e *enqueueManagedClusterByCustomizedCAConfigmaps) Delete(ctx context.Context,
	evt event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(configmapKey(evt.Object.GetNamespace(), evt.Object.GetName()), q)
}

func (e *enqueueManagedClusterByCustomizedCAConfigmaps) Generic(ctx context.Context,
	evt event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.enqueue(configmapKey(evt.Object.GetNamespace(), evt.Object.GetName()), q)
}

func (e *enqueueManagedClusterByCustomizedCAConfigmaps) enqueue(
	configmapKey string, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	klusterletconfigObjs, err := e.klusterletconfigIndexer.ByIndex(
		KlusterletConfigCustomizedCAConfigmapsIndexKey, configmapKey)
	if err != nil {
		klog.Error(err, "Failed to get klusterletconfigs by customized ca configmap by indexer",
			"configmaps", configmapKey)
		return
	}
	for _, kcObj := range klusterletconfigObjs {
		kc := kcObj.(*klusterletconfigv1alpha1.KlusterletConfig)
		managedclusterObjs, err := e.managedclusterIndexer.ByIndex(
			ManagedClusterKlusterletConfigAnnotationIndexKey, kc.GetName())
		if err != nil {
			klog.Error(err, "Failed to get managedclusters by klusterletconfig annotation by indexer",
				"klusterletconfig", kc.GetName())
			return
		}
		for _, mcObj := range managedclusterObjs {
			mc := mcObj.(*clusterv1.ManagedCluster)
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name: mc.GetName(),
			}})
		}
	}
}

func IndexKlusterletConfigByCustomizedCAConfigmaps() func(obj interface{}) ([]string, error) {
	return func(obj interface{}) ([]string, error) {
		kc, ok := obj.(*klusterletconfigv1alpha1.KlusterletConfig)
		if !ok {
			return nil, fmt.Errorf("not a klustereltconfig object")
		}

		var configmaps []string
		if kc.Spec.HubKubeAPIServerConfig != nil && len(kc.Spec.HubKubeAPIServerConfig.TrustedCABundles) > 0 {
			for _, bundle := range kc.Spec.HubKubeAPIServerConfig.TrustedCABundles {
				configmaps = append(configmaps, configmapKey(bundle.CABundle.Namespace, bundle.CABundle.Name))
			}
		}

		return configmaps, nil
	}
}

func configmapKey(namespace, name string) string {
	return namespace + "/" + name
}
