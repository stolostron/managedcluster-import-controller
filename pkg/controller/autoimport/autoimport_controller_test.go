// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	importstatus "github.com/stolostron/managedcluster-import-controller/pkg/controller/importstatus"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
}

func TestReconcile(t *testing.T) {

	// if err := os.Setenv("KUBEBUILDER_ASSETS", "./../../../_output/kubebuilder/bin"); err != nil { // uncomment these lines to run the test locally
	// 	t.Fatal(err)
	// }

	apiServer := &envtest.Environment{}
	config, err := apiServer.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer apiServer.Stop()

	spokeKubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := spokeKubeClient.CoreV1().Namespaces().Create(
		context.TODO(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "open-cluster-management-agent",
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := spokeKubeClient.CoreV1().Secrets("open-cluster-management-agent").Create(
		context.TODO(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bootstrap-hub-kubeconfig",
			},
			Data: map[string][]byte{
				"kubeconfig": []byte("dumb"),
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		t.Fatal(err)
	}

	managedClusterName := "test"
	staleTransitionTime := metav1.NewTime(time.Now().Add(-time.Hour))
	cases := []struct {
		name                    string
		objs                    []client.Object
		works                   []runtime.Object
		secrets                 []runtime.Object
		autoImportStrategy      string
		expectedErr             bool
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
		expectedStaleAnnotation *bool
	}{
		{
			name:        "no cluster",
			objs:        []client.Object{},
			expectedErr: false,
		},
		{
			name: "hosted cluster",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).
					WithAnnotations(constants.KlusterletDeployModeAnnotation, string(operatorv1.InstallModeHosted)).
					Build(),
			},
			works:       []runtime.Object{},
			secrets:     []runtime.Object{},
			expectedErr: false,
		},
		{
			name: "auto import disabled",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).
					WithAnnotations(apiconstants.DisableAutoImportAnnotation, "").
					Build(),
			},
			works:       []runtime.Object{},
			secrets:     []runtime.Object{},
			expectedErr: false,
		},
		{
			name: "with ImportOnly strategy",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).
					WithImportedCondition(true).
					Build(),
			},
			works:                   []runtime.Object{},
			secrets:                 []runtime.Object{},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: constants.ConditionReasonManagedClusterImported,
		},
		{
			name: "with ImportOnly strategy and immediate-import annotation",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).
					WithAnnotations(apiconstants.AnnotationImmediateImport, "").
					WithImportedCondition(true).
					Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name: "with ImportOnly strategy and unempty immediate-import annotation",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).
					WithImportedCondition(true).
					WithAnnotations(apiconstants.AnnotationImmediateImport, "Completed").
					Build(),
			},
			works:                   []runtime.Object{},
			secrets:                 []runtime.Object{},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: constants.ConditionReasonManagedClusterImported,
		},
		{
			name: "with ImportOnly strategy and import success predating the auto import secret",
			objs: []client.Object{
				func() *clusterv1.ManagedCluster {
					mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
						WithImportedCondition(true).Build()
					// the import success was recorded by a stale klusterlet agent of the
					// previous cluster incarnation an hour ago (ACM-49455)
					condition := meta.FindStatusCondition(
						mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
					condition.LastTransitionTime = staleTransitionTime
					return mc
				}(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
						// the auto import secret is created now, after the import success
						// was recorded, an explicit request to import the cluster again
						CreationTimestamp: metav1.Now(),
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			// applying the import resources fails: the failure condition is written so the
			// failure is visible, and the stale import success annotation keeps the
			// re-trigger armed
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
			expectedStaleAnnotation: ptr.To(true),
		},
		{
			name: "with ImportOnly strategy and import success predating the auto import secret while waiting for import secret",
			objs: []client.Object{
				func() *clusterv1.ManagedCluster {
					mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
						WithImportedCondition(true).Build()
					// the import success was recorded by a stale klusterlet agent of the
					// previous cluster incarnation an hour ago (ACM-49455)
					condition := meta.FindStatusCondition(
						mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
					condition.LastTransitionTime = staleTransitionTime
					return mc
				}(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				// the auto import secret is newer than the stale import success (an
				// explicit re-import request), but the import secret is not created yet:
				// the re-triggered import waits before applying resources, the wait
				// condition is written and the stale import success annotation keeps the
				// re-trigger armed
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "auto-import-secret",
						Namespace:         managedClusterName,
						CreationTimestamp: metav1.Now(),
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedStaleAnnotation: ptr.To(true),
		},
		{
			name: "with ImportOnly strategy and stale import success annotation",
			objs: []client.Object{
				func() *clusterv1.ManagedCluster {
					mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
						WithImportedCondition(true).
						WithAnnotations(constants.AnnotationStaleImportSuccess, constants.LabelValueTrue).
						Build()
					// the import success condition has a fresh timestamp (e.g. it was
					// restored by the import status controller from the ManifestWork
					// availability of the previous cluster incarnation); the annotation
					// must keep the re-trigger armed regardless of the timestamp
					condition := meta.FindStatusCondition(
						mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
					condition.LastTransitionTime = metav1.NewTime(time.Now())
					return mc
				}(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				// the auto import secret is older than the fresh import success timestamp,
				// so only the annotation re-triggers the import
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "auto-import-secret",
						Namespace:         managedClusterName,
						CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
			expectedStaleAnnotation: ptr.To(true),
		},
		{
			name: "with ImportOnly strategy and import success after the auto import secret",
			objs: []client.Object{
				func() *clusterv1.ManagedCluster {
					mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
						WithImportedCondition(true).Build()
					// the import succeeded after the auto import secret was created, so the
					// import is fresh and must not be retried
					condition := meta.FindStatusCondition(
						mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
					condition.LastTransitionTime = metav1.NewTime(time.Now())
					return mc
				}(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
						// the auto import secret was created an hour before the import
						// succeeded, so the import is fresh and the skip must be kept
						CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: constants.ConditionReasonManagedClusterImported,
			expectedStaleAnnotation: ptr.To(false),
		},
		{
			name: "with ImportOnly strategy and unknown auto import secret creation time",
			objs: []client.Object{
				func() *clusterv1.ManagedCluster {
					mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
						WithImportedCondition(true).Build()
					condition := meta.FindStatusCondition(
						mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
					condition.LastTransitionTime = metav1.NewTime(time.Now().Add(-time.Hour))
					return mc
				}(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
						// no CreationTimestamp, the creation time of the auto import
						// secret is unknown so the current skip behavior is kept
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: constants.ConditionReasonManagedClusterImported,
		},
		{
			name: "no auto-import-secret",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works:       []runtime.Object{},
			secrets:     []runtime.Object{},
			expectedErr: false,
		},
		{
			name: "unsupported auto-import-secret type",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{},
				},
			},
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name: "no manifest works",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name: "no manifest works (compatibility)",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: corev1.SecretTypeOpaque,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name: "no import-secret",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name: "return error when applying manifests fails",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name: "import cluster with auto-import secret invalid",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{
						"server": []byte(config.Host),
						// no auth info
					},
					Type: constants.AutoImportSecretKubeToken,
				},
			},
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name: "import cluster with auto-import secret invalid (compatibility)",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{
						"server": []byte(config.Host),
						// no auth info
					},
					Type: corev1.SecretTypeOpaque,
				},
			},
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name: "import rosa cluster with auto-import secret invalid",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
					},
					Data: map[string][]byte{},
					Type: constants.AutoImportSecretRosaConfig,
				},
			},
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name: "only update the bootstrap secret",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.LabelAutoImportRestore: "true",
						},
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name: "only update the bootstrap secret - works unavailable",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.LabelAutoImportRestore: "true",
						},
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name: "only update the bootstrap secret - works available",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
					Status: workv1.ManifestWorkStatus{
						Conditions: []metav1.Condition{
							{
								Type:   workv1.WorkAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret(managedClusterName),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
						Labels: map[string]string{
							constants.LabelAutoImportRestore: "true",
						},
					},
					Data: map[string][]byte{
						"kubeconfig": testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(c.secrets...)
			kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
			secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
			for _, secret := range c.secrets {
				secretInformer.GetStore().Add(secret)
			}

			workClient := workfake.NewSimpleClientset(c.works...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			ctx := context.TODO()
			importConfigLister := testinghelpers.FakeImportControllerConfigLister("test", c.autoImportStrategy, "")
			r := NewReconcileAutoImport(
				fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.objs...).WithStatusSubresource(c.objs...).Build(),
				kubeClient,
				&source.InformerHolder{
					AutoImportSecretLister: kubeInformerFactory.Core().V1().Secrets().Lister(),
					ImportSecretLister:     kubeInformerFactory.Core().V1().Secrets().Lister(),
					KlusterletWorkLister:   workInformerFactory.Work().V1().ManifestWorks().Lister(),
				},
				eventstesting.NewTestingEventRecorder(t),
				helpers.NewManagedClusterEventRecorder(ctx, kubeClient),
				helpers.NewImportControllerConfig("test", importConfigLister,
					logf.Log.WithName("fake-import-controller-config")),
			)

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}
			_, err := r.Reconcile(ctx, req)
			if c.expectedErr && err == nil {
				t.Errorf("name: %v, expected error, but failed", c.name)
			}
			if !c.expectedErr && err != nil {
				t.Errorf("name: %v, unexpected error: %v", c.name, err)
			}

			if c.expectedConditionReason != "" {

				managedCluster := &clusterv1.ManagedCluster{}
				err = r.client.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
				if err != nil {
					t.Errorf("name %v : get managed cluster error: %v", c.name, err)
				}
				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions,
					constants.ConditionManagedClusterImportSucceeded,
				)
				if condition != nil && condition.Status != c.expectedConditionStatus {
					t.Errorf("name %v : expect condition status %s, got %s",
						c.name, c.expectedConditionStatus, condition.Status)
				}
				if condition != nil && condition.Reason != c.expectedConditionReason {
					t.Errorf("name %v : expect condition reason %s, got %s, message: %s",
						c.name, c.expectedConditionReason, condition.Reason, condition.Message)
				}
				if c.expectedStaleAnnotation != nil {
					_, hasAnnotation := managedCluster.Annotations[constants.AnnotationStaleImportSuccess]
					if hasAnnotation != *c.expectedStaleAnnotation {
						t.Errorf("name %v : expect stale import success annotation %t, got %t",
							c.name, *c.expectedStaleAnnotation, hasAnnotation)
					}
				}
			}

		})
	}
}

// TestImportStatusKeepsWaitingConditionDuringReTrigger runs the autoimport controller and
// the importstatus controller through a waiting re-trigger of a stale import success: the
// importstatus controller must not replace the waiting condition with a success taken from
// the ManifestWork availability acknowledged by the agent of the previous cluster
// incarnation while the stale import success annotation is present.
func TestImportStatusKeepsWaitingConditionDuringReTrigger(t *testing.T) {
	apiServer := &envtest.Environment{}
	config, err := apiServer.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer apiServer.Stop()

	managedClusterName := "test"
	staleTransitionTime := metav1.NewTime(time.Now().Add(-time.Hour))

	// the managed cluster has an import success recorded an hour ago by the agent of the
	// previous cluster incarnation, and the klusterlet ManifestWorks are still Available
	// from that agent's acknowledgements
	managedCluster := func() *clusterv1.ManagedCluster {
		mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
			WithImportedCondition(true).Build()
		condition := meta.FindStatusCondition(
			mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
		condition.LastTransitionTime = staleTransitionTime
		return mc
	}()

	works := []runtime.Object{
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-klusterlet-crds",
				Namespace: managedClusterName,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue},
				},
			},
		},
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-klusterlet",
				Namespace: managedClusterName,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue},
				},
			},
		},
	}

	// a new auto import secret requests the re-import, but the import secret is not
	// created yet, so the re-triggered import waits before applying resources
	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "auto-import-secret",
				Namespace:         managedClusterName,
				CreationTimestamp: metav1.Now(),
			},
			Data: map[string][]byte{
				"kubeconfig": testinghelpers.BuildKubeconfig(config),
			},
			Type: constants.AutoImportSecretKubeConfig,
		},
	}

	kubeClient := kubefake.NewSimpleClientset(secrets...)
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
	for _, secret := range secrets {
		secretInformer.GetStore().Add(secret)
	}

	workClient := workfake.NewSimpleClientset(works...)
	workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
	workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
	for _, work := range works {
		workInformer.GetStore().Add(work)
	}

	ctx := context.TODO()
	importConfigLister := testinghelpers.FakeImportControllerConfigLister("test", apiconstants.AutoImportStrategyImportOnly, "")
	runtimeClient := fake.NewClientBuilder().WithScheme(testscheme).
		WithObjects(managedCluster).WithStatusSubresource(managedCluster).Build()
	mcRecorder := helpers.NewManagedClusterEventRecorder(ctx, kubeClient)

	r := NewReconcileAutoImport(
		runtimeClient,
		kubeClient,
		&source.InformerHolder{
			AutoImportSecretLister: kubeInformerFactory.Core().V1().Secrets().Lister(),
			ImportSecretLister:     kubeInformerFactory.Core().V1().Secrets().Lister(),
			KlusterletWorkLister:   workInformerFactory.Work().V1().ManifestWorks().Lister(),
		},
		eventstesting.NewTestingEventRecorder(t),
		mcRecorder,
		helpers.NewImportControllerConfig("test", importConfigLister,
			logf.Log.WithName("fake-import-controller-config")),
	)

	// the re-triggered import waits for the import secret: it records the stale import
	// success annotation and writes the waiting condition. Run both controllers through
	// several reconciliations: the import status controller reacts to each condition
	// update and must not replace the waiting condition with a success taken from the
	// stale ManifestWork availability.
	importStatusReconciler := importstatus.NewReconcileImportStatus(
		runtimeClient, kubeClient, workClient, mcRecorder)
	for i := 0; i < 3; i++ {
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}})
		if err != nil {
			t.Fatalf("unexpected error from the autoimport reconcile (round %d): %v", i, err)
		}
		_, err = importStatusReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}})
		if err != nil {
			t.Fatalf("unexpected error from the importstatus reconcile (round %d): %v", i, err)
		}

		result := &clusterv1.ManagedCluster{}
		if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
			t.Fatalf("get managed cluster error (round %d): %v", i, err)
		}

		condition := meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
		if condition == nil {
			t.Fatalf("the import condition is missing (round %d)", i)
		}
		if condition.Status != metav1.ConditionFalse || condition.Reason != constants.ConditionReasonManagedClusterImporting {
			t.Errorf("round %d: expect the waiting condition to be kept, got status %s reason %s message %s",
				i, condition.Status, condition.Reason, condition.Message)
		}
		if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; !hasAnnotation {
			t.Errorf("round %d: expect the stale import success annotation to be present", i)
		}
	}
}

// TestReTriggerKeepsAnnotationWhenPostApplyCleanupFails runs the autoimport controller and
// the importstatus controller through a re-trigger whose import applies resources but whose
// post-apply cleanup fails: the stale import success annotation must be kept, so the retry
// state survives the failure and the import status controller does not restore a success
// from the stale ManifestWork availability.
func TestReTriggerKeepsAnnotationWhenPostApplyCleanupFails(t *testing.T) {
	apiServer := &envtest.Environment{}
	config, err := apiServer.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer apiServer.Stop()

	// the spoke side: the bootstrap secret that the restore import path updates, so the
	// import reaches the apply stage and succeeds in this test
	spokeKubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spokeKubeClient.CoreV1().Namespaces().Create(
		context.TODO(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-agent"}},
		metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := spokeKubeClient.CoreV1().Secrets("open-cluster-management-agent").Create(
		context.TODO(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-hub-kubeconfig"},
			Data:       map[string][]byte{"kubeconfig": []byte("dumb")},
		},
		metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	managedClusterName := "test"
	staleTransitionTime := metav1.NewTime(time.Now().Add(-time.Hour))

	// the managed cluster has an import success recorded an hour ago by the agent of the
	// previous cluster incarnation, and the klusterlet ManifestWorks are still Available
	// from that agent's acknowledgements
	managedCluster := func() *clusterv1.ManagedCluster {
		mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
			WithImportedCondition(true).Build()
		condition := meta.FindStatusCondition(
			mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
		condition.LastTransitionTime = staleTransitionTime
		return mc
	}()

	works := []runtime.Object{
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-klusterlet-crds",
				Namespace: managedClusterName,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue},
				},
			},
		},
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-klusterlet",
				Namespace: managedClusterName,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue},
				},
			},
		},
	}

	// a restore-labeled auto import secret newer than the stale import success triggers
	// the re-import; the restore import path only updates the bootstrap secret, so the
	// import applies resources successfully and reaches the post-apply cleanup
	secrets := []runtime.Object{
		testinghelpers.GetImportSecret(managedClusterName),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "auto-import-secret",
				Namespace: managedClusterName,
				Labels: map[string]string{
					constants.LabelAutoImportRestore: "true",
				},
				CreationTimestamp: metav1.Now(),
			},
			Data: map[string][]byte{
				"kubeconfig": testinghelpers.BuildKubeconfig(config),
			},
			Type: constants.AutoImportSecretKubeConfig,
		},
	}

	kubeClient := kubefake.NewSimpleClientset(secrets...)
	// make the auto import secret deletion fail, so the post-apply cleanup fails
	kubeClient.PrependReactor("delete", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("deletion not allowed")
	})
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
	for _, secret := range secrets {
		secretInformer.GetStore().Add(secret)
	}

	workClient := workfake.NewSimpleClientset(works...)
	workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
	workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
	for _, work := range works {
		workInformer.GetStore().Add(work)
	}

	ctx := context.TODO()
	importConfigLister := testinghelpers.FakeImportControllerConfigLister("test", apiconstants.AutoImportStrategyImportOnly, "")
	runtimeClient := fake.NewClientBuilder().WithScheme(testscheme).
		WithObjects(managedCluster).WithStatusSubresource(managedCluster).Build()
	mcRecorder := helpers.NewManagedClusterEventRecorder(ctx, kubeClient)

	r := NewReconcileAutoImport(
		runtimeClient,
		kubeClient,
		&source.InformerHolder{
			AutoImportSecretLister: kubeInformerFactory.Core().V1().Secrets().Lister(),
			ImportSecretLister:     kubeInformerFactory.Core().V1().Secrets().Lister(),
			KlusterletWorkLister:   workInformerFactory.Work().V1().ManifestWorks().Lister(),
		},
		eventstesting.NewTestingEventRecorder(t),
		mcRecorder,
		helpers.NewImportControllerConfig("test", importConfigLister,
			logf.Log.WithName("fake-import-controller-config")),
	)

	// the re-triggered import applies resources, but the post-apply cleanup fails on the
	// auto import secret deletion
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}})
	if err == nil {
		t.Fatal("expected the error from the failed post-apply cleanup")
	}

	// the import status controller must not restore a success from the stale
	// ManifestWork availability while the retry state is kept
	importStatusReconciler := importstatus.NewReconcileImportStatus(
		runtimeClient, kubeClient, workClient, mcRecorder)
	if _, err = importStatusReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}); err != nil {
		t.Fatalf("unexpected error from the importstatus reconcile: %v", err)
	}

	result := &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}

	condition := meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if condition == nil {
		t.Fatal("the import condition is missing")
	}
	if condition.Status != metav1.ConditionFalse {
		t.Errorf("expect the importing condition to be kept, got status %s reason %s message %s",
			condition.Status, condition.Reason, condition.Message)
	}
	if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; !hasAnnotation {
		t.Error("expect the stale import success annotation to be kept when the post-apply cleanup fails")
	}
}

// TestReTriggerRecoversAnnotationRemovalFailureAfterSecretDeletion runs a re-trigger
// whose import applies resources and deletes the auto import secret, but whose stale
// import success annotation removal fails: a later reconcile must recover by removing
// the annotation once the secret is gone, so the success status is not blocked forever.
func TestReTriggerRecoversAnnotationRemovalFailureAfterSecretDeletion(t *testing.T) {
	apiServer := &envtest.Environment{}
	config, err := apiServer.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer apiServer.Stop()

	// the spoke side: the bootstrap secret that the restore import path updates, so the
	// import reaches the apply stage and succeeds in this test
	spokeKubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spokeKubeClient.CoreV1().Namespaces().Create(
		context.TODO(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-agent"}},
		metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := spokeKubeClient.CoreV1().Secrets("open-cluster-management-agent").Create(
		context.TODO(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-hub-kubeconfig"},
			Data:       map[string][]byte{"kubeconfig": []byte("dumb")},
		},
		metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	managedClusterName := "test"
	staleTransitionTime := metav1.NewTime(time.Now().Add(-time.Hour))

	// the managed cluster has an import success recorded an hour ago by the agent of the
	// previous cluster incarnation
	managedCluster := func() *clusterv1.ManagedCluster {
		mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
			WithImportedCondition(true).Build()
		condition := meta.FindStatusCondition(
			mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
		condition.LastTransitionTime = staleTransitionTime
		return mc
	}()

	works := []runtime.Object{
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-klusterlet-crds",
				Namespace: managedClusterName,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue},
				},
			},
		},
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-klusterlet",
				Namespace: managedClusterName,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue},
				},
			},
		},
	}

	// a restore-labeled auto import secret newer than the stale import success triggers
	// the re-import; the restore import path only updates the bootstrap secret, so the
	// import applies resources successfully and reaches the post-apply cleanup
	autoImportSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-import-secret",
			Namespace: managedClusterName,
			Labels: map[string]string{
				constants.LabelAutoImportRestore: "true",
			},
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string][]byte{
			"kubeconfig": testinghelpers.BuildKubeconfig(config),
		},
		Type: constants.AutoImportSecretKubeConfig,
	}
	secrets := []runtime.Object{
		testinghelpers.GetImportSecret(managedClusterName),
		autoImportSecret,
	}

	kubeClient := kubefake.NewSimpleClientset(secrets...)
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
	for _, secret := range secrets {
		secretInformer.GetStore().Add(secret)
	}

	workClient := workfake.NewSimpleClientset(works...)
	workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
	workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
	for _, work := range works {
		workInformer.GetStore().Add(work)
	}

	ctx := context.TODO()
	importConfigLister := testinghelpers.FakeImportControllerConfigLister("test", apiconstants.AutoImportStrategyImportOnly, "")

	// make the stale import success annotation removal fail once, after the auto import
	// secret is deleted: the removal patch targets a managed cluster without the
	// annotation, while the initial annotation set targets one with it
	annotationRemovalFailed := false
	runtimeClient := fake.NewClientBuilder().WithScheme(testscheme).
		WithObjects(managedCluster).WithStatusSubresource(managedCluster).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if mc, ok := obj.(*clusterv1.ManagedCluster); ok && !annotationRemovalFailed {
					if _, has := mc.Annotations[constants.AnnotationStaleImportSuccess]; !has {
						annotationRemovalFailed = true
						return fmt.Errorf("patch not allowed")
					}
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	mcRecorder := helpers.NewManagedClusterEventRecorder(ctx, kubeClient)

	r := NewReconcileAutoImport(
		runtimeClient,
		kubeClient,
		&source.InformerHolder{
			AutoImportSecretLister: kubeInformerFactory.Core().V1().Secrets().Lister(),
			ImportSecretLister:     kubeInformerFactory.Core().V1().Secrets().Lister(),
			KlusterletWorkLister:   workInformerFactory.Work().V1().ManifestWorks().Lister(),
		},
		eventstesting.NewTestingEventRecorder(t),
		mcRecorder,
		helpers.NewImportControllerConfig("test", importConfigLister,
			logf.Log.WithName("fake-import-controller-config")),
	)

	// the re-triggered import applies resources and deletes the auto import secret, but
	// the annotation removal fails. The first reconcile can also conflict on the cluster
	// spec update (the annotation patch and the condition update bump the resource
	// version of the stored object), which self-heals on the retry, so drive the
	// re-trigger until the annotation removal has failed.
	for i := 0; i < 3; i++ {
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}})
		if err == nil {
			t.Fatalf("expected the error from the failed annotation removal (round %d)", i)
		}
		if annotationRemovalFailed {
			break
		}
	}
	if !annotationRemovalFailed {
		t.Fatal("the annotation removal did not fail")
	}

	result := &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; !hasAnnotation {
		t.Fatal("expect the stale import success annotation to be present after the failed removal")
	}
	if _, err := kubeClient.CoreV1().Secrets(managedClusterName).Get(
		ctx, "auto-import-secret", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expect the auto import secret to be deleted, got %v", err)
	}

	// the watch delivers the secret deletion; a later reconcile recovers by removing the
	// annotation, so the success status is not blocked forever
	secretInformer.GetStore().Delete(autoImportSecret)
	if _, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}); err != nil {
		t.Fatalf("unexpected error from the recovery reconcile: %v", err)
	}

	result = &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; hasAnnotation {
		t.Error("expect the stale import success annotation to be removed by the recovery")
	}

	// with the annotation removed, the import status controller reports the success
	importStatusReconciler := importstatus.NewReconcileImportStatus(
		runtimeClient, kubeClient, workClient, mcRecorder)
	if _, err = importStatusReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}); err != nil {
		t.Fatalf("unexpected error from the importstatus reconcile: %v", err)
	}

	result = &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	condition := meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if condition == nil || condition.Status != metav1.ConditionTrue ||
		condition.Reason != constants.ConditionReasonManagedClusterImported {
		t.Errorf("expect the success condition to be reported after the recovery, got %+v", condition)
	}
}
