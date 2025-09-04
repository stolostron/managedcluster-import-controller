// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusterdeployment

import (
	"context"
	"testing"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})
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

	cases := []struct {
		name                    string
		objs                    []client.Object
		works                   []runtime.Object
		secrets                 []runtime.Object
		autoImportStrategy      string
		expectedErr             bool
		expectedConditionReason string
	}{
		{
			name:    "no clusterdeployment",
			objs:    []client.Object{},
			works:   []runtime.Object{},
			secrets: []runtime.Object{},
		},
		{
			name: "no cluster",
			objs: []client.Object{
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
					},
				},
			},
			works:   []runtime.Object{},
			secrets: []runtime.Object{},
		},
		{
			name: "auto import disabled",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").
					WithAnnotations(apiconstants.DisableAutoImportAnnotation, "").Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
					},
				},
			},
		},
		{
			name: "with ImportOnly strategy",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").
					WithImportedCondition(true).Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
					},
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             false,
			expectedConditionReason: constants.ConditionReasonManagedClusterImported,
		},
		{
			name: "with ImportOnly strategy and immediate-import annotation",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").
					WithAnnotations(apiconstants.AnnotationImmediateImport, "").
					WithImportedCondition(true).Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
						ClusterMetadata: &hivev1.ClusterMetadata{
							AdminKubeconfigSecretRef: corev1.LocalObjectReference{
								Name: "test",
							},
						},
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token":  []byte(config.BearerToken),
						"server": []byte(config.Host),
					},
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             true,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name: "with ImportOnly strategy and unempty immediate-import annotation",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").
					WithAnnotations(apiconstants.AnnotationImmediateImport, "Completed").
					WithImportedCondition(true).Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
					},
				},
			},
			autoImportStrategy:      apiconstants.AutoImportStrategyImportOnly,
			expectedErr:             false,
			expectedConditionReason: constants.ConditionReasonManagedClusterImported,
		},
		{
			name: "clusterdeployment is not installed",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			works:   []runtime.Object{},
			secrets: []runtime.Object{},
		},
		{
			name: "clusterdeployment is not claimed",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed:      true,
						ClusterPoolRef: &hivev1.ClusterPoolReference{},
					},
				},
			},
			works:   []runtime.Object{},
			secrets: []runtime.Object{},
		},
		{
			name: "import cluster with auto-import secret",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
					},
				},
			},
			works: []runtime.Object{},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "auto-import-secret",
						Namespace: "test",
					},
				},
			},
		},
		{
			name: "import cluster with clusterdeployment secret",
			objs: []client.Object{
				testinghelpers.NewManagedClusterBuilder("test").Build(),
				&hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: hivev1.ClusterDeploymentSpec{
						Installed: true,
						ClusterMetadata: &hivev1.ClusterMetadata{
							AdminKubeconfigSecretRef: corev1.LocalObjectReference{
								Name: "test",
							},
						},
					},
				},
			},
			works: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet-crds",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-klusterlet",
						Namespace: "test",
						Labels: map[string]string{
							constants.KlusterletWorksLabel: "true",
						},
					},
				},
			},
			secrets: []runtime.Object{
				testinghelpers.GetImportSecret("test"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"token":  []byte(config.BearerToken),
						"server": []byte(config.Host),
					},
				},
			},
			expectedErr:             true,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
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

			workClient := workfake.NewSimpleClientset()
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			ctx := context.TODO()
			importConfigLister := testinghelpers.FakeImportControllerConfigLister("test", c.autoImportStrategy, "")
			r := NewReconcileClusterDeployment(
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

			_, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}})
			if c.expectedErr && err == nil {
				t.Errorf("name: %v, expected error, but failed", c.name)
			}
			if !c.expectedErr && err != nil {
				t.Errorf("name: %v, unexpected error: %v", c.name, err)
			}

			if c.expectedConditionReason != "" {
				managedCluster := &clusterv1.ManagedCluster{}
				err = r.client.Get(context.TODO(), types.NamespacedName{Name: "test"}, managedCluster)
				if err != nil {
					t.Errorf("name %v : get managed cluster error: %v", c.name, err)
				}
				condition := meta.FindStatusCondition(
					managedCluster.Status.Conditions,
					constants.ConditionManagedClusterImportSucceeded,
				)

				if condition != nil && condition.Reason != c.expectedConditionReason {
					t.Errorf("name %v : expect condition reason %s, got %s, message: %s",
						c.name, c.expectedConditionReason, condition.Reason, condition.Message)
				}
			}
		})
	}
}
