// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifestwork

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

type manifestWorker interface {
	validateImportSecret(importSecret *corev1.Secret) error

	generateManifestWorks(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret) ([]runtime.Object, error)
	// createKlusterletManifestWork(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret) *workv1.ManifestWork
	deleteManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork) error
}

type defaultWorker struct {
	p commonProcessor
}

var _ manifestWorker = &defaultWorker{}

func (w *defaultWorker) validateImportSecret(importSecret *corev1.Secret) error {
	return helpers.ValidateImportSecret(importSecret)
}

func (w *defaultWorker) generateManifestWorks(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret) ([]runtime.Object, error) {
	return []runtime.Object{
		createKlusterletCRDsManifestWork(managedCluster, importSecret),
		// For default mode, the klusterletManifestWork contains klusterlet-operator,
		// delete the klusterletManifestWork with orphan policy, and delete klusterlet
		// CRD resource will trigger to delete the klusterlet CR and the operator.
		createKlusterletManifestWork(managedCluster, importSecret, managedCluster.Name, workv1.DeletePropagationPolicyTypeOrphan),
	}, nil
}

// deleteManifestWorks deletes manifest works when a managed cluster is deleting
// If the managed cluster is unavailable, we will force delete all manifest works
// If the managed cluster is available, we will
//   1. delete the manifest work with the postpone-delete annotation until 10 min after the cluster is deleted.
//   2. delete the manifest works that do not include klusterlet works and klusterlet addon works
//   3. delete the klusterlet manifest work, the delete option of the klusterlet manifest work
//      is orphan, so we can delete it safely
//   4. after the klusterlet manifest work is deleted, we delete the klusterlet-crds manifest work,
//      after the klusterlet-crds manifest work is deleted from the hub cluster, its klusterlet
//      crds will be deleted from the managed cluster, then the kube system will delete the klusterlet
//      cr from the managed cluster, once the klusterlet cr is deleted, the klusterlet operator will
//      clean up the klusterlet on the managed cluster
func (w *defaultWorker) deleteManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork) error {
	if len(works) == 0 {
		return nil
	}

	if isClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all manifest works
		return w.p.forceDeleteAllManifestWorks(ctx, works)
	}

	// delete works that do not include klusterlet works and klusterlet addon works, the addon works will be removed by
	// klusterlet-addon-controller, we need to wait the klusterlet-addon-controller delete them
	for _, manifestWork := range works {
		if manifestWork.GetName() == fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix) ||
			strings.HasPrefix(manifestWork.GetName(), fmt.Sprintf("%s-klusterlet-addon", manifestWork.GetNamespace())) {
			continue
		}

		annotations := manifestWork.GetAnnotations()
		if _, ok := annotations[postponeDeletionAnnotation]; ok {
			if time.Since(cluster.DeletionTimestamp.Time) < manifestWorkPostponeDeleteTime {
				continue
			}
		}
		if err := w.p.deleteManifestWork(ctx, manifestWork.Namespace, manifestWork.Name); err != nil {
			return err
		}
	}

	ignoreKlusterlet := func(clusterName string, manifestWork workv1.ManifestWork) bool {
		return manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, klusterletSuffix) ||
			manifestWork.GetName() == fmt.Sprintf("%s-%s", clusterName, klusterletCRDsSuffix)
	}
	noPending, err := w.p.noPendingManifestWorks(ctx, cluster, ignoreKlusterlet)
	if err != nil {
		return err
	}
	if !noPending {
		// still have other works, do nothing
		return nil
	}

	// only have klusterlet manifest works, delete klusterlet manifest works
	klusterletName := fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix)
	klusterletWork := &workv1.ManifestWork{}
	err = w.p.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Namespace: cluster.Name, Name: klusterletName}, klusterletWork)
	if errors.IsNotFound(err) {
		// the klusterlet work could be deleted, ensure the klusterlet crds work is deleted
		return w.p.forceDeleteManifestWork(ctx, cluster.Name, fmt.Sprintf("%s-%s", cluster.Name, klusterletCRDsSuffix))
	}
	if err != nil {
		return err
	}

	// if the manifest work is not applied, we do nothing to avoid to delete the cluster prematurely
	// Note: there is a corner case, the registration-agent is availabel, but the work-agent is unavailable,
	// this will cause that the klusterlet work cannot be deleted, we need user to handle this manually
	if !meta.IsStatusConditionTrue(klusterletWork.Status.Conditions, workv1.WorkApplied) {
		log.Info(fmt.Sprintf("delete the manifest work %s until it is applied ...", klusterletWork.Name))
		return nil
	}

	return w.p.deleteManifestWork(ctx, klusterletWork.Namespace, klusterletWork.Name)
}

type hypershiftDetachedWorker struct {
	p commonProcessor
}

var _ manifestWorker = &hypershiftDetachedWorker{}

func (w *hypershiftDetachedWorker) validateImportSecret(importSecret *corev1.Secret) error {
	return helpers.ValidateHypershiftDetachedImportSecret(importSecret)
}

func (w *hypershiftDetachedWorker) generateManifestWorks(managedCluster *clusterv1.ManagedCluster, importSecret *corev1.Secret) ([]runtime.Object, error) {
	managementCluster, err := helpers.GetManagementCluster(managedCluster)
	if err != nil {
		return nil, err
	}
	return []runtime.Object{
		// For detached mode, the klusterletManifestWork only contains a klusterlet CR
		// and a bootstrap secret, delete it in foreground.
		createKlusterletManifestWork(managedCluster, importSecret, managementCluster, workv1.DeletePropagationPolicyTypeForeground),
	}, nil
}

func (w *hypershiftDetachedWorker) deleteManifestWorks(ctx context.Context, cluster *clusterv1.ManagedCluster, works []workv1.ManifestWork) error {
	if len(works) == 0 {
		return nil
	}

	if isClusterUnavailable(cluster) {
		// the managed cluster is offline, force delete all manifest works
		err := w.p.forceDeleteAllManifestWorks(ctx, works)
		if err != nil {
			return err
		}
		return w.deleteKlusterletManifestWork(ctx, cluster)
	}

	// delete works that do not include klusterlet addon works, the addon works will be removed by
	// klusterlet-addon-controller, we need to wait the klusterlet-addon-controller delete them
	for _, manifestWork := range works {
		if strings.HasPrefix(manifestWork.GetName(), fmt.Sprintf("%s-klusterlet-addon", manifestWork.GetNamespace())) {
			continue
		}

		annotations := manifestWork.GetAnnotations()
		if _, ok := annotations[postponeDeletionAnnotation]; ok {
			if time.Since(cluster.DeletionTimestamp.Time) < manifestWorkPostponeDeleteTime {
				continue
			}
		}
		if err := w.p.deleteManifestWork(ctx, manifestWork.Namespace, manifestWork.Name); err != nil {
			return err
		}
	}

	ignoreNothing := func(_ string, _ workv1.ManifestWork) bool { return false }
	noPending, err := w.p.noPendingManifestWorks(ctx, cluster, ignoreNothing)
	if err != nil {
		return err
	}
	if !noPending {
		// still have other works, do nothing
		return nil
	}

	// no other manifest works, delete klusterlet manifest works in the management cluster namespace
	return w.deleteKlusterletManifestWork(ctx, cluster)
}

func (w *hypershiftDetachedWorker) deleteKlusterletManifestWork(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	klusterletName := fmt.Sprintf("%s-%s", cluster.Name, klusterletSuffix)
	managementCluster, err := helpers.GetManagementCluster(cluster)
	if err != nil {
		return err
	}

	return w.p.deleteManifestWork(ctx, managementCluster, klusterletName)
}
