// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
)

// DeleteAutoImportSecret delete the auto-import-secret if the secret does not have the keeping annotation
func DeleteAutoImportSecret(ctx context.Context, kubeClient kubernetes.Interface,
	secret *corev1.Secret, recorder events.Recorder) error {
	if _, ok := secret.Annotations[constants.AnnotationKeepingAutoImportSecret]; ok {
		recorder.Eventf("AutoImportSecretSaved",
			fmt.Sprintf("The auto import secret %s/%s is saved", secret.Namespace, secret.Name))
		return nil
	}

	if err := kubeClient.CoreV1().Secrets(secret.Namespace).Delete(
		ctx, secret.Name, metav1.DeleteOptions{}); err != nil {
		return err
	}

	recorder.Eventf("AutoImportSecretDeleted",
		fmt.Sprintf("The auto import secret %s/%s is deleted", secret.Namespace, secret.Name))
	return nil
}
