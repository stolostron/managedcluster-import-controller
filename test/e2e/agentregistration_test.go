// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Agent Registration", Label("agent-registration"), Ordered, func() {
	It(`Should have the managed cluster "cluster-e2e-test-agent" registrated.`, func() {
		clusterName := "cluster-e2e-test-agent"
		By("Managed Cluster Created", func() {
			Eventually(func() error {
				_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				return nil
			}, 5*time.Minute, 10*time.Second).Should(Succeed())
		})
	})
})
