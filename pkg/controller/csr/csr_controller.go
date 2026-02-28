// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package csr

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	userNameSignature = "system:serviceaccount:%s:%s-bootstrap-sa"
	clusterLabel      = "open-cluster-management.io/cluster-name"
)

var log = logf.Log.WithName("controller_csr")

func getClusterName(csr *certificatesv1.CertificateSigningRequest) (clusterName string) {
	for label, v := range csr.GetObjectMeta().GetLabels() {
		if label == clusterLabel {
			clusterName = v
		}
	}
	return clusterName
}

func getApprovalType(csr *certificatesv1.CertificateSigningRequest) string {
	if csr.Status.Conditions == nil {
		return ""
	}
	for _, c := range csr.Status.Conditions {
		if c.Type == certificatesv1.CertificateApproved || c.Type == certificatesv1.CertificateDenied {
			return string(c.Type)
		}
	}
	return ""
}

func validUsername(csr *certificatesv1.CertificateSigningRequest, clusterName string) bool {
	return csr.Spec.Username == fmt.Sprintf(userNameSignature, clusterName, clusterName)
}

func csrPredicate(csr *certificatesv1.CertificateSigningRequest) bool {
	clusterName := getClusterName(csr)
	return clusterName != "" &&
		getApprovalType(csr) == "" &&
		validUsername(csr, clusterName)
}

// ReconcileCSR reconciles the managed cluster CSR object
type ReconcileCSR struct {
	clientHolder *helpers.ClientHolder
	recorder     events.Recorder
}

// blank assignment to verify that ReconcileCSR implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileCSR{}

// Reconcile reads that state of the csr for a ReconcileCSR object and makes changes based on the state read
// and what is in the CertificateSigningRequest.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCSR) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	csrReq := r.clientHolder.KubeClient.CertificatesV1().CertificateSigningRequests()

	csr, err := csrReq.Get(ctx, request.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}
	if isCSRInTerminalState(&csr.Status) {
		return reconcile.Result{}, nil
	}

	clusterName := getClusterName(csr)
	cluster := clusterv1.ManagedCluster{}
	err = r.clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: clusterName}, &cluster)
	if errors.IsNotFound(err) {
		// no managed cluster, do nothing.
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Reconciling CSR")

	csr = csr.DeepCopy()
	csr.Status.Conditions = append(csr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type:           certificatesv1.CertificateApproved,
		Status:         corev1.ConditionTrue,
		Reason:         "AutoApprovedByCSRController",
		Message:        "The managedcluster-import-controller auto approval automatically approved this CSR",
		LastUpdateTime: metav1.Now(),
	})
	if _, err := csrReq.UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{}); err != nil {
		return reconcile.Result{}, err
	}

	r.recorder.Eventf("ManagedClusterCSRAutoApproved", "managed cluster csr %q is auto approved by import controller", csr.Name)
	return reconcile.Result{}, nil
}

// check whether a CSR is in terminal state
func isCSRInTerminalState(status *certificatesv1.CertificateSigningRequestStatus) bool {
	for _, c := range status.Conditions {
		if c.Type == certificatesv1.CertificateApproved {
			return true
		}
		if c.Type == certificatesv1.CertificateDenied {
			return true
		}
	}
	return false
}
