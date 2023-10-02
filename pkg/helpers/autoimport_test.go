// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package helpers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
)

func TestDeleteAutoImportSecret(t *testing.T) {
	cases := []struct {
		name             string
		autoImportSecret *corev1.Secret
		expectedErr      bool
		verifyFunc       func(t *testing.T, kubeClinent kubernetes.Interface)
	}{
		{
			name: "delete the secret succeeded",
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"autoImportRetry": []byte("0"),
				},
			},
			verifyFunc: func(t *testing.T, kubeClinent kubernetes.Interface) {
				_, err := kubeClinent.CoreV1().Secrets("test").Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
				if !errors.IsNotFound(err) {
					t.Errorf("unexpect err %v", err)
				}
			},
		},
		{
			name: "delete the secret failed",
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: "test",
					Annotations: map[string]string{
						constants.AnnotationKeepingAutoImportSecret: "",
					},
				},
				Data: map[string][]byte{
					"autoImportRetry": []byte("0"),
				},
			},
			verifyFunc: func(t *testing.T, kubeClinent kubernetes.Interface) {
				_, err := kubeClinent.CoreV1().Secrets("test").Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpect err %v", err)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset(c.autoImportSecret)

			err := DeleteAutoImportSecret(context.TODO(), kubeClient, c.autoImportSecret, eventstesting.NewTestingEventRecorder(t))
			if !c.expectedErr && err != nil {
				t.Errorf("unexpected err %v", err)
			}

			c.verifyFunc(t, kubeClient)
		})
	}
}

func TestImportHelper(t *testing.T) {

	// if err := os.Setenv("KUBEBUILDER_ASSETS", "./../../_output/kubebuilder/bin"); err != nil { // uncomment these lines to run the test locally
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
		autoImportSecret        *corev1.Secret
		importSecret            *corev1.Secret
		works                   []runtime.Object
		lastRetry               int
		totalRetry              int
		expectedErr             bool
		expectedCurrentRetry    int
		expectedRequeueAfter    time.Duration
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
	}{
		{
			name:       "no manifest works",
			lastRetry:  0,
			totalRetry: 1,
			works:      []runtime.Object{},
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: managedClusterName,
				},
				Data: map[string][]byte{
					"autoImportRetry": []byte("2"),
					"kubeconfig":      testinghelpers.BuildKubeconfig(config),
				},
			},
			importSecret:            testinghelpers.GetImportSecret(managedClusterName),
			expectedErr:             false,
			expectedCurrentRetry:    0,
			expectedRequeueAfter:    3 * time.Second,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name:       "no import-secret",
			lastRetry:  0,
			totalRetry: 1,
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
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: managedClusterName,
				},
				Data: map[string][]byte{
					"kubeconfig": testinghelpers.BuildKubeconfig(config),
				},
			},
			expectedErr:             false,
			expectedCurrentRetry:    0,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name:       "update auto import secret current retry times",
			lastRetry:  1,
			totalRetry: 3,
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
			importSecret: testinghelpers.GetImportSecret(managedClusterName),
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: managedClusterName,
				},
				Data: map[string][]byte{
					"kubeconfig": testinghelpers.BuildKubeconfig(config),
				},
			},
			expectedErr:             false,
			expectedCurrentRetry:    2,
			expectedRequeueAfter:    10 * time.Second,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name:       "import cluster with auto-import secret invalid",
			lastRetry:  0,
			totalRetry: 1,
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
			importSecret: testinghelpers.GetImportSecret(managedClusterName),
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: managedClusterName,
				},
				Data: map[string][]byte{
					"server": []byte(config.Host),
					// no auth info
				},
			},
			expectedErr:             false,
			expectedCurrentRetry:    0,
			expectedRequeueAfter:    0 * time.Second,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImportFailed,
		},
		{
			name:       "only update the bootstrap secret",
			lastRetry:  0,
			totalRetry: 1,
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
			importSecret: testinghelpers.GetImportSecret(managedClusterName),
			autoImportSecret: &corev1.Secret{
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
			},
			expectedErr:             false,
			expectedCurrentRetry:    1,
			expectedRequeueAfter:    0 * time.Second,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
		{
			name:       "import cluster with auto-import secret valid",
			lastRetry:  0,
			totalRetry: 1,
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
			importSecret: testinghelpers.GetImportSecret(managedClusterName),
			autoImportSecret: &corev1.Secret{
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
			},
			expectedErr:             false,
			expectedCurrentRetry:    1,
			expectedRequeueAfter:    0,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: constants.ConditionReasonManagedClusterImporting,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			secrets := []runtime.Object{}
			for _, s := range []*corev1.Secret{c.autoImportSecret, c.importSecret} {
				if s != nil {
					secrets = append(secrets, s)
				}
			}
			kubeClient := kubefake.NewSimpleClientset(secrets...)
			kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
			secretInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
			for _, secret := range secrets {
				secretInformer.GetStore().Add(secret)
			}

			workClient := workfake.NewSimpleClientset(c.works...)
			workInformerFactory := workinformers.NewSharedInformerFactory(workClient, 10*time.Minute)
			workInformer := workInformerFactory.Work().V1().ManifestWorks().Informer()
			for _, work := range c.works {
				workInformer.GetStore().Add(work)
			}

			importHelper := NewImportHelper(&source.InformerHolder{
				ImportSecretLister:   kubeInformerFactory.Core().V1().Secrets().Lister(),
				KlusterletWorkLister: workInformerFactory.Work().V1().ManifestWorks().Lister(),
			}, eventstesting.NewTestingEventRecorder(t), logf.Log.WithName("import-helper-tester"))

			backupRestore := false
			if c.autoImportSecret != nil {
				if val, ok := c.autoImportSecret.Labels[constants.LabelAutoImportRestore]; ok &&
					strings.EqualFold(val, "true") {
					backupRestore = true
				}
			}

			result, condition, _, currentRetry, err := importHelper.Import(
				backupRestore, managedClusterName, c.autoImportSecret, c.lastRetry, c.totalRetry)
			if c.expectedErr && err == nil {
				t.Errorf("name %v : expected error, but failed", c.name)
			}
			if !c.expectedErr && err != nil {
				t.Errorf("name %v :unexpected error: %v", c.name, err)
			}
			if result.RequeueAfter != c.expectedRequeueAfter {
				t.Errorf("name %v : expected requeueAfter %v, but got %v",
					c.name, c.expectedRequeueAfter, result.RequeueAfter)
			}

			if c.expectedConditionReason != "" {
				if condition.Status != c.expectedConditionStatus {
					t.Errorf("name %v : expect condition status %s, got %s",
						c.name, c.expectedConditionStatus, condition.Status)
				}
				if condition.Reason != c.expectedConditionReason {
					t.Errorf("name %v : expect condition reason %s, got %s, message: %s",
						c.name, c.expectedConditionReason, condition.Reason, condition.Message)
				}
			}

			if currentRetry != c.expectedCurrentRetry {
				t.Errorf("name %v : expected currentRetry %v, but got %v", c.name, c.expectedCurrentRetry, currentRetry)
			}

		})
	}
}
