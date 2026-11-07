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

	// Old ManifestWork is still Available, but ObservedGeneration lags the current
	// generation: that is a stale acknowledgment and must not report import success.
	works := []runtime.Object{
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet-crds",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
				},
			},
		},
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
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

	// Old ManifestWork Available with a lagging ObservedGeneration must not report success.
	works := []runtime.Object{
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet-crds",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
				},
			},
		},
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
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

// TestReTriggerWaitsForFreshManifestWorkAvailability verifies that after a re-trigger
// applies resources and deletes the auto import secret, the stale-import-success
// annotation stays set while the old ManifestWork is still Available with a lagging
// ObservedGeneration, and is cleared only once importstatus sees a current-generation
// acknowledgment and reports success.
func TestReTriggerWaitsForFreshManifestWorkAvailability(t *testing.T) {
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

	managedCluster := func() *clusterv1.ManagedCluster {
		mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
			WithImportedCondition(true).Build()
		condition := meta.FindStatusCondition(
			mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
		condition.LastTransitionTime = staleTransitionTime
		return mc
	}()

	staleWorks := []runtime.Object{
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet-crds",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
				},
			},
		},
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
				},
			},
		},
	}

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

	workClient := workfake.NewSimpleClientset(staleWorks...)
	workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
	workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
	for _, work := range staleWorks {
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

	// Annotation set + condition/URL updates can conflict on the managed cluster
	// resource version; retry until the auto import secret is deleted.
	for i := 0; i < 5; i++ {
		_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}})
		if _, getErr := kubeClient.CoreV1().Secrets(managedClusterName).Get(
			ctx, "auto-import-secret", metav1.GetOptions{}); apierrors.IsNotFound(getErr) {
			break
		}
	}
	if _, err := kubeClient.CoreV1().Secrets(managedClusterName).Get(
		ctx, "auto-import-secret", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expect the auto import secret to be deleted, got %v", err)
	}

	result := &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; !hasAnnotation {
		t.Fatal("expect the stale import success annotation to remain after apply")
	}
	condition := meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if condition == nil || !helpers.ImportingResourcesApplied(condition) {
		t.Fatalf("expect the applied condition after import, got %+v", condition)
	}

	importStatusReconciler := importstatus.NewReconcileImportStatus(
		runtimeClient, kubeClient, workClient, mcRecorder)

	if _, err = importStatusReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}); err != nil {
		t.Fatalf("unexpected error from the importstatus reconcile with stale availability: %v", err)
	}
	result = &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	condition = meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if condition == nil || condition.Status != metav1.ConditionFalse {
		t.Errorf("expect success to wait for a fresh ManifestWork acknowledgment, got %+v", condition)
	}
	if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; !hasAnnotation {
		t.Error("expect the stale import success annotation to remain while availability is stale")
	}

	for _, name := range []string{"test-klusterlet-crds", "test-klusterlet"} {
		work, err := workClient.WorkV1().ManifestWorks(managedClusterName).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get manifestwork %s: %v", name, err)
		}
		meta.SetStatusCondition(&work.Status.Conditions, metav1.Condition{
			Type:               workv1.WorkAvailable,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: work.Generation,
			Reason:             "Available",
			Message:            "fresh acknowledgment",
		})
		if _, err := workClient.WorkV1().ManifestWorks(managedClusterName).UpdateStatus(ctx, work, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("update manifestwork %s status: %v", name, err)
		}
	}

	if _, err = importStatusReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}); err != nil {
		t.Fatalf("unexpected error from the importstatus reconcile with fresh availability: %v", err)
	}

	result = &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	condition = meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if condition == nil || condition.Status != metav1.ConditionTrue ||
		condition.Reason != constants.ConditionReasonManagedClusterImported {
		t.Errorf("expect the success condition after a fresh acknowledgment, got %+v", condition)
	}
	if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; hasAnnotation {
		t.Error("expect the stale import success annotation to be cleared after fresh success")
	}
}

// TestReTriggerUpdatesConditionOnUnchangedApplyRetryAndKeepsAnnotation verifies that a
// re-trigger retry from a failed attempt still updates the condition when the apply is
// unchanged, deletes the auto import secret, and keeps the stale-import-success
// annotation until importstatus sees a fresh ManifestWork acknowledgment.
func TestReTriggerUpdatesConditionOnUnchangedApplyRetryAndKeepsAnnotation(t *testing.T) {
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

	crdManifest := []byte(`apiVersion: v1
kind: Secret
metadata:
  name: retrigger-apply-crd
  namespace: default
type: Opaque
data:
  key: dmFsdWU=
`)
	importManifest := []byte(`apiVersion: v1
kind: Secret
metadata:
  name: retrigger-apply-import
  namespace: default
type: Opaque
data:
  key: dmFsdWU=
`)
	for _, manifest := range [][]byte{crdManifest, importManifest} {
		secret := helpers.MustCreateObject(manifest).(*corev1.Secret)
		if _, err := spokeKubeClient.CoreV1().Secrets(secret.Namespace).Create(
			context.TODO(), secret, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	managedClusterName := "test"

	managedCluster := func() *clusterv1.ManagedCluster {
		mc := testinghelpers.NewManagedClusterBuilder(managedClusterName).
			WithImportedCondition(true).
			WithAnnotations(constants.AnnotationStaleImportSuccess, constants.LabelValueTrue).
			Build()
		condition := meta.FindStatusCondition(
			mc.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
		condition.Status = metav1.ConditionFalse
		condition.Reason = constants.ConditionReasonManagedClusterImportFailed
		condition.Message = "failed to import"
		return mc
	}()

	works := []runtime.Object{
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet-crds",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
				},
			},
		},
		&workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-klusterlet",
				Namespace:  managedClusterName,
				Generation: 2,
				Labels: map[string]string{
					constants.KlusterletWorksLabel: "true",
				},
			},
			Status: workv1.ManifestWorkStatus{
				Conditions: []metav1.Condition{
					{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue, ObservedGeneration: 1},
				},
			},
		},
	}

	autoImportSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "auto-import-secret",
			Namespace:         managedClusterName,
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string][]byte{
			"kubeconfig": testinghelpers.BuildKubeconfig(config),
		},
		Type: constants.AutoImportSecretKubeConfig,
	}
	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-import",
				Namespace: managedClusterName,
			},
			Data: map[string][]byte{
				constants.ImportSecretCRDSYamlKey:   crdManifest,
				constants.ImportSecretImportYamlKey: append([]byte("---\n"), importManifest...),
			},
		},
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

	for i := 0; i < 5; i++ {
		_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}})
		if _, getErr := kubeClient.CoreV1().Secrets(managedClusterName).Get(
			ctx, "auto-import-secret", metav1.GetOptions{}); apierrors.IsNotFound(getErr) {
			break
		}
	}

	result := &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	condition := meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if condition == nil || !helpers.ImportingResourcesApplied(condition) {
		t.Errorf("expect the applied condition to be persisted on the unchanged apply retry, got %+v", condition)
	}
	if _, hasAnnotation := result.Annotations[constants.AnnotationStaleImportSuccess]; !hasAnnotation {
		t.Fatal("expect the stale import success annotation to remain after the applied import")
	}
	if _, err := kubeClient.CoreV1().Secrets(managedClusterName).Get(
		ctx, "auto-import-secret", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expect the auto import secret to be deleted, got %v", err)
	}

	importStatusReconciler := importstatus.NewReconcileImportStatus(
		runtimeClient, kubeClient, workClient, mcRecorder)
	if _, err = importStatusReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}); err != nil {
		t.Fatalf("unexpected error from the importstatus reconcile with stale availability: %v", err)
	}
	result = &clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: managedClusterName}, result); err != nil {
		t.Fatalf("get managed cluster error: %v", err)
	}
	condition = meta.FindStatusCondition(result.Status.Conditions, constants.ConditionManagedClusterImportSucceeded)
	if condition == nil || condition.Status != metav1.ConditionFalse {
		t.Errorf("expect success to wait for a fresh ManifestWork acknowledgment, got %+v", condition)
	}
}

