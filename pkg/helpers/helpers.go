// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crdv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const maxConcurrentReconcilesEnvVarName = "MAX_CONCURRENT_RECONCILES"

// AnnotationNodeSelector key name of nodeSelector annotation synced from mch
const AnnotationNodeSelector = "open-cluster-management/nodeSelector"

var v1APIExtensionMinVersion = version.MustParseGeneric("v1.16.0")

var crdGroupKind = schema.GroupKind{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition"}

var (
	genericScheme = runtime.NewScheme()
	genericCodecs = serializer.NewCodecFactory(genericScheme)
	genericCodec  = genericCodecs.UniversalDeserializer()
)

func init() {
	utilruntime.Must(appsv1.AddToScheme(genericScheme))
	utilruntime.Must(corev1.AddToScheme(genericScheme))
	utilruntime.Must(rbacv1.AddToScheme(genericScheme))
	utilruntime.Must(crdv1beta1.AddToScheme(genericScheme))
	utilruntime.Must(crdv1.AddToScheme(genericScheme))
	utilruntime.Must(operatorv1.AddToScheme(genericScheme))
}

type ClientHolder struct {
	KubeClient          kubernetes.Interface
	APIExtensionsClient apiextensionsclient.Interface
	OperatorClient      operatorclient.Interface
	RuntimeClient       client.Client
}

// GetMaxConcurrentReconciles get the max concurrent reconciles from MAX_CONCURRENT_RECONCILES env,
// if the reconciles cannot be found, return 1
func GetMaxConcurrentReconciles() int {
	maxConcurrentReconciles := 1
	if os.Getenv(maxConcurrentReconcilesEnvVarName) != "" {
		var err error
		maxConcurrentReconciles, err = strconv.Atoi(os.Getenv(maxConcurrentReconcilesEnvVarName))
		if err != nil {
			klog.Warningf("The value of %s env is wrong, using default reconciles (1)", maxConcurrentReconcilesEnvVarName)
			maxConcurrentReconciles = 1
		}
	}
	return maxConcurrentReconciles
}

// GenerateClientFromSecret generate a client from a given secret
func GenerateClientFromSecret(secret *corev1.Secret) (*ClientHolder, meta.RESTMapper, error) {
	var err error
	var config *clientcmdapi.Config

	if kubeconfig, ok := secret.Data["kubeconfig"]; ok {
		config, err = clientcmd.Load(kubeconfig)
		if err != nil {
			return nil, nil, err
		}
	}

	token, tok := secret.Data["token"]
	server, sok := secret.Data["server"]
	if tok && sok {
		config = clientcmdapi.NewConfig()
		config.Clusters["default"] = &clientcmdapi.Cluster{
			Server:                string(server),
			InsecureSkipTLSVerify: true,
		}
		config.AuthInfos["default"] = &clientcmdapi.AuthInfo{
			Token: string(token),
		}
		config.Contexts["default"] = &clientcmdapi.Context{
			Cluster:  "default",
			AuthInfo: "default",
		}
		config.CurrentContext = "default"
	}

	if config == nil {
		return nil, nil, fmt.Errorf("kubeconfig or token and server are missing")
	}

	clientConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, err
	}

	apiExtensionsClient, err := apiextensionsclient.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, err
	}

	operatorClient, err := operatorclient.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, err
	}

	runtimeClient, err := client.New(clientConfig, client.Options{})
	if err != nil {
		return nil, nil, err
	}

	mapper, err := apiutil.NewDiscoveryRESTMapper(clientConfig)
	if err != nil {
		return nil, nil, err
	}

	return &ClientHolder{
		KubeClient:          kubeClient,
		APIExtensionsClient: apiExtensionsClient,
		OperatorClient:      operatorClient,
		RuntimeClient:       runtimeClient,
	}, mapper, nil
}

// AddManagedClusterFinalizer add a finalizer to a managed cluster
func AddManagedClusterFinalizer(modified *bool, managedCluster *clusterv1.ManagedCluster, finalizer string) {
	for i := range managedCluster.Finalizers {
		if managedCluster.Finalizers[i] == finalizer {
			return
		}
	}

	managedCluster.Finalizers = append(managedCluster.Finalizers, finalizer)
	*modified = true
}

// RemoveManagedClusterFinalizer remove a finalizer from a managed cluster
func RemoveManagedClusterFinalizer(ctx context.Context, runtimeClient client.Client, recorder events.Recorder,
	managedCluster *clusterv1.ManagedCluster, finalizer string) error {
	copiedFinalizers := []string{}
	for i := range managedCluster.Finalizers {
		if managedCluster.Finalizers[i] == finalizer {
			continue
		}
		copiedFinalizers = append(copiedFinalizers, managedCluster.Finalizers[i])
	}

	if len(managedCluster.Finalizers) == len(copiedFinalizers) {
		return nil
	}

	patch := client.MergeFrom(managedCluster.DeepCopy())
	managedCluster.Finalizers = copiedFinalizers
	if err := runtimeClient.Patch(ctx, managedCluster, patch); err != nil {
		return err
	}

	recorder.Eventf("ManagedClusterFinalizerRemoved",
		"The managed cluster %s finalizer %s is removed", managedCluster.Name, finalizer)
	return nil
}

// UpdateManagedClusterStatus update managed cluster status
func UpdateManagedClusterStatus(client client.Client, recorder events.Recorder,
	managedClusterName string, cond metav1.Condition) error {
	managedCluster := &clusterv1.ManagedCluster{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: managedClusterName}, managedCluster)
	if err != nil {
		return err
	}

	oldStatus := &managedCluster.Status
	newStatus := oldStatus.DeepCopy()

	meta.SetStatusCondition(&newStatus.Conditions, cond)
	if equality.Semantic.DeepEqual(managedCluster.Status.Conditions, newStatus.Conditions) {
		return nil
	}

	managedCluster.Status = *newStatus
	if err := client.Status().Update(context.TODO(), managedCluster); err != nil {
		return err
	}

	recorder.Eventf("ManagedClusterStatusUpdated",
		"Update the ManagedClusterImportSucceeded status of managed cluster %s to %s", managedClusterName, cond.Status)

	return nil
}

// ValidateImportSecret validate managed cluster import secret
func ValidateImportSecret(importSecret *corev1.Secret) error {
	if data, ok := importSecret.Data[constants.ImportSecretCRDSYamlKey]; !ok || len(data) == 0 {
		return fmt.Errorf("the %s is required", constants.ImportSecretCRDSYamlKey)
	}

	if data, ok := importSecret.Data[constants.ImportSecretCRDSV1beta1YamlKey]; !ok || len(data) == 0 {
		return fmt.Errorf("the %s is required", constants.ImportSecretCRDSV1beta1YamlKey)
	}

	if data, ok := importSecret.Data[constants.ImportSecretCRDSV1YamlKey]; !ok || len(data) == 0 {
		return fmt.Errorf("the %s is required", constants.ImportSecretCRDSV1YamlKey)
	}

	if data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]; !ok || len(data) == 0 {
		return fmt.Errorf("the %s is required", constants.ImportSecretImportYamlKey)
	}
	return nil
}

// ValidateHypershiftDetachedImportSecret validate hypershift managed cluster import secret
func ValidateHypershiftDetachedImportSecret(importSecret *corev1.Secret) error {
	if data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]; !ok || len(data) == 0 {
		return fmt.Errorf("the %s is required", constants.ImportSecretImportYamlKey)
	}
	return nil
}

// ImportManagedClusterFromSecret use managed cluster client to import managed cluster from import-secret
func ImportManagedClusterFromSecret(client *ClientHolder, restMapper meta.RESTMapper, recorder events.Recorder,
	importSecret *corev1.Secret) error {
	if err := ValidateImportSecret(importSecret); err != nil {
		return err
	}

	crdsKey := constants.ImportSecretCRDSV1YamlKey
	if _, err := restMapper.RESTMapping(crdGroupKind, "v1"); err != nil {
		klog.Infof("crd v1 is not supported, deploy v1beta1")
		crdsKey = constants.ImportSecretCRDSV1beta1YamlKey
	}

	objs := []runtime.Object{}
	objs = append(objs, mustCreateObject(importSecret.Data[crdsKey]))
	for _, yaml := range SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
		objs = append(objs, mustCreateObject(yaml))
	}
	// using managed cluster client to apply resources in managed cluster, so the owner is not need
	return ApplyResources(client, recorder, nil, nil, objs...)
}

// SplitYamls split yamls with sperator `---`
func SplitYamls(yamls []byte) [][]byte {
	bYamls := [][]byte{}
	// remove the head sperator
	sYamls := strings.Replace(string(yamls), constants.YamlSperator, "", 1)
	for _, yaml := range strings.Split(sYamls, constants.YamlSperator) {
		bYamls = append(bYamls, []byte(yaml))
	}
	return bYamls
}

// IsAPIExtensionV1Supported if the cluster can support the crdv1, return true
func IsAPIExtensionV1Supported(kubeVersion string) bool {
	isV1, err := v1APIExtensionMinVersion.Compare(kubeVersion)
	if err != nil {
		klog.Errorf("a bad kube version: %v", kubeVersion)
		return false
	}
	return isV1 == -1
}

// MustCreateObjectFromTemplate render a template to a runtime object with its configuration
// If it's failed, this function will panic
func MustCreateObjectFromTemplate(file string, template []byte, config interface{}) runtime.Object {
	raw := MustCreateAssetFromTemplate(file, template, config)
	return mustCreateObject(raw)
}

// MustCreateAssetFromTemplate render a template with its configuration
// If it's failed, this function will panic
func MustCreateAssetFromTemplate(name string, tb []byte, config interface{}) []byte {
	tmpl, err := template.New(name).Parse(string(tb))
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// ManifestsEqual if two manifests are equal, return true
func ManifestsEqual(newManifests, oldManifests []workv1.Manifest) bool {
	if len(newManifests) != len(oldManifests) {
		return false
	}

	for i := range newManifests {
		if !equality.Semantic.DeepEqual(newManifests[i].Raw, oldManifests[i].Raw) {
			return false
		}
	}
	return true
}

// ApplyResources apply resources, includes: serviceaccount, secret, deployment, clusterrole, clusterrolebinding,
// crdv1beta1, crdv1, manifestwork and klusterlet
func ApplyResources(clientHolder *ClientHolder, recorder events.Recorder,
	scheme *runtime.Scheme, owner metav1.Object, objs ...runtime.Object) error {
	errs := []error{}
	for _, obj := range objs {
		if owner != nil {
			required, ok := obj.(metav1.Object)
			if !ok {
				errs = append(errs, fmt.Errorf("%T is not a metav1.Object, cannot call SetControllerReference", obj))
				continue
			}

			if err := controllerutil.SetControllerReference(owner, required, scheme); err != nil {
				errs = append(errs, err)
				continue
			}
		}

		switch required := obj.(type) {
		case *corev1.ServiceAccount:
			_, _, err := resourceapply.ApplyServiceAccount(clientHolder.KubeClient.CoreV1(), recorder, required)
			errs = append(errs, err)
		case *corev1.Secret:
			_, _, err := resourceapply.ApplySecret(clientHolder.KubeClient.CoreV1(), recorder, required)
			errs = append(errs, err)
		case *corev1.Namespace:
			_, _, err := resourceapply.ApplyNamespace(clientHolder.KubeClient.CoreV1(), recorder, required)
			errs = append(errs, err)
		case *appsv1.Deployment:
			errs = append(errs, applyDeployment(clientHolder, recorder, required))
		case *rbacv1.ClusterRole:
			_, _, err := resourceapply.ApplyClusterRole(clientHolder.KubeClient.RbacV1(), recorder, required)
			errs = append(errs, err)
		case *rbacv1.ClusterRoleBinding:
			_, _, err := resourceapply.ApplyClusterRoleBinding(clientHolder.KubeClient.RbacV1(), recorder, required)
			errs = append(errs, err)
		case *crdv1beta1.CustomResourceDefinition:
			_, _, err := resourceapply.ApplyCustomResourceDefinitionV1Beta1(
				clientHolder.APIExtensionsClient.ApiextensionsV1beta1(),
				recorder,
				required,
			)
			errs = append(errs, err)
		case *crdv1.CustomResourceDefinition:
			_, _, err := resourceapply.ApplyCustomResourceDefinitionV1(
				clientHolder.APIExtensionsClient.ApiextensionsV1(),
				recorder,
				required,
			)
			errs = append(errs, err)
		case *workv1.ManifestWork:
			errs = append(errs, applyManifestWork(clientHolder.RuntimeClient, recorder, required))
		case *operatorv1.Klusterlet:
			errs = append(errs, applyKlusterlet(clientHolder.OperatorClient, recorder, required))
		}
	}

	return utilerrors.NewAggregate(errs)
}

func applyDeployment(clientHolder *ClientHolder, recorder events.Recorder, required *appsv1.Deployment) error {
	key := types.NamespacedName{Namespace: required.Namespace, Name: required.Name}
	existing := &appsv1.Deployment{}
	err := clientHolder.RuntimeClient.Get(context.TODO(), key, existing)
	if errors.IsNotFound(err) {
		_, _, err := resourceapply.ApplyDeployment(clientHolder.KubeClient.AppsV1(), recorder, required, -1)
		return err
	}
	if err != nil {
		return err
	}

	_, _, err = resourceapply.ApplyDeployment(clientHolder.KubeClient.AppsV1(), recorder, required, existing.Generation)
	return err
}

func applyKlusterlet(client operatorclient.Interface, recorder events.Recorder, required *operatorv1.Klusterlet) error {
	existing, err := client.OperatorV1().Klusterlets().Get(context.TODO(), required.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		if _, err := client.OperatorV1().Klusterlets().Create(context.TODO(), required, metav1.CreateOptions{}); err != nil {
			return err
		}

		reportEvent(recorder, required, "Klusterlet", "created")
		return nil
	}
	if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(existing.Spec, required.Spec) {
		return nil
	}

	existing = existing.DeepCopy()
	existing.Spec = required.Spec
	if _, err := client.OperatorV1().Klusterlets().Update(context.TODO(), existing, metav1.UpdateOptions{}); err != nil {
		return err
	}
	reportEvent(recorder, required, "Klusterlet", "updated")
	return nil
}

func applyManifestWork(client client.Client, recorder events.Recorder, required *workv1.ManifestWork) error {
	existing := &workv1.ManifestWork{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: required.Namespace, Name: required.Name}, existing)
	if errors.IsNotFound(err) {
		if err := client.Create(context.TODO(), required); err != nil {
			return err
		}

		reportEvent(recorder, required, "ManifestWork", "created")
		return nil
	}
	if err != nil {
		return err
	}

	if ManifestsEqual(existing.Spec.Workload.Manifests, required.Spec.Workload.Manifests) {
		return nil
	}

	existing.Spec = required.Spec
	if err := client.Update(context.TODO(), existing); err != nil {
		return err
	}
	reportEvent(recorder, required, "ManifestWork", "updated")
	return nil
}

func mustCreateObject(raw []byte) runtime.Object {
	obj, _, err := genericCodec.Decode(raw, nil, nil)
	if err != nil {
		panic(err)
	}

	return obj
}

func reportEvent(recorder events.Recorder, metaObj metav1.Object, objKind, action string) {
	name := metaObj.GetName()
	if len(metaObj.GetNamespace()) != 0 {
		name = fmt.Sprintf("%s/%s", metaObj.GetNamespace(), metaObj.GetName())
	}

	recorder.Eventf(fmt.Sprintf("%s%s", objKind, strings.Title(action)), "%s is %s", name, action)
}

func NewEventRecorder(kubeClient kubernetes.Interface, controllerName string) events.Recorder {
	namespace, err := GetComponentNamespace()
	if err != nil {
		klog.Warningf("unable to identify the current namespace for events: %v", err)
	}

	controllerRef, err := events.GetControllerReferenceForCurrentPod(kubeClient, namespace, nil)
	if err != nil {
		klog.Warningf("unable to get owner reference (falling back to namespace): %v", err)
	}

	options := events.RecommendedClusterSingletonCorrelatorOptions()
	return events.NewKubeRecorderWithOptions(kubeClient.CoreV1().Events(namespace), options, controllerName, controllerRef)
}

func GetComponentNamespace() (string, error) {
	namespace := os.Getenv(constants.PodNamespaceEnvVarName)
	if len(namespace) > 0 {
		return namespace, nil
	}
	nsBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}
	return string(nsBytes), nil
}

func GetNodeSelector(cluster *clusterv1.ManagedCluster) (map[string]string, error) {
	// This part is to support running pods related local-cluster on specified nodes,like infra nodes.
	nodeSelector := map[string]string{}
	if cluster.GetName() != "local-cluster" {
		return nodeSelector, nil
	}

	annotations := cluster.GetAnnotations()
	nodeSelectorString, ok := annotations[AnnotationNodeSelector]
	if !ok {
		return nodeSelector, nil
	}

	if err := json.Unmarshal([]byte(nodeSelectorString), &nodeSelector); err != nil {
		klog.Errorf("failed to unmarshal nodeSelector annotation of cluster %v. err=%v", cluster.GetName(), err)
		return nodeSelector, err
	}

	return nodeSelector, nil
}

// DetermineKlusterletMode gets the klusterlet deploy mode for the managed cluster.
func DetermineKlusterletMode(cluster *clusterv1.ManagedCluster) (string, error) {
	mode, ok := cluster.Labels[constants.KlusterletDeployModeLabel]
	if !ok {
		return constants.KlusterletDeployModeDefault, nil
	}

	if strings.EqualFold(mode, constants.KlusterletDeployModeDefault) {
		return constants.KlusterletDeployModeDefault, nil
	}

	if strings.EqualFold(mode, constants.KlusterletDeployModeHypershiftDetached) {
		_, err := GetManagementCluster(cluster)
		if err != nil {
			return constants.KlusterletDeployModeHypershiftDetached,
				fmt.Errorf("annotation %s is required for Hypershift-Detached mode", constants.ManagementClusterNameAnnotation)
		}

		// TODO(zhujian7): check if management cluster is a managed cluster of hub, and check its status.
		return constants.KlusterletDeployModeHypershiftDetached, nil
	}

	return "", fmt.Errorf("value %s of the label %s invalid, only supports Hypershift-Detached and Default", mode, constants.KlusterletDeployModeLabel)
}

// GetManagementCluster gets the management cluster name from the managed cluster annotation
func GetManagementCluster(cluster *clusterv1.ManagedCluster) (string, error) {
	if managementCluster, ok := cluster.Annotations[constants.ManagementClusterNameAnnotation]; ok {
		return managementCluster, nil
	}

	return "", fmt.Errorf("annotation %s not found", constants.ManagementClusterNameAnnotation)
}
