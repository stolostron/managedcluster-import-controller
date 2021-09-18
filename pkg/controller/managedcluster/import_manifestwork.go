// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

//Package managedcluster ...
package managedcluster

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const manifestWorkNamePostfix = "-klusterlet"
const manifestWorkCRDSPostfix = "-crds"

func manifestWorkNsN(managedCluster *clusterv1.ManagedCluster) (types.NamespacedName, error) {
	if managedCluster == nil {
		return types.NamespacedName{}, fmt.Errorf("managedCluster is nil")
	} else if managedCluster.Name == "" {
		return types.NamespacedName{}, fmt.Errorf("managedCluster.Name is blank")
	}
	return types.NamespacedName{
		Name:      managedCluster.Name + manifestWorkNamePostfix,
		Namespace: managedCluster.Name,
	}, nil
}

func newManifestWorks(
	managedCluster *clusterv1.ManagedCluster,
	crds []*unstructured.Unstructured,
	yamls []*unstructured.Unstructured,
) (*workv1.ManifestWork, *workv1.ManifestWork, error) {

	manifestCRDs, err := convertToManifests(crds)
	if err != nil {
		return nil, nil, err
	}

	manifestYAMLs, err := convertToManifests(yamls)
	if err != nil {
		return nil, nil, err
	}

	mwNsN, err := manifestWorkNsN(managedCluster)
	if err != nil {
		return nil, nil, err
	}

	crdsManifestWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mwNsN.Name + manifestWorkCRDSPostfix,
			Namespace: mwNsN.Namespace,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifestCRDs,
			},
		},
	}

	yamlsManifestWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mwNsN.Name,
			Namespace: mwNsN.Namespace,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifestYAMLs,
			},
		},
	}

	return crdsManifestWork, yamlsManifestWork, nil
}

func convertToManifests(us []*unstructured.Unstructured) (manifests []workv1.Manifest, err error) {
	for _, u := range us {
		d, err := u.MarshalJSON()
		if err != nil {
			return nil, err
		}
		manifest := workv1.Manifest{
			runtime.RawExtension{Raw: d},
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

// CreateManifestWorks create the manifestWork use for installing klusterlet
func createOrUpdateManifestWorks(
	client client.Client,
	scheme *runtime.Scheme,
	managedCluster *clusterv1.ManagedCluster,
	ucrds []*unstructured.Unstructured,
	uyamls []*unstructured.Unstructured,
) (*workv1.ManifestWork, *workv1.ManifestWork, error) {
	crds, yamls, err := newManifestWorks(managedCluster, ucrds, uyamls)
	if err != nil {
		return nil, nil, err
	}

	mwcrds, err := createOrUpdateManifestWork(client, scheme, managedCluster, crds)
	if err != nil {
		return nil, nil, err
	}

	mwyamls, err := createOrUpdateManifestWork(client, scheme, managedCluster, yamls)
	if err != nil {
		return nil, nil, err
	}

	return mwcrds, mwyamls, nil
}

func createOrUpdateManifestWork(
	client client.Client,
	scheme *runtime.Scheme,
	managedCluster *clusterv1.ManagedCluster,
	mw *workv1.ManifestWork,
) (*workv1.ManifestWork, error) {
	// set ownerReference to klusterletconfig
	if err := controllerutil.SetControllerReference(managedCluster, mw, scheme); err != nil {
		return nil, err
	}
	log.Info("Create/update of Import manifestWork", "name", mw.Name, "namespace", mw.Namespace)
	oldManifestWork := &workv1.ManifestWork{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, oldManifestWork)
	if err != nil {
		if errors.IsNotFound(err) {
			err := client.Create(context.TODO(), mw)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		if !reflect.DeepEqual(oldManifestWork.Spec, mw.Spec) {
			log.Info("Exist then Update of Import manifestWork", "name", mw.Name, "namespace", mw.Namespace)
			oldManifestWork.Spec = mw.Spec
			if err := client.Update(context.TODO(), oldManifestWork); err != nil {
				return nil, err
			}
		}
	}
	return mw, nil
}

func deleteKlusterletManifestWorks(
	client client.Client,
	managedCluster *clusterv1.ManagedCluster,
) error {
	mwNsN, err := manifestWorkNsN(managedCluster)
	if err != nil {
		return err
	}
	//Delete the CRD manifestWork
	errCRDs := deleteManifestWork(client, mwNsN.Name+manifestWorkCRDSPostfix, mwNsN.Namespace)
	if errCRDs != nil {
		return err
	}

	return nil
	//The manifestworks yamls should not be deleted otherwize
	//The agent is deleted before removing the finalizer.

	//Delete the YAML manifestWork
	// return deleteManifestWork(client, mwNsN.Name, mwNsN.Namespace)
}

func deleteManifestWork(client client.Client, name, namespace string) error {
	mw := &workv1.ManifestWork{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      name,
			Namespace: namespace},
		mw)
	if err == nil {
		err := client.Delete(context.TODO(), mw)
		if err != nil {
			return err
		}
	} else if !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func deleteAllOtherManifestWork(c client.Client, instance *clusterv1.ManagedCluster) error {
	mwNsN, err := manifestWorkNsN(instance)
	if err != nil {
		return err
	}

	mws := &workv1.ManifestWorkList{}
	err = c.List(context.TODO(), mws, &client.ListOptions{
		Namespace: mwNsN.Namespace,
	})

	if err != nil {
		return err
	}
	for _, mw := range mws.Items {
		if mw.GetName() == mwNsN.Name || mw.GetName() == mwNsN.Name+manifestWorkCRDSPostfix {
			continue
		}
		err := deleteManifestWork(c, mw.GetName(), mw.GetNamespace())
		if err != nil {
			return err
		}
	}
	return nil
}

func evictKlusterletManifestWorks(
	client client.Client,
	managedCluster *clusterv1.ManagedCluster,
) error {
	mwNsN, err := manifestWorkNsN(managedCluster)
	if err != nil {
		return err
	}
	//Delete the CRD manifestWork
	errCRDs := evictManifestWork(client, mwNsN.Name+manifestWorkCRDSPostfix, mwNsN.Namespace)
	if errCRDs != nil {
		return errCRDs
	}

	//Delete the YAML manifestWork
	return evictManifestWork(client, mwNsN.Name, mwNsN.Namespace)
}

func evictManifestWork(client client.Client, name, namespace string) error {
	mw := &workv1.ManifestWork{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      name,
			Namespace: namespace},
		mw)
	if err == nil {
		if len(mw.Finalizers) > 0 {
			mw.SetFinalizers([]string{})
			err := client.Update(context.TODO(), mw)
			if err != nil {
				return err
			}
		}
	} else if !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func evictAllOtherManifestWork(c client.Client, instance *clusterv1.ManagedCluster) error {
	mwNsN, err := manifestWorkNsN(instance)
	if err != nil {
		return err
	}

	mws := &workv1.ManifestWorkList{}
	err = c.List(context.TODO(), mws, &client.ListOptions{
		Namespace: mwNsN.Namespace,
	})
	if err != nil {
		return err
	}
	for _, mw := range mws.Items {
		if mw.GetName() == mwNsN.Name ||
			mw.GetName() == mwNsN.Name+manifestWorkCRDSPostfix {
			continue
		}
		err := evictManifestWork(c, mw.GetName(), mw.GetNamespace())
		if err != nil {
			return err
		}
	}
	return nil
}
