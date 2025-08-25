// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Invalid CSR", ginkgo.Label("config"), func() {
	ginkgo.It("Should not approve the CSR with wrong labels", func() {
		csr := util.NewCSR(util.NewLable("open-cluster-management.io/cluster-name", "wrong"))
		csrReq := hubKubeClient.CertificatesV1().CertificateSigningRequests()

		ginkgo.By("Create a csr with wrong labels", func() {
			_, err := csrReq.Create(context.TODO(), csr, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By("Should not approve", func() {
			gomega.Consistently(func() bool {
				got, err := csrReq.Get(context.TODO(), csr.Name, metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				for _, c := range got.Status.Conditions {
					if c.Type == certificatesv1.CertificateApproved {
						return false
					}
				}

				return true
			}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
		})
	})
})
