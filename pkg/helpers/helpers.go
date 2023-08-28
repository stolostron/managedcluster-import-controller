// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crdv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextclientv1beta1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
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
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/features"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
)

const maxConcurrentReconcilesEnvVarName = "MAX_CONCURRENT_RECONCILES"

const (
	nodeSelectorAnnotation = "open-cluster-management/nodeSelector"
	tolerationsAnnotation  = "open-cluster-management/tolerations"
)

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
	utilruntime.Must(addonv1alpha1.AddToScheme(genericScheme))
}

type ClientHolder struct {
	KubeClient          kubernetes.Interface
	APIExtensionsClient apiextensionsclient.Interface
	OperatorClient      operatorclient.Interface
	RuntimeClient       client.Client
	ImageRegistryClient imageregistry.Interface
	WorkClient          workclient.Interface
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

	httpclient, err := rest.HTTPClientFor(clientConfig)
	if err != nil {
		return nil, nil, err
	}
	mapper, err := apiutil.NewDiscoveryRESTMapper(clientConfig, httpclient)
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
func UpdateManagedClusterStatus(client client.Client, managedClusterName string, cond metav1.Condition) error {
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
		klog.Errorf("Update the managed cluster %s condition %v failed, error: %v", managedClusterName, cond, err)
		return err
	}

	klog.V(4).Infof("Update the managed cluster %s condition %v succeeded.", managedClusterName, cond)

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

// ValidateHostedImportSecret validate hosted mode managed cluster import secret
func ValidateHostedImportSecret(importSecret *corev1.Secret) error {
	if data, ok := importSecret.Data[constants.ImportSecretImportYamlKey]; !ok || len(data) == 0 {
		return fmt.Errorf("the %s is required", constants.ImportSecretImportYamlKey)
	}
	return nil
}

// ImportManagedClusterFromSecret use managed cluster client to import managed cluster from import-secret
func ImportManagedClusterFromSecret(client *ClientHolder, restMapper meta.RESTMapper, recorder events.Recorder,
	importSecret *corev1.Secret) (bool, error) {
	if err := ValidateImportSecret(importSecret); err != nil {
		return false, err
	}

	crdsKey := constants.ImportSecretCRDSV1YamlKey
	if _, err := restMapper.RESTMapping(crdGroupKind, "v1"); err != nil {
		klog.Infof("crd v1 is not supported, deploy v1beta1")
		crdsKey = constants.ImportSecretCRDSV1beta1YamlKey
	}

	objs := []runtime.Object{}
	objs = append(objs, MustCreateObject(importSecret.Data[crdsKey]))
	for _, yaml := range SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
		objs = append(objs, MustCreateObject(yaml))
	}
	// using managed cluster client to apply resources in managed cluster, so the owner is not need
	return ApplyResources(client, recorder, nil, nil, objs...)
}

// UpdateManagedClusterBootstrapSecret update the bootstrap secret on the managed cluster
func UpdateManagedClusterBootstrapSecret(client *ClientHolder, importSecret *corev1.Secret,
	recorder events.Recorder) (bool, error) {

	var obj runtime.Object
	for _, yaml := range SplitYamls(importSecret.Data[constants.ImportSecretImportYamlKey]) {
		obj = MustCreateObject(yaml)
		// bootstrap-hub-kubeconfig
		secret, ok := obj.(*corev1.Secret)
		if !ok {
			continue
		}
		if secret.Name == "bootstrap-hub-kubeconfig" {
			break
		}
	}
	if obj == nil {
		return false, fmt.Errorf("failed to find bootstrap-hub-kubeconfig in import secret %s/%s",
			importSecret.Namespace, importSecret.Name)
	}
	return ApplyResources(client, recorder, nil, nil, obj)
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
	return MustCreateObject(raw)
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
	scheme *runtime.Scheme, owner metav1.Object, objs ...runtime.Object) (bool, error) {
	changed := false
	errs := []error{}
	for _, obj := range objs {
		if owner != nil && !reflect.ValueOf(owner).IsNil() {
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
			_, modified, err := resourceapply.ApplyServiceAccount(context.TODO(),
				clientHolder.KubeClient.CoreV1(), recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		case *corev1.Secret:
			_, modified, err := resourceapply.ApplySecret(context.TODO(),
				clientHolder.KubeClient.CoreV1(), recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		case *corev1.Namespace:
			_, modified, err := resourceapply.ApplyNamespace(context.TODO(),
				clientHolder.KubeClient.CoreV1(), recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		case *appsv1.Deployment:
			modified, err := applyDeployment(clientHolder, recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		case *rbacv1.ClusterRole:
			_, modified, err := resourceapply.ApplyClusterRole(context.TODO(),
				clientHolder.KubeClient.RbacV1(), recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		case *rbacv1.ClusterRoleBinding:
			_, modified, err := resourceapply.ApplyClusterRoleBinding(context.TODO(),
				clientHolder.KubeClient.RbacV1(), recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		case *crdv1beta1.CustomResourceDefinition:
			_, modified, err := ApplyCustomResourceDefinitionV1Beta1(
				clientHolder.APIExtensionsClient.ApiextensionsV1beta1(),
				recorder,
				required,
			)
			errs = append(errs, err)
			changed = changed || modified
		case *crdv1.CustomResourceDefinition:
			_, modified, err := resourceapply.ApplyCustomResourceDefinitionV1(
				context.TODO(),
				clientHolder.APIExtensionsClient.ApiextensionsV1(),
				recorder,
				required,
			)
			errs = append(errs, err)
			changed = changed || modified
		case *workv1.ManifestWork:
			modified, err := applyManifestWork(clientHolder.WorkClient, recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		case *operatorv1.Klusterlet:
			modified, err := applyKlusterlet(clientHolder.OperatorClient, recorder, required)
			errs = append(errs, err)
			changed = changed || modified
		default:
			errs = append(errs, fmt.Errorf("unknown type %T", required))
		}
	}

	return changed, utilerrors.NewAggregate(errs)
}

func applyDeployment(clientHolder *ClientHolder, recorder events.Recorder, required *appsv1.Deployment) (bool, error) {
	existing, err := clientHolder.KubeClient.AppsV1().Deployments(required.Namespace).Get(
		context.TODO(), required.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, modified, err := resourceapply.ApplyDeployment(context.TODO(),
			clientHolder.KubeClient.AppsV1(), recorder, required, -1)
		return modified, err
	}
	if err != nil {
		return false, err
	}

	_, modified, err := resourceapply.ApplyDeployment(context.TODO(),
		clientHolder.KubeClient.AppsV1(), recorder, required, existing.Generation)
	return modified, err
}

func applyKlusterlet(client operatorclient.Interface, recorder events.Recorder,
	required *operatorv1.Klusterlet) (bool, error) {

	existing, err := client.OperatorV1().Klusterlets().Get(context.TODO(), required.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		if _, err := client.OperatorV1().Klusterlets().Create(
			context.TODO(), required, metav1.CreateOptions{}); err != nil {
			return false, err
		}

		reportEvent(recorder, required, "Klusterlet", "created")
		return true, nil
	}
	if err != nil {
		return false, err
	}

	if equality.Semantic.DeepEqual(existing.Spec, required.Spec) {
		return false, nil
	}

	existing = existing.DeepCopy()
	existing.Spec = required.Spec
	if _, err := client.OperatorV1().Klusterlets().Update(
		context.TODO(), existing, metav1.UpdateOptions{}); err != nil {
		return false, err
	}
	reportEvent(recorder, required, "Klusterlet", "updated")
	return true, nil
}

func applyManifestWork(workClient workclient.Interface, recorder events.Recorder,
	required *workv1.ManifestWork) (bool, error) {

	existing, err := workClient.WorkV1().ManifestWorks(required.Namespace).Get(
		context.TODO(), required.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err := workClient.WorkV1().ManifestWorks(required.Namespace).Create(
			context.TODO(), required, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}

		reportEvent(recorder, required, "ManifestWork", "created")
		return true, nil
	}
	if err != nil {
		return false, err
	}

	modified := resourcemerge.BoolPtr(false)
	resourcemerge.EnsureObjectMeta(modified, &existing.ObjectMeta, required.ObjectMeta)
	if !ManifestsEqual(existing.Spec.Workload.Manifests, required.Spec.Workload.Manifests) {
		*modified = true
	}

	if !*modified {
		return false, nil
	}

	existing.Spec = required.Spec
	if _, err := workClient.WorkV1().ManifestWorks(required.Namespace).Update(context.TODO(), existing, metav1.UpdateOptions{}); err != nil {
		return false, err
	}
	reportEvent(recorder, required, "ManifestWork", "updated")
	return true, nil
}

// MustCreateObject translate object from raw bytes to runtime object
func MustCreateObject(raw []byte) runtime.Object {
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

	recorder.Eventf(fmt.Sprintf("%s%s", objKind, cases.Title(language.English).String(action)), "%s is %s", name, action)
}

func NewEventRecorder(kubeClient kubernetes.Interface, controllerName string) events.Recorder {
	namespace, err := GetComponentNamespace()
	if err != nil {
		klog.Warningf("unable to identify the current namespace for events: %v", err)
	}

	controllerRef, err := events.GetControllerReferenceForCurrentPod(context.TODO(), kubeClient, namespace, nil)
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
	nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}
	return string(nsBytes), nil
}

func GetNodeSelectorFromManagedClusterAnnotations(clusterAnnotations map[string]string) (map[string]string, error) {
	nodeSelector := map[string]string{}

	nodeSelectorString, ok := clusterAnnotations[nodeSelectorAnnotation]
	if !ok {
		return nodeSelector, nil
	}

	if err := json.Unmarshal([]byte(nodeSelectorString), &nodeSelector); err != nil {
		return nil, fmt.Errorf("invalid nodeSelector annotation %v", err)
	}

	return nodeSelector, nil
}

func GetTolerationsFromManagedClusterAnnotations(clusterAnnotations map[string]string) ([]corev1.Toleration, error) {
	tolerations := []corev1.Toleration{}

	tolerationsString, ok := clusterAnnotations[tolerationsAnnotation]
	if !ok {
		// return a defautl toleration
		return []corev1.Toleration{
			{
				Effect:   corev1.TaintEffectNoSchedule,
				Key:      "node-role.kubernetes.io/infra",
				Operator: corev1.TolerationOpExists,
			},
		}, nil
	}

	if err := json.Unmarshal([]byte(tolerationsString), &tolerations); err != nil {
		return nil, fmt.Errorf("invalid tolerations annotation %v", err)
	}

	return tolerations, nil
}

// DetermineKlusterletMode gets the klusterlet deploy mode for the managed cluster.
func DetermineKlusterletMode(cluster *clusterv1.ManagedCluster) string {
	mode, ok := cluster.Annotations[constants.KlusterletDeployModeAnnotation]
	if !ok {
		return constants.KlusterletDeployModeDefault
	}

	if strings.EqualFold(mode, constants.KlusterletDeployModeDefault) {
		return constants.KlusterletDeployModeDefault
	}

	if strings.EqualFold(mode, constants.KlusterletDeployModeHosted) {
		return constants.KlusterletDeployModeHosted
	}

	return "Unknown"
}

func ValidateKlusterletMode(mode string) error {
	if strings.EqualFold(mode, constants.KlusterletDeployModeHosted) && !features.DefaultMutableFeatureGate.Enabled(features.KlusterletHostedMode) {
		return fmt.Errorf("featurn gate %s is not enabled", features.KlusterletHostedMode)
	}
	return nil
}

// GetHostingCluster gets the hosting cluster name from the managed cluster annotation
func GetHostingCluster(cluster *clusterv1.ManagedCluster) (string, error) {
	if hostingCluster, ok := cluster.Annotations[constants.HostingClusterNameAnnotation]; ok {
		return hostingCluster, nil
	}

	return "", fmt.Errorf("annotation %s not found", constants.HostingClusterNameAnnotation)
}

// ForceDeleteManagedClusterAddon will delete the managedClusterAddon regardless of finalizers.
func ForceDeleteManagedClusterAddon(
	ctx context.Context,
	runtimeClient client.Client,
	recorder events.Recorder,
	addonNamespace, addonName string) error {

	addon := addonv1alpha1.ManagedClusterAddOn{}
	if err := runtimeClient.Get(ctx,
		types.NamespacedName{Namespace: addonNamespace, Name: addonName}, &addon); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if addon.DeletionTimestamp.IsZero() {
		if err := runtimeClient.Delete(ctx, &addon); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}

	// Remove all finalizers except "hosting-manifests-cleanup" and "hosting-addon-pre-delete"
	// The hosted mode managed cluster addon uses these finalizer to clean up the manifestworks on
	// the hosting cluster, if we remove it, the manifestworks on the hosting cluster may remain
	var finalizers []string
	for _, f := range addon.Finalizers {
		if f == addonv1alpha1.AddonDeprecatedHostingManifestFinalizer ||
			f == addonv1alpha1.AddonDeprecatedHostingPreDeleteHookFinalizer ||
			f == addonv1alpha1.AddonHostingManifestFinalizer ||
			f == addonv1alpha1.AddonHostingPreDeleteHookFinalizer {
			finalizers = append(finalizers, f)
		}
	}

	if len(finalizers) != len(addon.Finalizers) {
		patch := client.MergeFrom(addon.DeepCopy())
		addon.Finalizers = finalizers

		if err := runtimeClient.Patch(ctx, &addon, patch); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}

	recorder.Eventf("ManagedClusterAddonForceDeleted",
		fmt.Sprintf("The managedClusterAddon %s/%s is force deleted", addon.Namespace, addon.Name))
	return nil
}

// ForceDeleteAllManagedClusterAddons delete all managed cluster addons forcefully
func ForceDeleteAllManagedClusterAddons(
	ctx context.Context,
	runtimeClient client.Client,
	recorder events.Recorder,
	clusterName string) error {
	addons, err := ListManagedClusterAddons(ctx, runtimeClient, clusterName)
	if err != nil {
		return err
	}
	for _, item := range addons.Items {
		if err := ForceDeleteManagedClusterAddon(ctx, runtimeClient, recorder, item.Namespace, item.Name); err != nil {
			return err
		}
	}
	return nil
}

// refer to https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/validation/validation.go#L3498
func ValidateNodeSelector(nodeSelector map[string]string) error {
	errs := []error{}
	for key, val := range nodeSelector {
		if errMsgs := validation.IsQualifiedName(key); len(errMsgs) != 0 {
			errs = append(errs, fmt.Errorf(strings.Join(errMsgs, ";")))
		}
		if errMsgs := validation.IsValidLabelValue(val); len(errMsgs) != 0 {
			errs = append(errs, fmt.Errorf(strings.Join(errMsgs, ";")))
		}
	}
	return utilerrors.NewAggregate(errs)
}

// refer to https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/validation/validation.go#L3330
func ValidateTolerations(tolerations []corev1.Toleration) error {
	errs := []error{}
	for _, toleration := range tolerations {
		// validate the toleration key
		if len(toleration.Key) > 0 {
			if errMsgs := validation.IsQualifiedName(toleration.Key); len(errMsgs) != 0 {
				errs = append(errs, fmt.Errorf(strings.Join(errMsgs, ";")))
			}
		}

		// empty toleration key with Exists operator and empty value means match all taints
		if len(toleration.Key) == 0 && toleration.Operator != corev1.TolerationOpExists {
			if len(toleration.Operator) == 0 {
				errs = append(errs, fmt.Errorf(
					"operator must be Exists when `key` is empty, which means \"match all values and all keys\""))
			}
		}

		if toleration.TolerationSeconds != nil && toleration.Effect != corev1.TaintEffectNoExecute {
			errs = append(errs, fmt.Errorf("effect must be 'NoExecute' when `tolerationSeconds` is set"))
		}

		// validate toleration operator and value
		switch toleration.Operator {
		// empty operator means Equal
		case corev1.TolerationOpEqual, "":
			if errMsgs := validation.IsValidLabelValue(toleration.Value); len(errMsgs) != 0 {
				errs = append(errs, fmt.Errorf(strings.Join(errMsgs, ";")))
			}
		case corev1.TolerationOpExists:
			if len(toleration.Value) > 0 {
				errs = append(errs, fmt.Errorf("value must be empty when `operator` is 'Exists'"))
			}
		default:
			errs = append(errs, fmt.Errorf("the operator %q is not supported", toleration.Operator))
		}

		// validate toleration effect, empty toleration effect means match all taint effects
		if len(toleration.Effect) > 0 {
			switch toleration.Effect {
			case corev1.TaintEffectNoSchedule, corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute:
				// allowed values are NoSchedule, PreferNoSchedule and NoExecute
			default:
				errs = append(errs, fmt.Errorf("the effect %q is not supported", toleration.Effect))
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

// In order to support ocp 311, copy this func from old library-go
func ApplyCustomResourceDefinitionV1Beta1(client apiextclientv1beta1.CustomResourceDefinitionsGetter,
	recorder events.Recorder,
	required *crdv1beta1.CustomResourceDefinition) (*crdv1beta1.CustomResourceDefinition, bool, error) {
	existing, err := client.CustomResourceDefinitions().Get(context.TODO(), required.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		actual, err := client.CustomResourceDefinitions().Create(context.TODO(), required, metav1.CreateOptions{})
		reportEvent(recorder, required, "CustomResourceDefinition", "created")
		return actual, true, err
	}
	if err != nil {
		return nil, false, err
	}

	modified := resourcemerge.BoolPtr(false)
	existingCopy := existing.DeepCopy()
	resourcemerge.EnsureCustomResourceDefinitionV1Beta1(modified, existingCopy, *required)
	if !*modified {
		return existing, false, nil
	}

	if klog.V(4).Enabled() {
		klog.Infof("CustomResourceDefinition %q changes: %s", existing.Name, resourceapply.JSONPatchNoError(existing, existingCopy))
	}

	actual, err := client.CustomResourceDefinitions().Update(context.TODO(), existingCopy, metav1.UpdateOptions{})
	reportEvent(recorder, required, "CustomResourceDefinition", "updated")

	return actual, true, err
}
