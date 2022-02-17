// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
)

// UpdateAutoImportRetryTimes minus 1 for the value of AutoImportRetryName in the auto import secret
func UpdateAutoImportRetryTimes(ctx context.Context, kubeClient kubernetes.Interface, recorder events.Recorder, secret *corev1.Secret) error {
	autoImportRetry, err := strconv.Atoi(string(secret.Data[constants.AutoImportRetryName]))
	if err != nil {
		recorder.Warningf("AutoImportRetryInvalid", "The value of autoImportRetry is invalid in auto-import-secret secret")
		return err
	}

	recorder.Eventf("RetryToImportCluster", "Retry to import cluster %s, %d", secret.Namespace, autoImportRetry)

	autoImportRetry--
	if autoImportRetry < 0 {
		// stop retry, delete the auto-import-secret
		err := kubeClient.CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		recorder.Eventf("AutoImportSecretDeleted",
			fmt.Sprintf("Exceed the retry times, delete the auto import secret %s/%s", secret.Namespace, secret.Name))
		return nil
	}

	secret.Data[constants.AutoImportRetryName] = []byte(strconv.Itoa(autoImportRetry))
	_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}
