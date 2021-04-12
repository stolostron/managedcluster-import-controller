// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	libgometav1 "github.com/open-cluster-management/library-go/pkg/apis/meta/v1"
	libgoconfig "github.com/open-cluster-management/library-go/pkg/config"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/open-cluster-management/applier/pkg/applier"
	"github.com/open-cluster-management/applier/pkg/templateprocessor"
)

func (r *ReconcileManagedCluster) importCluster(
	managedCluster *clusterv1.ManagedCluster,
	clusterDeployment *hivev1.ClusterDeployment,
	autoImportSecret *corev1.Secret) (res reconcile.Result, err error) {
	res = reconcile.Result{}

	//Assuming that is a local import
	client := r.client

	rConfig, err := libgoconfig.LoadConfig("", "", "")
	if err != nil {
		return reconcile.Result{}, err
	}

	//A clusterDeployment exist then get the client
	if clusterDeployment != nil {
		klog.Infof("Use hive client to import cluster %s", managedCluster.Name)
		client, rConfig, err = r.getManagedClusterClientFromHive(clusterDeployment, managedCluster)
		if err != nil {
			return reconcile.Result{}, err
		}
		//Testing to avoid update which will generate roundtrip as the clusterDeployment is watched
		if !libgometav1.HasFinalizer(clusterDeployment, managedClusterFinalizer) {
			klog.Info("Add finalizer in clusterDeployment")
			libgometav1.AddFinalizer(clusterDeployment, managedClusterFinalizer)
			err = r.client.Update(context.TODO(), clusterDeployment)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	//Check if auto-import and get client from the importSecret
	if autoImportSecret != nil {
		klog.Infof("Use autoImportSecret to import cluster %s", managedCluster.Name)
		client, rConfig, err = r.getManagedClusterClientFromAutoImportSecret(autoImportSecret)
	}

	if err == nil {
		var managedClusterKubeVersion string
		managedClusterKubeVersion, err = getManagedClusterKubeVersion(rConfig)
		if err != nil {
			return reconcile.Result{}, err
		}
		res, err = r.importClusterWithClient(managedCluster, autoImportSecret, client, managedClusterKubeVersion)
	}
	if err != nil && autoImportSecret != nil {
		errUpdate := r.updateAutoImportRetry(managedCluster, autoImportSecret)
		if errUpdate != nil {
			return res, errUpdate
		}
	}

	return res, err

}

//get the client from hive clusterDeployment credentials secret
func (r *ReconcileManagedCluster) getManagedClusterClientFromHive(
	clusterDeployment *hivev1.ClusterDeployment,
	managedCluster *clusterv1.ManagedCluster) (client.Client, *rest.Config, error) {
	managedClusterKubeSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Name:      clusterDeployment.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name,
		Namespace: managedCluster.Name,
	},
		managedClusterKubeSecret)
	if err != nil {
		return nil, nil, err
	}
	return getClientFromKubeConfig(managedClusterKubeSecret.Data["kubeconfig"])

}

//Get the client from the auto-import-secret
func (r *ReconcileManagedCluster) getManagedClusterClientFromAutoImportSecret(
	autoImportSecret *corev1.Secret) (client.Client, *rest.Config, error) {
	//generate client using kubeconfig
	if k, ok := autoImportSecret.Data["kubeconfig"]; ok {
		return getClientFromKubeConfig(k)
	}
	token, tok := autoImportSecret.Data["token"]
	server, sok := autoImportSecret.Data["server"]
	if tok && sok {
		return getClientFromToken(string(token), string(server))
	}

	return nil, nil, fmt.Errorf("kubeconfig or token and server are missing")
}

//Create client from kubeconfig
func getClientFromKubeConfig(kubeconfig []byte) (client.Client, *rest.Config, error) {
	config, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return nil, nil, err
	}

	rconfig, err := clientcmd.NewDefaultClientConfig(
		*config,
		&clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, nil, err
	}

	client, err := client.New(rconfig, client.Options{})
	if err != nil {
		return nil, nil, err
	}

	return client, rconfig, nil
}

//Create client from token and server
func getClientFromToken(token, server string) (client.Client, *rest.Config, error) {
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
		return nil, nil, err
	}
	clientClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return nil, nil, err
	}

	return clientClient, restConfig, nil
}

func getManagedClusterKubeVersion(rConfig *rest.Config) (string, error) {
	kubeClient, err := kubernetes.NewForConfig(rConfig)
	if err != nil {
		return "", err
	}

	//Search the kubernestes version by connecting to the managed cluster
	kubeVersion, err := kubeClient.ServerVersion()
	if err != nil {
		return "", err
	}
	return kubeVersion.String(), nil
}

func (r *ReconcileManagedCluster) updateAutoImportRetry(
	managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret) error {
	if autoImportSecret != nil {
		//Decrement the autoImportRetry
		autoImportRetry, err := strconv.Atoi(string(autoImportSecret.Data[autoImportRetryName]))
		if err != nil {
			return err
		}
		klog.Infof("Retry left to import %s: %d", managedCluster.Name, autoImportRetry)
		autoImportRetry--
		//Remove if negatif as a label can not start with "-", should start by a char
		if autoImportRetry < 0 {
			err = r.client.Delete(context.TODO(), autoImportSecret)
			if err != nil {
				return err
			}
			autoImportSecret = nil
		} else {
			v := []byte(strconv.Itoa(autoImportRetry))
			autoImportSecret.Data[autoImportRetryName] = v
			err := r.client.Update(context.TODO(), autoImportSecret)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

//importCluster import a cluster if autoImportRetry > 0
func (r *ReconcileManagedCluster) importClusterWithClient(
	managedCluster *clusterv1.ManagedCluster,
	autoImportSecret *corev1.Secret,
	managedClusterClient client.Client,
	managedClusterKubeVersion string) (reconcile.Result, error) {

	klog.Infof("Importing cluster: %s", managedCluster.Name)

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

	var bb [][]byte
	//Convert crds to Yaml
	isV1, err := isAPIExtensionV1(managedClusterClient, managedCluster, managedClusterKubeVersion)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}
	if isV1 {
		bb, err = templateprocessor.ToYAMLsUnstructured(crds["v1"])
	} else {
		bb, err = templateprocessor.ToYAMLsUnstructured(crds["v1beta1"])
	}
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
		return reconcile.Result{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
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

	reqLogger.Info(fmt.Sprintf("deleteKlusterletManifestWorks: %s", instance.Name))
	err = deleteKlusterletManifestWorks(r.client, instance)
	if err != nil {
		return reconcile.Result{}, err
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
