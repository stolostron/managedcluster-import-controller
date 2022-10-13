// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package helpers

import (
	"context"
	"strconv"
	"testing"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestUpdateAutoImportRetryTimes(t *testing.T) {
	cases := []struct {
		name             string
		autoImportSecret *corev1.Secret
		expectedErr      bool
		verifyFunc       func(t *testing.T, kubeClinent kubernetes.Interface)
	}{
		{
			name: "invalid autoImportRetry",
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: "test",
				},
				Data: map[string][]byte{},
			},
			expectedErr: true,
			verifyFunc:  func(t *testing.T, kubeClinent kubernetes.Interface) {},
		},
		{
			name: "update autoImportRetry",
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"autoImportRetry": []byte("1"),
				},
			},
			verifyFunc: func(t *testing.T, kubeClinent kubernetes.Interface) {
				secret, err := kubeClinent.CoreV1().Secrets("test").Get(context.TODO(), "auto-import-secret", metav1.GetOptions{})
				if err != nil {
					t.Errorf("unexpect err %v", err)
				}

				retry, err := strconv.Atoi(string(secret.Data["autoImportRetry"]))
				if err != nil {
					t.Errorf("unexpect err %v", err)
				}

				if retry != 0 {
					t.Errorf("unexpect 0 but %v", retry)
				}
			},
		},
		{
			name: "delete the secret after retry",
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
			name: "delete the secret after retry",
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
			name: "keey the secret after retry",
			autoImportSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "auto-import-secret",
					Namespace: "test",
					Annotations: map[string]string{
						"managedcluster-import-controller.open-cluster-management.io/keeping-auto-import-secret": "",
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

			err := UpdateAutoImportRetryTimes(context.TODO(), kubeClient, eventstesting.NewTestingEventRecorder(t), c.autoImportSecret)
			if !c.expectedErr && err != nil {
				t.Errorf("unexpect err %v", err)
			}

			c.verifyFunc(t, kubeClient)
		})
	}
}
