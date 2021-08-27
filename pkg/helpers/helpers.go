// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	operatorv1 "github.com/open-cluster-management/api/operator/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"

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
		RuntimeClient:       runtimeClient,
	}, mapper, nil
}

// AddManagedClusterFinalizer add a finalizer to a managed cluster
func AddManagedClusterFinalizer(client client.Client, recorder events.Recorder,
	managedCluster *clusterv1.ManagedCluster, finalizer string) error {
	for i := range managedCluster.Finalizers {
		if managedCluster.Finalizers[i] == finalizer {
			return nil
		}
	}

	managedCluster.Finalizers = append(managedCluster.Finalizers, finalizer)
	if err := client.Update(context.TODO(), managedCluster); err != nil {
		return err
	}

	recorder.Eventf("ManagedClusterFinalizerAdded",
		"The managed cluster %s finalizer %s is added", managedCluster.Name, finalizer)

	return nil
}

// RemoveManagedClusterFinalizer remove a finalizer from a managed cluster
func RemoveManagedClusterFinalizer(client client.Client, recorder events.Recorder,
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

	managedCluster.Finalizers = copiedFinalizers
	if err := client.Update(context.TODO(), managedCluster); err != nil {
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
	if equality.Semantic.DeepEqual(managedCluster.Status, newStatus) {
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
		return fmt.Errorf("the %s is required", constants.ImportSecretCRDSV1YamlKey)
	}
	return nil
}

// ImportManagedClusterFromSecret use managed clustr client to import managed cluster from import-secret
func ImportManagedClusterFromSecret(client *ClientHolder, restMapper meta.RESTMapper, recorder events.Recorder,
	importSecret *corev1.Secret) error {
	if err := ValidateImportSecret(importSecret); err != nil {
		return err
	}

	objs := []runtime.Object{}
	objs = append(objs, mustCreateObject(importSecret.Data[constants.ImportSecretCRDSV1beta1YamlKey]))

	_, err := restMapper.RESTMapping(crdGroupKind, "v1")
	if err == nil {
		// the two versions of the crd are added to avoid the unexpected removal during the work-agent upgrade.
		// we will remove the v1beta1 in a future z-release. see: https://github.com/open-cluster-management/backlog/issues/13631
		klog.V(4).Infof("crd v1 is supported")
		objs = append(objs, mustCreateObject(importSecret.Data[constants.ImportSecretCRDSV1YamlKey]))
	}

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
			errs = append(errs, applyKlusterlet(clientHolder.RuntimeClient, recorder, required))
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

func applyKlusterlet(client client.Client, recorder events.Recorder, required *operatorv1.Klusterlet) error {
	existing := &operatorv1.Klusterlet{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: required.Name}, existing)
	if errors.IsNotFound(err) {
		if err := client.Create(context.TODO(), required); err != nil {
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

	existing.Spec = required.Spec
	if err := client.Update(context.TODO(), existing); err != nil {
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
	namespace, err := k8sutil.GetOperatorNamespace()
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
