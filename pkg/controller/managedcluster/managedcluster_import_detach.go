// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/open-cluster-management/library-go/pkg/applier"
	"github.com/open-cluster-management/library-go/pkg/templateprocessor"
)

func (r *ReconcileManagedCluster) importCluster(
	clusterDeployment *hivev1.ClusterDeployment,
	managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret) (reconcile.Result, error) {
	var client client.Client
	var err error
	// if clusterDeployment != nil {
	// 	// This code is not currently used as we use syncset for the time being.
	// 	if !clusterDeployment.Spec.Installed {
	// 		return reconcile.Result{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
	// 	}
	// 	klog.Infof("Use hive client to import cluster %s", managedCluster.Name)
	// 	client, err = r.getManagedClusterClientFromHive(clusterDeployment, managedCluster)
	// 	if err != nil {
	// 		return reconcile.Result{}, err
	// 	}
	// } else
	if autoImportSecret != nil {
		klog.Infof("Use autoImportSecret to import cluster %s", managedCluster.Name)
		client, err = r.getManagedClusterClientFromAutoImportSecret(managedCluster, autoImportSecret)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		klog.Infof("Use local client to import cluster %s", managedCluster.Name)
		client = r.client
	}
	return r.importClusterWithClient(clusterDeployment, managedCluster, autoImportSecret, client)
}

//get the client from hive clusterDeployment credentials secret
// func (r *ReconcileManagedCluster) getManagedClusterClientFromHive(
// 	clusterDeployment *hivev1.ClusterDeployment,
// 	managedCluster *clusterv1.ManagedCluster) (client.Client, error) {
// 	managedClusterKubeSecret := &corev1.Secret{}
// 	err := r.client.Get(context.TODO(), types.NamespacedName{
// 		Name:      clusterDeployment.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name,
// 		Namespace: managedCluster.Name,
// 	},
// 		managedClusterKubeSecret)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return getClientFromKubeConfig(managedClusterKubeSecret.Data["kubeconfig"])

// }

//Get the client from the auto-import-secret
func (r *ReconcileManagedCluster) getManagedClusterClientFromAutoImportSecret(
	managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret) (client.Client, error) {
	//generate client using kubeconfig
	if k, ok := autoImportSecret.Data["kubeconfig"]; ok {
		return getClientFromKubeConfig(k)
	} else {
		token, tok := autoImportSecret.Data["token"]
		server, sok := autoImportSecret.Data["server"]
		if tok && sok {
			return getClientFromToken(string(token), string(server))
		}
	}
	return nil, fmt.Errorf("kubeconfig or token and server are missing")
}

//Create client from kubeconfig
func getClientFromKubeConfig(kubeconfig []byte) (client.Client, error) {
	config, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return nil, err
	}

	rconfig, err := clientcmd.NewDefaultClientConfig(
		*config,
		&clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, err
	}

	client, err := client.New(rconfig, client.Options{})
	if err != nil {
		return nil, err
	}

	return client, nil
}

//Create client from token and server
func getClientFromToken(token, server string) (client.Client, error) {
	//Create config
	config := clientcmdapi.NewConfig()
	config.Clusters["default"] = &clientcmdapi.Cluster{
		Server:                server,
		InsecureSkipTLSVerify: true,
	}
	config.AuthInfos["default"] = &clientcmdapi.AuthInfo{
		Token: token,
	}
	config.Contexts["default"] = &clientcmdapi.Context{
		Cluster:  "default",
		AuthInfo: "default",
	}
	config.CurrentContext = "default"

	clientConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	clientClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return nil, err
	}
	return clientClient, nil
}

//importCluster import a cluster if autoImportRetry > 0
func (r *ReconcileManagedCluster) importClusterWithClient(
	clusterDeployment *hivev1.ClusterDeployment,
	managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret,
	managedClusterClient client.Client) (reconcile.Result, error) {

	klog.Infof("Importing cluster: %s", managedCluster.Name)

	mwNSN, err := manifestWorkNsN(managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}
	mw := &workv1.ManifestWork{}
	err = r.client.Get(context.TODO(), mwNSN, mw)
	// import already done as mw already created
	if err == nil {
		return reconcile.Result{}, nil
	}
	if !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// yamlSecret := &corev1.Secret{}
	// err = r.client.Get(context.TODO(),
	// 	types.NamespacedName{Namespace: managedCluster.Name, Name: managedCluster.Name + "-import"},
	// 	yamlSecret)
	// if err != nil {
	// 	return reconcile.Result{}, err
	// }

	if autoImportSecret != nil {
		//Decrement the autoImportRetry
		autoImportRetry, err := strconv.Atoi(string(autoImportSecret.Data[autoImportRetryName]))
		if err != nil {
			return reconcile.Result{}, err
		}
		klog.Infof("Retry left to import %s: %d", managedCluster.Name, autoImportRetry)
		autoImportRetry--
		//Remove if negatif as a label can not start with "-", should start by a char
		if autoImportRetry < 0 {
			err = r.client.Delete(context.TODO(), autoImportSecret)
			if err != nil {
				return reconcile.Result{}, err
			}
			autoImportSecret = nil
		} else {
			v := []byte(strconv.Itoa(autoImportRetry))
			autoImportSecret.Data[autoImportRetryName] = v
			err := r.client.Update(context.TODO(), autoImportSecret)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	//Do not create SA if already exists
	excluded := make([]string, 0)
	sa := &corev1.ServiceAccount{}
	if err := managedClusterClient.Get(context.TODO(),
		types.NamespacedName{
			Name:      "klusterlet",
			Namespace: klusterletNamespace,
		}, sa); err == nil {
		excluded = append(excluded, "klusterlet/service_account.yaml")
	}
	//Generate crds and yamls
	crds, yamls, err := generateImportYAMLs(r.client, managedCluster, excluded)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	//Convert crds to Yaml
	bb, err := templateprocessor.ToYAMLsUnstructured(crds)
	if err != nil {
		return reconcile.Result{}, err
	}
	//create applier for crds
	a, err := applier.NewApplier(
		templateprocessor.NewYamlStringReader(templateprocessor.ConvertArrayOfBytesToString(bb),
			templateprocessor.KubernetesYamlsDelimiter),
		nil,
		managedClusterClient,
		nil,
		nil,
		applier.DefaultKubernetesMerger, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	//Create the crds resources
	err = a.CreateOrUpdateInPath(".", nil, false, nil)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	//Convert yamls to yaml
	bb, err = templateprocessor.ToYAMLsUnstructured(yamls)
	if err != nil {
		return reconcile.Result{}, err
	}
	//Create applier for yamls
	a, err = applier.NewApplier(
		templateprocessor.NewYamlStringReader(templateprocessor.ConvertArrayOfBytesToString(bb),
			templateprocessor.KubernetesYamlsDelimiter),
		nil,
		managedClusterClient,
		nil,
		nil,
		applier.DefaultKubernetesMerger,
		nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	//Create the yamls resources
	err = a.CreateOrUpdateInPath(".", excluded, false, nil)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	//Succeeded do not retry, then remove the autoImportRetryLabel
	if autoImportSecret != nil {
		if err := r.client.Delete(context.TODO(), autoImportSecret); err != nil {
			return reconcile.Result{}, err
		}
	}
	klog.Infof("Successfully imported %s", managedCluster.Name)
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
