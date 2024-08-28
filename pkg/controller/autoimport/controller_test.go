// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package autoimport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
}

func TestReconcile(t *testing.T) {

	// uncomment these lines to run the test locally
	// if err := os.Setenv("KUBEBUILDER_ASSETS", "./../../../_output/kubebuilder/bin"); err != nil {
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
	cases := []struct {
		name                    string
		objs                    []client.Object
		works                   []runtime.Object
		secrets                 []runtime.Object
		expectedErr             bool
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
		expectedAPIServerURL    string
	}{
		{
			name:        "no cluster",
			objs:        []client.Object{},
			expectedErr: false,
		},
		{
			name: "hosted cluster without",
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
			works:                []runtime.Object{},
			secrets:              []runtime.Object{},
			expectedErr:          false,
			expectedAPIServerURL: "",
		},
		{
			name: "no auto-import-secret",
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
					Data: map[string][]byte{
						"autoImportRetry": []byte("0"),
						"server":          []byte(config.Host),
						// no auth info
					},
					Type: constants.AutoImportSecretKubeToken,
				},
			},
			expectedErr:          false,
			expectedAPIServerURL: strings.TrimSuffix(config.Host, "/"),
		},
		{
			name: "auto-import-secret AutoImportRetry invalid",
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
					Data: map[string][]byte{
						"autoImportRetry": []byte("a"),
					},
				},
			},
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
			expectedAPIServerURL:    "",
		},
		{
			name: "auto-import-secret current retry annotation invalid",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder(managedClusterName).Build(),
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: managedClusterName,
						Annotations: map[string]string{
							constants.AnnotationAutoImportCurrentRetry: "a",
						},
					},
					Data: map[string][]byte{
						"autoImportRetry": []byte("1"),
					},
				},
			},
			expectedErr:          true,
			expectedAPIServerURL: "",
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
			expectedAPIServerURL:    "",
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
						Annotations: map[string]string{
							constants.AnnotationAutoImportCurrentRetry: "1",
						},
					},
					Data: map[string][]byte{
						"autoImportRetry": []byte("2"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
						Annotations: map[string]string{
							constants.AnnotationAutoImportCurrentRetry: "1",
						},
					},
					Data: map[string][]byte{
						"autoImportRetry": []byte("2"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
					Type: corev1.SecretTypeOpaque,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
						"autoImportRetry": []byte("2"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
		},
		{
			name: "update auto import secret current retry times",
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
						"autoImportRetry": []byte("2"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
						"autoImportRetry": []byte("0"),
						"server":          []byte(config.Host),
						// no auth info
					},
					Type: constants.AutoImportSecretKubeToken,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
						"autoImportRetry": []byte("0"),
						"server":          []byte(config.Host),
						// no auth info
					},
					Type: corev1.SecretTypeOpaque,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
					Data: map[string][]byte{
						constants.AutoImportSecretRosaConfigAPIURLKey: []byte(config.Host),
					},
					Type: constants.AutoImportSecretRosaConfig,
				},
			},
			expectedErr:             true,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
			expectedAPIServerURL:    "",
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
						"autoImportRetry": []byte("0"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
						"autoImportRetry": []byte("0"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
						"autoImportRetry": []byte("0"),
						"kubeconfig":      testinghelpers.BuildKubeconfig(config),
					},
					Type: constants.AutoImportSecretKubeConfig,
				},
			},
			expectedErr:             false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
			expectedAPIServerURL:    strings.TrimSuffix(config.Host, "/"),
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
			)

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: managedClusterName}}
			_, err := r.Reconcile(ctx, req)
			if c.expectedErr && err == nil {
				t.Errorf("name: %v, expected error, but failed", c.name)
			}
			if !c.expectedErr && err != nil {
				t.Errorf("name: %v, unexpected error: %v", c.name, err)
			}

			managedCluster := &clusterv1.ManagedCluster{}
			err = r.client.Get(ctx, types.NamespacedName{Name: managedClusterName}, managedCluster)
			if err != nil && !errors.IsNotFound(err) {
				t.Errorf("name %v : get managed cluster error: %v", c.name, err)
			}

			if c.expectedConditionReason != "" {
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
			}
			if c.expectedAPIServerURL != "" {
				clientConfigs := managedCluster.Spec.ManagedClusterClientConfigs
				if len(clientConfigs) != 1 {
					t.Errorf("name %v : expect 1 clientConfigs, got %d", c.name, len(clientConfigs))
				}
				if clientConfigs[0].URL != c.expectedAPIServerURL {
					t.Errorf("expected apiserver URL %s, got %s", c.expectedAPIServerURL, clientConfigs[0].URL)
				}
			}

		})
	}
}
