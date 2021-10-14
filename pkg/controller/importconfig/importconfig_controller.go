// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importconfig

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/open-cluster-management/managedcluster-import-controller/pkg/constants"
	"github.com/open-cluster-management/managedcluster-import-controller/pkg/helpers"

	"github.com/openshift/library-go/pkg/operator/events"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const bootstrapSASuffix = "bootstrap-sa"

const clusterImageRegistryLabel = "open-cluster-management.io/image-registry"

/* #nosec */
const (
	registrationOperatorImageEnvVarName = "REGISTRATION_OPERATOR_IMAGE"
	registrationImageEnvVarName         = "REGISTRATION_IMAGE"
	workImageEnvVarName                 = "WORK_IMAGE"
	defaultImagePullSecretEnvVarName    = "DEFAULT_IMAGE_PULL_SECRET"
)

const klusterletNamespace = "open-cluster-management-agent"

const managedClusterImagePullSecretName = "open-cluster-management-image-pull-credentials"

const (
	klusterletCrdsV1File      = "manifests/klusterlet/crds/klusterlets.crd.v1.yaml"
	klusterletCrdsV1beta1File = "manifests/klusterlet/crds/klusterlets.crd.v1beta1.yaml"
)

var hubFiles = []string{
	"manifests/hub/managedcluster-service-account.yaml",
	"manifests/hub/managedcluster-clusterrole.yaml",
	"manifests/hub/managedcluster-clusterrolebinding.yaml",
}

var klusterletFiles = []string{
	"manifests/klusterlet/namespace.yaml",
	"manifests/klusterlet/service_account.yaml",
	"manifests/klusterlet/bootstrap_secret.yaml",
	"manifests/klusterlet/cluster_role.yaml",
	"manifests/klusterlet/clusterrole_aggregate.yaml",
	"manifests/klusterlet/cluster_role_binding.yaml",
	"manifests/klusterlet/operator.yaml",
	"manifests/klusterlet/klusterlet.yaml",
}

var log = logf.Log.WithName(controllerName)

//go:embed manifests
var manifestFiles embed.FS

// ReconcileImportConfig reconciles a managed cluster to prepare its import secret
type ReconcileImportConfig struct {
	clientHolder *helpers.ClientHolder
	scheme       *runtime.Scheme
	recorder     events.Recorder
}

// blank assignment to verify that ReconcileImportConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImportConfig{}

// Reconcile one managed cluster to prepare its import secret.
//
// Note: The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImportConfig) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)
	reqLogger.Info("Reconciling managed cluster import secret")

	managedCluster := &clusterv1.ManagedCluster{}
	err := r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: request.Name}, managedCluster)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// make sure the managed cluster clusterrole, cluserrolebiding and bootstrap sa are updated
	config := struct {
		ManagedClusterName          string
		ManagedClusterNamespace     string
		BootstrapServiceAccountName string
	}{
		ManagedClusterName:          managedCluster.Name,
		ManagedClusterNamespace:     managedCluster.Name,
		BootstrapServiceAccountName: fmt.Sprintf("%s-%s", managedCluster.Name, bootstrapSASuffix),
	}
	objects := []runtime.Object{}
	for _, file := range hubFiles {
		template, err := manifestFiles.ReadFile(file)
		if err != nil {
			// this should not happen, if happened, panic here
			panic(err)
		}

		objects = append(objects, helpers.MustCreateObjectFromTemplate(file, template, config))
	}

	if err := helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, objects...); err != nil {
		return reconcile.Result{}, err
	}

	// make sure the managed cluster import secret is updated
	importSecret, err := r.generateImportSecret(ctx, managedCluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := helpers.ApplyResources(r.clientHolder, r.recorder, r.scheme, managedCluster, importSecret); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileImportConfig) generateImportSecret(ctx context.Context, managedCluster *clusterv1.ManagedCluster) (*corev1.Secret, error) {
	bootStrapSecret, err := getBootstrapSecret(ctx, r.clientHolder.KubeClient, managedCluster)
	if err != nil {
		return nil, err
	}

	bootstrapKubeconfigData, err := createKubeconfigData(ctx, r.clientHolder, bootStrapSecret)
	if err != nil {
		return nil, err
	}

	imageRegistry, imagePullSecret, err := getImagePullSecret(ctx, r.clientHolder, managedCluster)
	if err != nil {
		return nil, err
	}

	useImagePullSecret := false
	imagePullSecretDataBase64 := ""
	if imagePullSecret != nil && len(imagePullSecret.Data[".dockerconfigjson"]) != 0 {
		imagePullSecretDataBase64 = base64.StdEncoding.EncodeToString(imagePullSecret.Data[".dockerconfigjson"])
		useImagePullSecret = true
	}

	registrationOperatorImageName, err := getImage(imageRegistry, registrationOperatorImageEnvVarName)
	if err != nil {
		return nil, err
	}

	registrationImageName, err := getImage(imageRegistry, registrationImageEnvVarName)
	if err != nil {
		return nil, err
	}

	workImageName, err := getImage(imageRegistry, workImageEnvVarName)
	if err != nil {
		return nil, err
	}

	nodeSelector, err := helpers.GetNodeSelector(managedCluster)
	if err != nil {
		return nil, err
	}

	config := struct {
		KlusterletNamespace       string
		ManagedClusterNamespace   string
		BootstrapKubeconfig       string
		UseImagePullSecret        bool
		ImagePullSecretName       string
		ImagePullSecretData       string
		ImagePullSecretType       corev1.SecretType
		RegistrationOperatorImage string
		RegistrationImageName     string
		WorkImageName             string
		NodeSelector              map[string]string
	}{
		ManagedClusterNamespace:   managedCluster.Name,
		KlusterletNamespace:       klusterletNamespace,
		BootstrapKubeconfig:       base64.StdEncoding.EncodeToString(bootstrapKubeconfigData),
		UseImagePullSecret:        useImagePullSecret,
		ImagePullSecretName:       managedClusterImagePullSecretName,
		ImagePullSecretData:       imagePullSecretDataBase64,
		ImagePullSecretType:       corev1.SecretTypeDockerConfigJson,
		RegistrationOperatorImage: registrationOperatorImageName,
		RegistrationImageName:     registrationImageName,
		WorkImageName:             workImageName,
		NodeSelector:              nodeSelector,
	}

	deploymentFiles := append([]string{}, klusterletFiles...)
	if useImagePullSecret {
		deploymentFiles = append(deploymentFiles, "manifests/klusterlet/image_pull_secret.yaml")
	}

	importYAML := new(bytes.Buffer)
	for _, file := range deploymentFiles {
		template, err := manifestFiles.ReadFile(file)
		if err != nil {
			// this should not happen, if happened, panic here
			panic(err)
		}
		raw := helpers.MustCreateAssetFromTemplate(file, template, config)
		importYAML.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(raw)))
	}

	crdsV1beta1YAML := new(bytes.Buffer)
	crdsV1beta1, err := manifestFiles.ReadFile(klusterletCrdsV1beta1File)
	if err != nil {
		return nil, err
	}
	crdsV1beta1YAML.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(crdsV1beta1)))

	crdsV1YAML := new(bytes.Buffer)
	crdsV1, err := manifestFiles.ReadFile(klusterletCrdsV1File)
	if err != nil {
		return nil, err
	}
	crdsV1YAML.WriteString(fmt.Sprintf("%s%s", constants.YamlSperator, string(crdsV1)))

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedCluster.Name, constants.ImportSecretNameSuffix),
			Namespace: managedCluster.Name,
			Labels: map[string]string{
				constants.ClusterImportSecretLabel: "",
			},
		},
		Data: map[string][]byte{
			constants.ImportSecretImportYamlKey:      importYAML.Bytes(),
			constants.ImportSecretCRDSYamlKey:        crdsV1YAML.Bytes(),
			constants.ImportSecretCRDSV1YamlKey:      crdsV1YAML.Bytes(),
			constants.ImportSecretCRDSV1beta1YamlKey: crdsV1beta1YAML.Bytes(),
		},
	}, nil
}
