//Package clusterdeployment ...
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package clusterdeployment

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/ghodss/yaml"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	multicloudv1beta1 "github.ibm.com/IBMPrivateCloud/ibm-klusterlet-operator/pkg/apis/multicloud/v1beta1"
	multicloudv1alpha1 "github.ibm.com/IBMPrivateCloud/mcm-cluster-controller/pkg/apis/multicloud/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_clusterdeployment")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ClusterDeployment Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeployment{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner ClusterDeployment

	return nil
}

// blank assignment to verify that ReconcileClusterDeployment implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileClusterDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterDeployment")

	// Fetch the ClusterDeployment instance
	instance := &hivev1.ClusterDeployment{}

	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// create cluster namespace if does not exist
	foundClusterRegistryNamespace := &corev1.Namespace{}

	if err := r.client.Get(context.TODO(), clusterRegistryNamespaceNamespacedName(instance), foundClusterRegistryNamespace); err != nil {
		if errors.IsNotFound(err) {
			namespace := newClusterRegistryNamespace(instance)

			reqLogger.Info("Creating a ClusterRegistry Namespace", "Namespace.Name", namespace.Name)

			if err := r.client.Create(context.TODO(), namespace); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// requeue until EndpointConfig is created for the cluster
	endpointConfig := &multicloudv1alpha1.EndpointConfig{}
	if err := r.client.Get(context.TODO(), endpointConfigNamespacedName(instance), endpointConfig); err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}

	// create cluster registry cluster if does not exist
	foundClusterRegistryCluster := &clusterregistryv1alpha1.Cluster{}
	if err := r.client.Get(context.TODO(), clusterRegistryClusterNamespacedName(instance), foundClusterRegistryCluster); err != nil {
		if errors.IsNotFound(err) {
			cluster := newClusterRegistryCluster(endpointConfig)

			if err := controllerutil.SetControllerReference(instance, cluster, r.scheme); err != nil {
				return reconcile.Result{}, err
			}

			reqLogger.Info("Creating a ClusterRegistry Cluster", "Cluster.Namespace", cluster.Namespace, "Cluster.Name", cluster.Name)

			if err := r.client.Create(context.TODO(), cluster); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	for _, condition := range foundClusterRegistryCluster.Status.Conditions {
		if condition.Type == clusterregistryv1alpha1.ClusterOK {
			//cluster already imported and online, so do nothing
			return reconcile.Result{}, nil
		}
	}

	// create bootstrap service account if does not exist
	foundBootStrapServiceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), bootstrapServiceAccountNamespacedName(instance), foundBootStrapServiceAccount); err != nil {
		if errors.IsNotFound(err) {
			serviceAccount := newBootstrapServiceAccount(instance)

			if err := controllerutil.SetControllerReference(instance, serviceAccount, r.scheme); err != nil {
				return reconcile.Result{}, err
			}

			reqLogger.Info("Creating a Bootstrap ServiceAccount", "ServiceAccount.Namespace", serviceAccount.Namespace, "ServiceAccount.Name", serviceAccount.Name)

			if err := r.client.Create(context.TODO(), serviceAccount); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// create or update syncset
	syncSet, err := newSyncSet(instance, r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	foundSyncSet := &hivev1.SyncSet{}

	if err := r.client.Get(context.TODO(), syncSetNamespacedName(instance), foundSyncSet); err != nil {
		if errors.IsNotFound(err) {
			if err := controllerutil.SetControllerReference(instance, syncSet, r.scheme); err != nil {
				return reconcile.Result{}, err
			}

			reqLogger.Info("Creating a Multicluster Endpoint SyncSet", "SyncSet.Namespace", syncSet.Namespace, "SyncSet.Name", syncSet.Name)

			if err := r.client.Create(context.TODO(), syncSet); err != nil {
				return reconcile.Result{}, err
			}

			foundSyncSet = syncSet
		}
	} else {
		foundSyncSet.Spec.SyncSetCommonSpec.Resources = syncSet.Spec.SyncSetCommonSpec.Resources
		if err := r.client.Update(context.TODO(), foundSyncSet); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func syncSetNamespacedName(cr *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-multicluster-endpoint",
		Namespace: cr.Namespace,
	}
}

func newSyncSet(cr *hivev1.ClusterDeployment, client client.Client) (*hivev1.SyncSet, error) {
	runtimeRawExtensions, err := generateImportObjects(cr, client)
	if err != nil {
		return nil, err
	}

	syncSet := &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "SyncSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-multicluster-endpoint",
			Namespace: cr.Namespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				Resources: runtimeRawExtensions,
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: cr.Name,
				},
			},
		},
	}

	return syncSet, nil
}

func generateOperatorDeployment(endpointConfig *multicloudv1alpha1.EndpointConfig) *appsv1.Deployment {
	imageName := endpointConfig.Spec.ImageRegistry +
		"/icp-multicluster-endpoint-operator" +
		endpointConfig.Spec.ImageNamePostfix +
		":" + endpointConfig.Spec.Version

	imagePullSecrets := []corev1.LocalObjectReference{}
	if len(endpointConfig.Spec.ImagePullSecret) > 0 {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: endpointConfig.Spec.ImagePullSecret})
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibm-multicluster-endpoint-operator",
			Namespace: "multicluster-endpoint",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "ibm-multicluster-endpoint-operator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": "ibm-multicluster-endpoint-operator",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "ibm-multicluster-endpoint-operator",
					Containers: []corev1.Container{
						{
							Name:            "ibm-multicluster-endpoint-operator",
							Image:           imageName,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "WATCH_NAMESPACE",
									Value: "",
								},
								{
									Name:  "OPERATOR_NAME",
									Value: "ibm-multicluster-endpoint-operator",
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
						},
					},
					ImagePullSecrets: imagePullSecrets,
				},
			},
		},
	}
}

func endpointConfigNamespacedName(cr *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Spec.ClusterName,
		Namespace: cr.Spec.ClusterName,
	}
}

func generateImportObjects(cr *hivev1.ClusterDeployment, client client.Client) ([]runtime.RawExtension, error) {
	runtimeObjects := []runtime.Object{}

	endpointConfig := &multicloudv1alpha1.EndpointConfig{}
	if err := client.Get(context.TODO(), endpointConfigNamespacedName(cr), endpointConfig); err != nil {
		return nil, err
	}

	crd, err := generateCRD(os.Getenv("ENDPOINT_CRD_FILE"))
	if err != nil {
		return nil, err
	}

	runtimeObjects = append(runtimeObjects, crd)

	endpointNamespace := generateEndpointNamespace()
	runtimeObjects = append(runtimeObjects, endpointNamespace)

	serviceAccount := generateOperatorServiceAccount()
	runtimeObjects = append(runtimeObjects, serviceAccount)

	clusterRoleBinding := generateClusterRoleBinding()
	runtimeObjects = append(runtimeObjects, clusterRoleBinding)

	bootstrapSecret, err := generateBootstrapSecret(cr, client)
	if err != nil {
		return nil, err
	}

	runtimeObjects = append(runtimeObjects, bootstrapSecret)

	imagePullSecret, err := generateImagePullSecret(endpointConfig, client)
	if err != nil {
		return nil, err
	}

	if imagePullSecret != nil {
		runtimeObjects = append(runtimeObjects, imagePullSecret)
	}

	operatorDeployment := generateOperatorDeployment(endpointConfig)
	runtimeObjects = append(runtimeObjects, operatorDeployment)

	endpoint := generateEndpoint(endpointConfig)
	runtimeObjects = append(runtimeObjects, endpoint)

	runtimeRawExtensions := []runtime.RawExtension{}

	for _, obj := range runtimeObjects {
		if obj != nil {
			runtimeRawExtensions = append(runtimeRawExtensions, runtime.RawExtension{Object: obj})
		}
	}

	return runtimeRawExtensions, nil
}

func generateClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "ibm-multicluster-endpoint-operator",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "ibm-multicluster-endpoint-operator",
				Namespace: "multicluster-endpoint",
			},
		},
	}
}

func generateOperatorServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibm-multicluster-endpoint-operator",
			Namespace: "multicluster-endpoint",
		},
	}
}

func generateEndpoint(endpointConfig *multicloudv1alpha1.EndpointConfig) *multicloudv1beta1.Endpoint {
	return &multicloudv1beta1.Endpoint{
		TypeMeta: metav1.TypeMeta{
			APIVersion: multicloudv1beta1.SchemeGroupVersion.String(),
			Kind:       "Endpoint",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoint",
			Namespace: "multicluster-endpoint",
		},
		Spec: endpointConfig.Spec,
	}
}

func generateImagePullSecret(endpointConfig *multicloudv1alpha1.EndpointConfig, client client.Client) (*corev1.Secret, error) {
	if len(endpointConfig.Spec.ImagePullSecret) == 0 {
		return nil, nil
	}

	foundSecret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{
		Name:      endpointConfig.Spec.ImagePullSecret,
		Namespace: endpointConfig.Namespace,
	}

	if err := client.Get(context.TODO(), secretNamespacedName, foundSecret); err != nil {
		return nil, err
	}

	//liuhao: add validation for secret Type
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      foundSecret.Name,
			Namespace: "multicluster-endpoint",
		},
		Data: foundSecret.Data,
		Type: foundSecret.Type,
	}, nil
}

func generateCRD(fileName string) (*apiextensionv1beta1.CustomResourceDefinition, error) {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Error(err, "fail to CRD ReadFile", "filename", fileName)
		return nil, err
	}

	crd := &apiextensionv1beta1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(data, crd); err != nil {
		log.Error(err, "fail to Unmarshal CRD", "content", data)
		return nil, err
	}

	return crd, nil
}

func generateEndpointNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "multicluster-endpoint",
		},
	}
}

func generateBootstrapSecret(cr *hivev1.ClusterDeployment, client client.Client) (*corev1.Secret, error) {
	saNamespacedName := bootstrapServiceAccountNamespacedName(cr)
	sa := &corev1.ServiceAccount{}

	if err := client.Get(context.TODO(), saNamespacedName, sa); err != nil {
		return nil, err
	}

	saSecret := &corev1.Secret{}

	for _, secret := range sa.Secrets {
		secretNamespacedName := types.NamespacedName{
			Name:      secret.Name,
			Namespace: saNamespacedName.Namespace,
		}

		if err := client.Get(context.TODO(), secretNamespacedName, saSecret); err != nil {
			continue
		}

		if saSecret.Type == corev1.SecretTypeServiceAccountToken {
			break
		}
	}

	kubeAPIServer, err := getAPIServerAddress(client)
	if err != nil {
		return nil, err
	}

	saToken := saSecret.Data["token"]

	bootstrapConfig := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                kubeAPIServer,
			InsecureSkipTLSVerify: true,
		}},
		// Define auth based on the obtained client cert.
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			Token: string(saToken),
		}},
		// Define a context that connects the auth info and cluster, and set it as the default
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	bootstrapConfigData, err := runtime.Encode(clientcmdlatest.Codec, &bootstrapConfig)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "klusterlet-bootstrap",
			Namespace: "multicluster-endpoint",
		},
		Data: map[string][]byte{
			"kubeconfig": bootstrapConfigData,
		},
	}, nil
}

func getAPIServerAddress(client client.Client) (string, error) {
	configmap := &corev1.ConfigMap{}
	clusterInfoNamespacedName := types.NamespacedName{
		Name:      "ibmcloud-cluster-info",
		Namespace: "kube-public",
	}

	if err := client.Get(context.TODO(), clusterInfoNamespacedName, configmap); err != nil {
		return "", err
	}

	apiServerHost, ok := configmap.Data["cluster_kube_apiserver_host"]
	if !ok {
		return "", fmt.Errorf("kube-public/ibmcloud-cluster-info does not contain cluster_kube_apiserver_host")
	}

	apiServerPort, ok := configmap.Data["cluster_kube_apiserver_port"]
	if !ok {
		return "https://" + apiServerHost, nil
	}

	return "https://" + apiServerHost + ":" + apiServerPort, nil
}

func bootstrapServiceAccountNamespacedName(cr *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Spec.ClusterName + "-bootstrap-sa",
		Namespace: cr.Spec.ClusterName,
	}
}

func clusterRegistryNamespaceNamespacedName(cr *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Spec.ClusterName,
		Namespace: "",
	}
}

func clusterRegistryClusterNamespacedName(cr *hivev1.ClusterDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Spec.ClusterName,
		Namespace: cr.Spec.ClusterName,
	}
}

// newClusterRegistryNamespace returns a busybox pod with the same name/namespace as the cr
func newClusterRegistryNamespace(cr *hivev1.ClusterDeployment) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: cr.Spec.ClusterName,
		},
	}
}

// newClusterRegistryCluster returns a ClusterRegistry Cluster
func newClusterRegistryCluster(endpointConfig *multicloudv1alpha1.EndpointConfig) *clusterregistryv1alpha1.Cluster {
	return &clusterregistryv1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterregistryv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointConfig.Spec.ClusterName,
			Namespace: endpointConfig.Spec.ClusterNamespace,
			Labels:    endpointConfig.Spec.ClusterLabels,
		},
	}
}

func newBootstrapServiceAccount(cr *hivev1.ClusterDeployment) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.ClusterName + "-bootstrap-sa",
			Namespace: cr.Spec.ClusterName,
		},
	}
}
