package agentregistration

import (
	"context"

	"github.com/openshift/library-go/pkg/operator/events"
	klusterletaddonconfigv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	AnnotationCreateWithDefaultKlusterletAddonConfig = "agent.open-cluster-management.io/create-with-default-klusterletaddonconfig"
)

// reconcileKlusterAddonConfig reconciles a ManagedCluster object, it will create a KlusterletAddonConfig when a ManagedCluster is created.
type reconcileKlusterAddonConfig struct {
	runtimeClient client.Client
	recorder      events.Recorder
}

var _ reconcile.Reconciler = &reconcileKlusterAddonConfig{}

func (r *reconcileKlusterAddonConfig) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	var err error

	klog.Info("Reconciling KlusterletAddonConfig", request.Name)
	managedClusterName := request.Name
	managedCluster := &clusterv1.ManagedCluster{}
	err = r.runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		klog.Infof("ManagedCluster %s is not found, do nothing", managedClusterName)
		return reconcile.Result{}, nil
	}
	if err != nil {
		klog.Errorf("Failed to get ManagedCluster %s: %v", managedClusterName, err)
		return reconcile.Result{}, err
	}

	// if the managed cluster doesn't have the label LabelCreateWithDefaultKlusterletAddonConfig, do nothing
	if _, ok := managedCluster.Annotations[AnnotationCreateWithDefaultKlusterletAddonConfig]; !ok {
		klog.Infof("ManagedCluster %s doesn't have label %s, do nothing", managedClusterName, AnnotationCreateWithDefaultKlusterletAddonConfig)
		return reconcile.Result{}, nil
	}

	// if kac is not found, create it, else do nothing
	kac := &klusterletaddonconfigv1.KlusterletAddonConfig{}
	err = r.runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName, Namespace: managedClusterName}, kac)
	if errors.IsNotFound(err) {
		klog.Infof("KlusterletAddonConfig %s is not found, creating it", managedClusterName)

		if err = r.runtimeClient.Create(ctx, &klusterletaddonconfigv1.KlusterletAddonConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      managedClusterName,
				Namespace: managedClusterName,
			},
			Spec: klusterletaddonconfigv1.KlusterletAddonConfigSpec{
				ApplicationManagerConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
					Enabled: true,
				},
				CertPolicyControllerConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
					Enabled: true,
				},
				IAMPolicyControllerConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
					Enabled: true,
				},
				PolicyController: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
					Enabled: true,
				},
				SearchCollectorConfig: klusterletaddonconfigv1.KlusterletAddonAgentConfigSpec{
					Enabled: false,
				},
			},
		}); err != nil {
			if errors.IsAlreadyExists(err) {
				klog.Infof("KlusterletAddonConfig %s is already created", managedClusterName)
				return reconcile.Result{}, nil
			}
			klog.Errorf("Failed to create KlusterletAddonConfig %s: %v", managedClusterName, err)
			return reconcile.Result{}, err
		}

		klog.Infof("KlusterletAddonConfig is created for ManagedCluster %s", managedClusterName)
		r.recorder.Eventf("KlusterletAddonConfigCreated", "KlusterletAddonConfig is created for ManagedCluster %s", managedClusterName)
	} else if err != nil {
		klog.Errorf("Failed to get KlusterletAddonConfig %s: %v", managedClusterName, err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
