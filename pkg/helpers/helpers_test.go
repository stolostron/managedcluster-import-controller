// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	clustersmgmttesting "github.com/openshift-online/ocm-sdk-go/testing"
	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crdv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/diff"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	operatorfake "open-cluster-management.io/api/client/operator/clientset/versioned/fake"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	testinghelpers "github.com/stolostron/managedcluster-import-controller/pkg/helpers/testing"
)

var testscheme = scheme.Scheme

func init() {
	testscheme.AddKnownTypes(clusterv1.GroupVersion, &clusterv1.ManagedCluster{})
	testscheme.AddKnownTypes(operatorv1.GroupVersion, &operatorv1.Klusterlet{})
	testscheme.AddKnownTypes(addonv1alpha1.GroupVersion, &addonv1alpha1.ManagedClusterAddOn{})
	testscheme.AddKnownTypes(addonv1alpha1.GroupVersion, &addonv1alpha1.ManagedClusterAddOnList{})
	testscheme.AddKnownTypes(crdv1beta1.SchemeGroupVersion, &crdv1beta1.CustomResourceDefinition{})
	testscheme.AddKnownTypes(crdv1.SchemeGroupVersion, &crdv1.CustomResourceDefinition{})
}

func TestGetMaxConcurrentReconciles(t *testing.T) {
	os.Setenv(maxConcurrentReconcilesEnvVarName, "invalid")
	defer os.Unsetenv(maxConcurrentReconcilesEnvVarName)

	reconciles := GetMaxConcurrentReconciles()
	if reconciles != 1 {
		t.Errorf("expected 1, but failed")
	}
}

func TestGenerateClientFromSecret(t *testing.T) {

	// if err := os.Setenv("KUBEBUILDER_ASSETS", "./../../_output/kubebuilder/bin"); err != nil { // uncomment these lines to run the test locally
	// 	t.Fatal(err)
	// }

	// This line prevents controller-runtime from complaining about log.SetLogger never being called
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	apiServer := &envtest.Environment{}
	config, err := apiServer.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer apiServer.Stop()

	cases := []struct {
		name           string
		generateSecret func(server string, config *rest.Config) *corev1.Secret
		expectedErr    string
	}{
		{
			name: "no client config",
			generateSecret: func(server string, config *rest.Config) *corev1.Secret {
				return &corev1.Secret{
					Data: map[string][]byte{
						"test": {},
					},
				}
			},
			expectedErr: "kubeconfig or token and server are missing",
		},
		{
			name: "using kubeconfig",
			generateSecret: func(server string, config *rest.Config) *corev1.Secret {
				apiConfig := createBasic(server, "test", config.CAData, config.KeyData, config.CertData)
				bconfig, err := clientcmd.Write(*apiConfig)
				if err != nil {
					t.Fatal(err)
				}
				return &corev1.Secret{
					Data: map[string][]byte{
						"kubeconfig": bconfig,
					},
				}
			},
		},
		{
			name: "using token",
			generateSecret: func(server string, config *rest.Config) *corev1.Secret {
				return &corev1.Secret{
					Data: map[string][]byte{
						// config.BearerToken is empty
						// TODO: find a way to set config.BearerToken
						"token":  []byte(config.BearerToken),
						"server": []byte(server),
					},
				}
			},
			expectedErr: "unknown",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			secret := c.generateSecret(config.Host, config)
			_, _, _, err = GenerateImportClientFromKubeConfigSecret(secret)
			if c.expectedErr != "" && err == nil {
				t.Errorf("expected error, but failed")
			}
			if c.expectedErr == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpdateManagedClusterImportCondition(t *testing.T) {
	cases := []struct {
		name                 string
		managedCluster       *clusterv1.ManagedCluster
		cond                 metav1.Condition
		expectedErr          string
		validateAddonActions func(t *testing.T, actions []clienttesting.Action)
	}{
		{
			name: "invalid condition type",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    "test",
				Status:  metav1.ConditionTrue,
				Message: "test",
				Reason:  "test",
			},
			expectedErr: "the condition type test is not supported",
		},
		{
			name: "invalid condition reason",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    constants.ConditionManagedClusterImportSucceeded,
				Status:  metav1.ConditionTrue,
				Message: "test",
				Reason:  "test",
			},
			expectedErr: "the condition reason test is not supported",
		},
		{
			name: "cluster importing event",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    constants.ConditionManagedClusterImportSucceeded,
				Status:  metav1.ConditionFalse,
				Message: "test",
				Reason:  constants.ConditionReasonManagedClusterImporting,
			},
			validateAddonActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 1 {
					t.Errorf("expected 1 action, but got %d", len(actions))
				}
				action := actions[0]
				if action.GetVerb() != "create" {
					t.Errorf("expected create action, but got %s", action.GetVerb())
				}
				if action.GetResource().Resource != "events" {
					t.Errorf("expected events resource, but got %s", action.GetResource().Resource)
				}
			},
		},
		{
			name: "cluster imported event",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    constants.ConditionManagedClusterImportSucceeded,
				Status:  metav1.ConditionFalse,
				Message: "test",
				Reason:  constants.ConditionReasonManagedClusterImported,
			},
			validateAddonActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 1 {
					t.Errorf("expected 1 action, but got %d", len(actions))
				}
				action := actions[0]
				if action.GetVerb() != "create" {
					t.Errorf("expected create action, but got %s", action.GetVerb())
				}
				if action.GetResource().Resource != "events" {
					t.Errorf("expected events resource, but got %s", action.GetResource().Resource)
				}
			},
		},
		{
			name: "cluster importing failed",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    constants.ConditionManagedClusterImportSucceeded,
				Status:  metav1.ConditionFalse,
				Message: "test",
				Reason:  constants.ConditionReasonManagedClusterImportFailed,
			},
			validateAddonActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 1 {
					t.Errorf("expected 1 action, but got %d", len(actions))
				}
				action := actions[0]
				if action.GetVerb() != "create" {
					t.Errorf("expected create action, but got %s", action.GetVerb())
				}
				if action.GetResource().Resource != "events" {
					t.Errorf("expected events resource, but got %s", action.GetResource().Resource)
				}
			},
		},
		{
			name: "cluster is being detached",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    constants.ConditionManagedClusterImportSucceeded,
				Status:  metav1.ConditionFalse,
				Message: "test",
				Reason:  constants.ConditionReasonManagedClusterDetaching,
			},
			validateAddonActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 1 {
					t.Errorf("expected 1 action, but got %d", len(actions))
				}
				action := actions[0]
				if action.GetVerb() != "create" {
					t.Errorf("expected create action, but got %s", action.GetVerb())
				}
				if action.GetResource().Resource != "events" {
					t.Errorf("expected events resource, but got %s", action.GetResource().Resource)
				}
			},
		},
		{
			name: "cluster is being forcibly detached",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    constants.ConditionManagedClusterImportSucceeded,
				Status:  metav1.ConditionFalse,
				Message: "test",
				Reason:  constants.ConditionReasonManagedClusterForceDetaching,
			},
			validateAddonActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 1 {
					t.Errorf("expected 1 action, but got %d", len(actions))
				}
				action := actions[0]
				if action.GetVerb() != "create" {
					t.Errorf("expected create action, but got %s", action.GetVerb())
				}
				if action.GetResource().Resource != "events" {
					t.Errorf("expected events resource, but got %s", action.GetResource().Resource)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(testscheme).
				WithObjects(c.managedCluster).WithStatusSubresource(c.managedCluster).Build()
			kubeClient := kubefake.NewSimpleClientset()

			ctx := context.TODO()
			recorder := NewManagedClusterEventRecorder(ctx, kubeClient)

			err := UpdateManagedClusterImportCondition(fakeClient, c.managedCluster, c.cond, recorder)
			if len(c.expectedErr) > 0 {
				if err == nil {
					t.Errorf("expected error %s, but got nil", c.expectedErr)
				}
				if err.Error() != c.expectedErr {
					t.Errorf("expected error %s, but got %s", c.expectedErr, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if c.validateAddonActions != nil {
				// under the hood, the events are created asynchronously, so we need to wait a bit
				time.Sleep(1 * time.Second)
				c.validateAddonActions(t, kubeClient.Actions())
			}
		})
	}
}

func TestUpdateManagedClusterStatus(t *testing.T) {
	cases := []struct {
		name           string
		managedCluster *clusterv1.ManagedCluster
		cond           metav1.Condition
	}{
		{
			name: "add condition",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			cond: metav1.Condition{
				Type:    "test",
				Status:  metav1.ConditionTrue,
				Message: "test",
				Reason:  "test",
			},
		},
		{
			name: "update an existing condition",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "test",
							Status:  metav1.ConditionTrue,
							Message: "test",
							Reason:  "test",
						},
					},
				},
			},
			cond: metav1.Condition{
				Type:    "test",
				Status:  metav1.ConditionTrue,
				Message: "test",
				Reason:  "test",
			},
		},
		{
			name: "update condition",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "test",
							Status:  metav1.ConditionTrue,
							Message: "test",
							Reason:  "test",
						},
					},
				},
			},
			cond: metav1.Condition{
				Type:    "test",
				Status:  metav1.ConditionFalse,
				Message: "test",
				Reason:  "test",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(testscheme).
				WithObjects(c.managedCluster).WithStatusSubresource(c.managedCluster).Build()

			_, err := updateManagedClusterStatus(fakeClient, c.managedCluster.Name, c.cond)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

}

func TestDetermineKlusterletMode(t *testing.T) {
	cases := []struct {
		name         string
		annotations  map[string]string
		expectedMode operatorv1.InstallMode
	}{
		{
			name:         "default",
			annotations:  map[string]string{},
			expectedMode: operatorv1.InstallModeSingleton,
		},
		{
			name: "singleton",
			annotations: map[string]string{
				constants.KlusterletDeployModeAnnotation: "singleton",
			},
			expectedMode: operatorv1.InstallModeSingleton,
		},
		{
			name: "default",
			annotations: map[string]string{
				constants.KlusterletDeployModeAnnotation: "default",
			},
			expectedMode: operatorv1.InstallModeDefault,
		},
		{
			name: "hosted",
			annotations: map[string]string{
				constants.KlusterletDeployModeAnnotation: "hosted",
			},
			expectedMode: operatorv1.InstallModeHosted,
		},
	}

	for _, c := range cases {
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: c.annotations,
			},
		}
		mode := DetermineKlusterletMode(cluster)
		if mode != c.expectedMode {
			t.Errorf("expected mode not expected")
		}
	}
}

func TestAddManagedClusterFinalizer(t *testing.T) {
	cases := []struct {
		name               string
		managedCluster     *clusterv1.ManagedCluster
		finalizer          string
		expectedFinalizers []string
	}{
		{
			name: "Add a finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{"test"},
		},
		{
			name: "Add an existent finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test"},
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{"test"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			modified := resourcemerge.BoolPtr(false)
			AddManagedClusterFinalizer(modified, c.managedCluster, c.finalizer)
			assertFinalizers(t, c.managedCluster, c.expectedFinalizers)
		})
	}
}

func TestRemoveManagedClusterFinalizer(t *testing.T) {
	cases := []struct {
		name               string
		managedCluster     *clusterv1.ManagedCluster
		finalizer          string
		expectedFinalizers []string
	}{
		{
			name: "Remove a finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test1", "test2"},
				},
			},
			finalizer:          "test2",
			expectedFinalizers: []string{"test1"},
		},
		{
			name: "Empty finalizers",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test"},
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{},
		},
		{
			name: "Remove a nonexistent finalizer",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test_cluster",
					Finalizers: []string{"test1"},
				},
			},
			finalizer:          "test",
			expectedFinalizers: []string{"test1"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.managedCluster).WithStatusSubresource(c.managedCluster).Build()

			managedCluster := &clusterv1.ManagedCluster{}
			if err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: c.managedCluster.Name}, managedCluster); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			err := RemoveManagedClusterFinalizer(context.TODO(), fakeClient, eventstesting.NewTestingEventRecorder(t), managedCluster, c.finalizer)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			updatedManagedCluster := &clusterv1.ManagedCluster{}
			if err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: c.managedCluster.Name}, updatedManagedCluster); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			assertFinalizers(t, updatedManagedCluster, c.expectedFinalizers)
		})
	}
}

func TestApplyResources(t *testing.T) {
	var replicas int32 = 2

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test_cluster",
			Namespace:  "test_cluster",
			Generation: 1,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}
	resourceapply.SetSpecHashAnnotation(&deployment.ObjectMeta, deployment.Spec)

	cases := []struct {
		name           string
		kubeObjs       []runtime.Object
		klusterletObjs []runtime.Object
		workObjs       []runtime.Object
		crds           []runtime.Object
		requiredObjs   []runtime.Object
		owner          *clusterv1.ManagedCluster
		modified       bool
	}{
		{
			name:           "create resources",
			kubeObjs:       []runtime.Object{},
			klusterletObjs: []runtime.Object{},
			workObjs:       []runtime.Object{},
			crds:           []runtime.Object{},
			requiredObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&schedulingv1.PriorityClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_priority_class",
					},
				},
			},
			owner: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			modified: true,
		},
		{
			name: "update resources",
			kubeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Subjects: []rbacv1.Subject{
						{
							Name: "test1",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Name: "test1",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&schedulingv1.PriorityClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_priority_class",
					},
				},
			},
			crds: []runtime.Object{
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
			},
			klusterletObjs: []runtime.Object{
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
			},
			workObjs: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "label_test_cluster",
						Namespace: "label_test_cluster",
					},
				},
			},
			requiredObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Rules: []rbacv1.PolicyRule{
						{
							Resources: []string{"test"},
						},
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Subjects: []rbacv1.Subject{
						{
							Name: "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Name: "test",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Data: map[string][]byte{
						"test": []byte("test"),
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
					},
				},
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: operatorv1.KlusterletSpec{
						Namespace: "test",
					},
				},
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: crdv1beta1.CustomResourceDefinitionSpec{
						Version: "test",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: crdv1.CustomResourceDefinitionSpec{
						PreserveUnknownFields: true,
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Spec: workv1.ManifestWorkSpec{
						Workload: workv1.ManifestsTemplate{
							Manifests: []workv1.Manifest{
								{
									RawExtension: runtime.RawExtension{Raw: []byte("{\"test\":\"test1\"}")},
								},
							},
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "label_test_cluster",
						Namespace: "label_test_cluster",
						Labels: map[string]string{
							"test": "test",
						},
					},
				},
				&schedulingv1.PriorityClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_priority_class",
					},
					GlobalDefault: true,
				},
			},
			owner: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
			modified: true,
		},
		{
			name: "not modified resources",
			kubeObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Subjects: []rbacv1.Subject{
						{
							Name: "test1",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Name: "test1",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Data: map[string][]byte{
						"test": []byte("test"),
					},
					Type: corev1.SecretTypeOpaque,
				},
				deployment,
				&schedulingv1.PriorityClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_priority_class",
					},
				},
			},
			crds: []runtime.Object{
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: crdv1.CustomResourceDefinitionSpec{
						Conversion: &crdv1.CustomResourceConversion{
							Strategy: crdv1.NoneConverter,
						},
					},
				},
			},
			klusterletObjs: []runtime.Object{
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
			},
			workObjs: []runtime.Object{
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "label_test_cluster",
						Namespace: "label_test_cluster",
					},
				},
			},
			requiredObjs: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Subjects: []rbacv1.Subject{
						{
							Name: "test1",
						},
					},
					RoleRef: rbacv1.RoleRef{
						Name: "test1",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
					Data: map[string][]byte{
						"test": []byte("test"),
					},
					Type: corev1.SecretTypeOpaque,
				},
				deployment,
				&operatorv1.Klusterlet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
				},
				&crdv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_cluster",
					},
					Spec: crdv1.CustomResourceDefinitionSpec{
						Conversion: &crdv1.CustomResourceConversion{
							Strategy: crdv1.NoneConverter,
						},
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test_cluster",
						Namespace: "test_cluster",
					},
				},
				&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "label_test_cluster",
						Namespace: "label_test_cluster",
					},
				},
				&schedulingv1.PriorityClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test_priority_class",
					},
				},
			},
			owner:    nil,
			modified: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clientHolder := &ClientHolder{
				KubeClient:          kubefake.NewSimpleClientset(c.kubeObjs...),
				APIExtensionsClient: apiextensionsfake.NewSimpleClientset(c.crds...),
				OperatorClient:      operatorfake.NewSimpleClientset(c.klusterletObjs...),
				RuntimeClient:       fake.NewClientBuilder().WithScheme(testscheme).WithObjects().Build(),
				WorkClient:          workfake.NewSimpleClientset(c.workObjs...),
			}
			modified, err := ApplyResources(clientHolder, eventstesting.NewTestingEventRecorder(t),
				testscheme, c.owner, c.requiredObjs...)
			if err != nil {
				t.Errorf("name %s: unexpect err %v", c.name, err)
			}
			if modified != c.modified {
				t.Errorf("name %s: expect modified %v, but got %v", c.name, c.modified, modified)
			}
		})
	}
}

var tb = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: klusterlet
  namespace: "{{ .KlusterletNamespace }}"
{{- if .UseImagePullSecret }}
imagePullSecrets:
- name: "{{ .ImagePullSecretName }}"
{{- end}}
`

func TestAssetFromTemplate(t *testing.T) {
	cases := []struct {
		name     string
		config   interface{}
		validate func(t *testing.T, raw []byte)
	}{
		{
			name: "without ImagePullSecret",
			config: struct {
				KlusterletNamespace string
				UseImagePullSecret  bool
				ImagePullSecretName string
			}{
				KlusterletNamespace: "test",
			},
			validate: func(t *testing.T, raw []byte) {
				_, _, err := genericCodec.Decode(raw, nil, nil)
				if err != nil {
					t.Errorf("unexpect err %v, %v", string(raw), err)
				}
			},
		},
		{
			name: "with ImagePullSecret",
			config: struct {
				KlusterletNamespace string
				UseImagePullSecret  bool
				ImagePullSecretName string
			}{
				KlusterletNamespace: "test",
				UseImagePullSecret:  true,
				ImagePullSecretName: "test",
			},
			validate: func(t *testing.T, raw []byte) {
				_, _, err := genericCodec.Decode(raw, nil, nil)
				if err != nil {
					t.Errorf("unexpect err %v, %v", string(raw), err)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.validate(t, MustCreateAssetFromTemplate("test", []byte(tb), c.config))
		})
	}
}

func TestImportManagedClusterFromSecret(t *testing.T) {
	cases := []struct {
		name              string
		apiGroupResources []*restmapper.APIGroupResources
	}{
		{
			name: "only have crdv1beta1",
			apiGroupResources: []*restmapper.APIGroupResources{
				{
					Group: metav1.APIGroup{
						Name: "apiextensions.k8s.io",
						Versions: []metav1.GroupVersionForDiscovery{
							{Version: "v1beta1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1beta1"},
					},
					VersionedResources: map[string][]metav1.APIResource{
						"v1beta1": {
							{Name: "customresourcedefinitions", Namespaced: false, Kind: "CustomResourceDefinition"},
						},
					},
				},
			},
		},
		{
			name: "have crdv1beta1 and crdv1",
			apiGroupResources: []*restmapper.APIGroupResources{
				{
					Group: metav1.APIGroup{
						Name: "apiextensions.k8s.io",
						Versions: []metav1.GroupVersionForDiscovery{
							{Version: "v1beta1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1beta1"},
					},
					VersionedResources: map[string][]metav1.APIResource{
						"v1beta1": {
							{Name: "customresourcedefinitions", Namespaced: false, Kind: "CustomResourceDefinition"},
						},
					},
				},
				{
					Group: metav1.APIGroup{
						Name: "apiextensions.k8s.io",
						Versions: []metav1.GroupVersionForDiscovery{
							{Version: "v1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1"},
					},
					VersionedResources: map[string][]metav1.APIResource{
						"v1": {
							{Name: "customresourcedefinitions", Namespaced: false, Kind: "CustomResourceDefinition"},
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mapper := restmapper.NewDiscoveryRESTMapper(c.apiGroupResources)
			fakeRecorder := eventstesting.NewTestingEventRecorder(t)
			importSecret := testinghelpers.GetImportSecret("test_cluster")
			clientHolder := &ClientHolder{
				KubeClient:          kubefake.NewSimpleClientset(),
				APIExtensionsClient: apiextensionsfake.NewSimpleClientset(),
				OperatorClient:      operatorfake.NewSimpleClientset(),
				RuntimeClient:       fake.NewClientBuilder().WithScheme(testscheme).Build(),
			}
			_, err := ImportManagedClusterFromSecret(clientHolder, mapper, fakeRecorder, importSecret)
			if err != nil {
				t.Errorf("unexpect err %v", err)
			}
		})
	}
}

var badImportYaml = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: klusterlet
`

func TestUpdateManagedClusterBootstrapSecret(t *testing.T) {
	cases := []struct {
		name         string
		importSecret *corev1.Secret
		expectedErr  bool
		verifyFunc   func(t *testing.T, clientHolder *ClientHolder)
	}{
		{
			name: "bad import secret",
			importSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-import",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"import.yaml": []byte(badImportYaml),
				},
			},
			expectedErr: true,
			verifyFunc: func(t *testing.T, clientHolder *ClientHolder) {
				_, err := clientHolder.KubeClient.CoreV1().Secrets("open-cluster-management-agent").Get(context.TODO(), "bootstrap-hub-kubeconfig", metav1.GetOptions{})
				if !errors.IsNotFound(err) {
					t.Errorf("unexpect err %v", err)
				}
			},
		},
		{
			name:         "update import secret",
			importSecret: testinghelpers.GetImportSecret("test"),
			expectedErr:  false,
			verifyFunc: func(t *testing.T, clientHolder *ClientHolder) {
				_, err := clientHolder.KubeClient.CoreV1().Secrets("open-cluster-management-agent").Get(context.TODO(), "bootstrap-hub-kubeconfig", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpect err %v", err)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clientHolder := &ClientHolder{
				KubeClient: kubefake.NewSimpleClientset(),
			}

			_, err := UpdateManagedClusterBootstrapSecret(clientHolder, c.importSecret, eventstesting.NewTestingEventRecorder(t))
			if !c.expectedErr && err != nil {
				t.Errorf("unexpect err %v", err)
			}

			c.verifyFunc(t, clientHolder)
		})
	}
}

func TestGetNodeSelectorAndValidate(t *testing.T) {
	cases := []struct {
		name           string
		managedCluster *clusterv1.ManagedCluster
		expectedErr    string
	}{
		{
			name: "no nodeSelector annotation",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
		},
		{
			name: "no nodeSelector value",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/nodeSelector": "",
					},
				},
			},
			expectedErr: "unexpected end of JSON input",
		},
		{
			name: "empty nodeSelector annotation",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/nodeSelector": "{}",
					},
				},
			},
		},
		{
			name: "invalid nodeSelector annotation",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/nodeSelector": "{\"=\":\"test\"}",
					},
				},
			},
			expectedErr: "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')",
		},
		{
			name: "invalid nodeSelector annotation",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/nodeSelector": "{\"test\":\"=\"}",
					},
				},
			},
			expectedErr: "a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')",
		},
		{
			name: "nodeSelector annotation",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/nodeSelector": "{\"kubernetes.io/os\":\"linux\"}",
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ns, err := GetNodeSelectorFromManagedClusterAnnotations(c.managedCluster.Annotations)
			if err == nil {
				err = ValidateNodeSelector(ns)
				if err != nil {
					err = fmt.Errorf("invalid nodeSelector annotation %v", err)
				}
			}
			switch {
			case len(c.expectedErr) == 0:
				if err != nil {
					t.Errorf("unexpect err: %v", err)
				}
			case len(c.expectedErr) != 0:
				if err == nil {
					t.Errorf("expect err %s, but failed", c.expectedErr)
				}

				if fmt.Sprintf("invalid nodeSelector annotation %s", c.expectedErr) != err.Error() {
					t.Errorf("expect %v, but %v", c.expectedErr, err.Error())
				}
			}
		})
	}
}

func TestGetTolerationsAndValidate(t *testing.T) {
	cases := []struct {
		name           string
		managedCluster *clusterv1.ManagedCluster
		expectedErr    string
	}{
		{
			name: "no tolerations annotation",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
				},
			},
		},
		{
			name: "no tolerations value",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "",
					},
				},
			},
			expectedErr: "unexpected end of JSON input",
		},
		{
			name: "empty tolerations array",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[]",
					},
				},
			},
		},
		{
			name: "empty toleration in tolerations",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{}]",
					},
				},
			},
			expectedErr: "operator must be Exists when `key` is empty, which means \"match all values and all keys\"",
		},
		{
			name: "invalid toleration key",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{\"key\":\"nospecialchars^=@\",\"operator\":\"Equal\",\"value\":\"bar\",\"effect\":\"NoSchedule\"}]",
					},
				},
			},
			expectedErr: "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')",
		},
		{
			name: "invalid toleration operator",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{\"key\":\"foo\",\"operator\":\"In\",\"value\":\"bar\",\"effect\":\"NoSchedule\"}]",
					},
				},
			},
			expectedErr: "the operator \"In\" is not supported",
		},
		{
			name: "invalid toleration effect",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{\"key\":\"foo\",\"value\":\"bar\",\"effect\":\"Test\"}]",
					},
				},
			},
			expectedErr: "the effect \"Test\" is not supported",
		},
		{
			name: "value must be empty when `operator` is 'Exists'",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{\"key\":\"foo\",\"operator\":\"Exists\",\"value\":\"bar\",\"effect\":\"NoSchedule\"}]",
					},
				},
			},
			expectedErr: "value must be empty when `operator` is 'Exists'",
		},
		{
			name: "operator must be 'Exists' when `key` is empty",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{\"operator\":\"Exists\",\"value\":\"bar\",\"effect\":\"NoSchedule\"}]",
					},
				},
			},
			expectedErr: "value must be empty when `operator` is 'Exists'",
		},
		{
			name: "effect must be 'NoExecute' when `TolerationSeconds` is set",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{\"key\":\"foo\",\"operator\":\"Exists\",\"effect\":\"NoSchedule\",\"tolerationSeconds\":20}]",
					},
				},
			},
			expectedErr: "effect must be 'NoExecute' when `tolerationSeconds` is set",
		},
		{
			name: "tolerations annotation",
			managedCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test_cluster",
					Annotations: map[string]string{
						"open-cluster-management/tolerations": "[{\"key\":\"foo\",\"operator\":\"Exists\",\"effect\":\"NoExecute\",\"tolerationSeconds\":20}]",
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tolerations, err := GetTolerationsFromManagedClusterAnnotations(c.managedCluster.GetAnnotations())
			if err == nil {
				err = ValidateTolerations(tolerations)
				if err != nil {
					err = fmt.Errorf("invalid tolerations annotation %v", err)
				}
			}
			switch {
			case len(c.expectedErr) == 0:
				if err != nil {
					t.Errorf("unexpect err: %v", err)
				}
			case len(c.expectedErr) != 0:
				if err == nil {
					t.Errorf("expect err %s, but failed", c.expectedErr)
				}

				if fmt.Sprintf("invalid tolerations annotation %s", c.expectedErr) != err.Error() {
					t.Errorf("expect %v, but %v", c.expectedErr, err.Error())
				}
			}
		})
	}
}

func assertFinalizers(t *testing.T, obj runtime.Object, finalizers []string) {
	accessor, _ := meta.Accessor(obj)
	actual := accessor.GetFinalizers()
	if len(actual) == 0 && len(finalizers) == 0 {
		return
	}
	if !reflect.DeepEqual(actual, finalizers) {
		t.Error(diff.ObjectDiff(actual, finalizers))
	}
}

func createBasic(serverURL, clusterName string, caCert, clientKey, clientCert []byte) *clientcmdapi.Config {
	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   serverURL,
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"default-context": {
				Cluster:  clusterName,
				AuthInfo: "default-auth",
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"default-auth": {
				ClientKeyData:         clientKey,
				ClientCertificateData: clientCert,
			},
		},
		CurrentContext: "default-context",
	}
}

func TestForceDeleteManagedClusterAddon(t *testing.T) {
	cases := []struct {
		name               string
		existAddon         *addonv1alpha1.ManagedClusterAddOn
		addon              addonv1alpha1.ManagedClusterAddOn
		expectAddonDeleted bool
		expectFinalizers   []string
	}{
		{
			name: "no finalizer",
			existAddon: &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			addon: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			expectAddonDeleted: true,
		},
		{
			name: "no addon",
			addon: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			expectAddonDeleted: true,
		},
		{
			name: "no hosting cleanup finalizer",
			existAddon: &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Finalizers: []string{"a", "b"},
				},
			},
			addon: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			expectAddonDeleted: true,
		},
		{
			name: "only hosting cleanup finalizer",
			existAddon: &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Finalizers: []string{addonv1alpha1.AddonHostingManifestFinalizer},
				},
			},
			addon: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			expectAddonDeleted: false,
			expectFinalizers:   []string{addonv1alpha1.AddonHostingManifestFinalizer},
		},
		{
			name: "only hosting pre delete hook cleanup finalizer",
			existAddon: &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test",
					Finalizers: []string{addonv1alpha1.AddonHostingPreDeleteHookFinalizer},
				},
			},
			addon: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			expectAddonDeleted: false,
			expectFinalizers:   []string{addonv1alpha1.AddonHostingPreDeleteHookFinalizer},
		},
		{
			name: "hosting cleanup and other finalizers",
			existAddon: &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Finalizers: []string{addonv1alpha1.AddonHostingManifestFinalizer,
						addonv1alpha1.AddonHostingPreDeleteHookFinalizer,
						addonv1alpha1.AddonDeprecatedHostingManifestFinalizer,
						addonv1alpha1.AddonDeprecatedHostingPreDeleteHookFinalizer,
						"a", "b"},
				},
			},
			addon: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			expectAddonDeleted: false,
			expectFinalizers: []string{addonv1alpha1.AddonHostingManifestFinalizer,
				addonv1alpha1.AddonHostingPreDeleteHookFinalizer,
				addonv1alpha1.AddonDeprecatedHostingManifestFinalizer,
				addonv1alpha1.AddonDeprecatedHostingPreDeleteHookFinalizer},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.TODO()
			fakeRecorder := eventstesting.NewTestingEventRecorder(t)

			runtimeClientBuilder := fake.NewClientBuilder().WithScheme(testscheme)
			if c.existAddon != nil {
				runtimeClientBuilder.WithObjects(c.existAddon)
			}
			runtimeClient := runtimeClientBuilder.Build()
			err := ForceDeleteManagedClusterAddon(ctx, runtimeClient, fakeRecorder, c.addon.Namespace, c.addon.Name)
			if err != nil {
				t.Errorf("unexpect err %v", err)
			}

			addon := addonv1alpha1.ManagedClusterAddOn{}
			err = runtimeClient.Get(ctx, types.NamespacedName{Namespace: c.addon.Namespace, Name: c.addon.Name}, &addon)

			if err != nil && !errors.IsNotFound(err) {
				t.Errorf("unexpect err %v", err)
			}
			if c.expectAddonDeleted && !errors.IsNotFound(err) {
				t.Errorf("addon should be deleted, but got err %v", err)
			}
			if !c.expectAddonDeleted {
				if err != nil {
					t.Errorf("addon should not be deleted, but got err %v", err)
				}

				if !reflect.DeepEqual(addon.Finalizers, c.expectFinalizers) {
					t.Errorf("expect finalizer %v but got %v", c.expectFinalizers, addon.Finalizers)
				}

				if addon.DeletionTimestamp.IsZero() {
					t.Errorf("addon should be in deleting status")
				}
			}
		})
	}
}

func TestGenerateImportClientFromKubeTokenSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var obj interface{}
		switch req.URL.Path {
		case "/api":
			obj = &metav1.APIVersions{
				Versions: []string{
					"v1",
				},
			}
		case "/apis":
			obj = &metav1.APIGroupList{
				Groups: []metav1.APIGroup{
					{
						Name: "extensions",
						Versions: []metav1.GroupVersionForDiscovery{
							{GroupVersion: "extensions/v1beta1"},
						},
					},
				},
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		output, err := json.Marshal(obj)
		if err != nil {
			t.Fatalf("unexpected encoding error: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(output)
	}))
	defer server.Close()

	cases := []struct {
		name      string
		secret    *corev1.Secret
		expectErr bool
	}{
		{
			name: "missing token",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "auto-import-secret",
				},
				Data: map[string][]byte{
					"server": []byte(server.URL),
					// no auth info
				},
			},
			expectErr: true,
		},
		{
			name: "token secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "auto-import-secret",
				},
				Data: map[string][]byte{
					"server": []byte(server.URL),
					"token":  []byte("1234"),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, _, err := GenerateImportClientFromKubeTokenSecret(c.secret)
			if !c.expectErr && err != nil {
				t.Errorf("unexpected error %v", err)
			}

			if c.expectErr && err == nil {
				t.Errorf("expected error, but failed")
			}
		})
	}
}

func TestGenerateImportClientFromRosaCluster(t *testing.T) {
	gomega.RegisterTestingT(t)

	accessToken := clustersmgmttesting.MakeTokenString("Bearer", 5*time.Minute)
	refreshToken := clustersmgmttesting.MakeTokenString("Refresh", 10*time.Hour)

	oidServer := clustersmgmttesting.MakeTCPServer()
	oidServer.AppendHandlers(
		ghttp.CombineHandlers(
			clustersmgmttesting.RespondWithAccessAndRefreshTokens(accessToken, refreshToken),
		),
	)
	oauthServer := clustersmgmttesting.MakeTCPServer()
	oauthServer.AppendHandlers(
		ghttp.CombineHandlers(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// just ensure the token request is received
				w.WriteHeader(http.StatusNotImplemented)
			}),
		),
	)
	apiServer := clustersmgmttesting.MakeTCPServer()
	apiServer.AppendHandlers(
		ghttp.CombineHandlers(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/c0001" {
					t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
				}
			}),
			clustersmgmttesting.RespondWithJSON(http.StatusOK, newRosaCluster(oauthServer.URL())),
		),
		ghttp.CombineHandlers(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/c0001/identity_providers" {
					t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
				}
			}),
			clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
		),
		ghttp.CombineHandlers(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/clusters_mgmt/v1/clusters/c0001/identity_providers" {
					t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
				}
			}),
			clustersmgmttesting.RespondWithJSON(http.StatusCreated, "{}"),
		),
		ghttp.CombineHandlers(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/clusters_mgmt/v1/clusters/c0001/groups/cluster-admins/users/acm-import" {
					t.Fatalf("unexpected request %s - %s", r.Method, r.URL.Path)
				}
			}),
			clustersmgmttesting.RespondWithJSON(http.StatusOK, "{}"),
		),
	)
	defer func() {
		oidServer.Close()
		oauthServer.Close()
		apiServer.Close()
	}()

	cases := []struct {
		name   string
		getter *RosaKubeConfigGetter
		secret *corev1.Secret
	}{
		{
			name:   "generate client",
			getter: NewRosaKubeConfigGetter(),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "auto-import-secret",
				},
				Data: map[string][]byte{
					"api_token":   []byte(accessToken),
					"cluster_id":  []byte("c0001"),
					"api_url":     []byte(apiServer.URL()),
					"token_url":   []byte(oidServer.URL()),
					"retry_times": []byte("2"),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, _, _, _ := GenerateImportClientFromRosaCluster(c.getter, c.secret)
			if !result.Requeue {
				t.Errorf("expected requeue result, but failed")
			}
		})
	}
}

func TestIsKubeVersionChanged(t *testing.T) {
	cases := []struct {
		name       string
		oldCluster runtime.Object
		newCluster runtime.Object
		changed    bool
	}{
		{
			name:       "invalid old cluster",
			oldCluster: &corev1.Secret{},
			newCluster: &clusterv1.ManagedCluster{},
		},
		{
			name:       "invalid new cluster",
			oldCluster: &clusterv1.ManagedCluster{},
			newCluster: &corev1.Secret{},
		},
		{
			name:       "kube version is set",
			oldCluster: &clusterv1.ManagedCluster{},
			newCluster: &clusterv1.ManagedCluster{
				Status: clusterv1.ManagedClusterStatus{
					Version: clusterv1.ManagedClusterVersion{
						Kubernetes: "v1.14.12",
					},
				},
			},
			changed: true,
		},
		{
			name: "kube version is unset",
			oldCluster: &clusterv1.ManagedCluster{
				Status: clusterv1.ManagedClusterStatus{
					Version: clusterv1.ManagedClusterVersion{
						Kubernetes: "v1.14.12",
					},
				},
			},
			newCluster: &clusterv1.ManagedCluster{},
			changed:    true,
		},
		{
			name: "cluster upgraded",
			oldCluster: &clusterv1.ManagedCluster{
				Status: clusterv1.ManagedClusterStatus{
					Version: clusterv1.ManagedClusterVersion{
						Kubernetes: "v1.14.12",
					},
				},
			},
			newCluster: &clusterv1.ManagedCluster{
				Status: clusterv1.ManagedClusterStatus{
					Version: clusterv1.ManagedClusterVersion{
						Kubernetes: "v1.15.03",
					},
				},
			},
			changed: true,
		},
		{
			name: "no change",
			oldCluster: &clusterv1.ManagedCluster{
				Status: clusterv1.ManagedClusterStatus{
					Version: clusterv1.ManagedClusterVersion{
						Kubernetes: "v1.14.12",
					},
				},
			},
			newCluster: &clusterv1.ManagedCluster{
				Status: clusterv1.ManagedClusterStatus{
					Version: clusterv1.ManagedClusterVersion{
						Kubernetes: "v1.14.12",
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changed := IsKubeVersionChanged(c.oldCluster, c.newCluster)
			if changed != c.changed {
				t.Errorf("expect changed %v but got %v", c.changed, changed)
			}
		})
	}
}

func TestSupportPriorityClass(t *testing.T) {
	cases := []struct {
		name              string
		kubeVersionString string
		supported         bool
		expectedErr       string
	}{
		{
			name: "nil cluster",
		},
		{
			name: "without kubeVersion",
		},
		{
			name:              "kube v1.13",
			kubeVersionString: "v1.13.0",
		},
		{
			name:              "kube v1.14",
			kubeVersionString: "v1.14.0",
			supported:         true,
		},
		{
			name:              "kube v1.22",
			kubeVersionString: "v1.22.5+5c84e52",
			supported:         true,
		},
		{
			name:              "invalid kubeVersion",
			kubeVersionString: "invalid-version",
			expectedErr:       "could not parse \"invalid-version\" as version",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cluster := &clusterv1.ManagedCluster{
				Status: clusterv1.ManagedClusterStatus{
					Version: clusterv1.ManagedClusterVersion{
						Kubernetes: c.kubeVersionString,
					},
				},
			}
			supported, err := SupportPriorityClass(cluster)
			if c.expectedErr != "" && err == nil {
				t.Errorf("expected error, but failed")
			}
			if err != nil && c.expectedErr != err.Error() {
				t.Errorf("unexpected error: %v", err)
			}
			if err != nil {
				if c.expectedErr == "" {
					t.Errorf("unexpected error: %v", err)
				} else if err.Error() != c.expectedErr {
					t.Errorf("expected error %q, but got %q", c.expectedErr, err.Error())
				}
			}
			if supported != c.supported {
				t.Errorf("expect supported %v but got %v", c.supported, supported)
			}
		})
	}
}

func TestResourceIsNotFound(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		expectedRst bool
	}{
		{
			name:        "nil error",
			err:         nil,
			expectedRst: false,
		},
		{
			name:        "other err",
			err:         fmt.Errorf("cluster abc is not found"),
			expectedRst: false,
		},
		{
			name: "resource not found err",
			err: fmt.Errorf("failed to get API group resources: unable to retrieve the complete " +
				"list of server APIs: config.openshift.io/v1: the server could not find the requested resource"),
			expectedRst: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.expectedRst != ResourceIsNotFound(c.err) {
				t.Errorf("expected %v, but got %v", c.expectedRst, ResourceIsNotFound(c.err))
			}
		})
	}
}
