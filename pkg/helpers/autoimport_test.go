// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package helpers

import (
	"context"
	"testing"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
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
