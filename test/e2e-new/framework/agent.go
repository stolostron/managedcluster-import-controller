// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package framework

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	agentNamespace = "open-cluster-management-agent"
	agentSelector  = "app=klusterlet-agent"
	leaseName      = "klusterlet-agent-lock"
)

// EnsureAgentReady forces an agent pod restart and waits for leader election to complete.
// This is a safety net against the leader election race condition that occurs after
// deployment rollouts. See test/e2e/README.md for full details.
//
// This method is a no-op if no agent pods exist (e.g., tests without klusterlet deployment).
func (h *Hub) EnsureAgentReady() {
	agentPods, err := h.KubeClient.CoreV1().Pods(agentNamespace).List(
		context.TODO(), metav1.ListOptions{LabelSelector: agentSelector})
	if err != nil || len(agentPods.Items) == 0 {
		return
	}

	// Delete all agent pods to force a restart
	for _, pod := range agentPods.Items {
		_ = h.KubeClient.CoreV1().Pods(agentNamespace).Delete(
			context.TODO(), pod.Name, metav1.DeleteOptions{})
	}

	// Delete the leader election lease so the new pod must re-elect
	_ = h.KubeClient.CoordinationV1().Leases(agentNamespace).Delete(
		context.TODO(), leaseName, metav1.DeleteOptions{})

	h.assertAgentLeaderElection()
}

// RestartAgentPods deletes all klusterlet-agent pods in the given namespaces,
// waits for replacements to come up, and ensures the new pod completes leader election.
func (h *Hub) RestartAgentPods(namespaces ...string) {
	if len(namespaces) == 0 {
		namespaces = []string{agentNamespace}
	}

	nsPodsNum := map[string]int{}
	for _, ns := range namespaces {
		pods, err := h.KubeClient.CoreV1().Pods(ns).List(
			context.TODO(), metav1.ListOptions{LabelSelector: agentSelector})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		nsPodsNum[ns] = len(pods.Items)
		for _, pod := range pods.Items {
			err = h.KubeClient.CoreV1().Pods(ns).Delete(
				context.TODO(), pod.Name, metav1.DeleteOptions{})
			// Tolerate NotFound: the pod may have already been deleted by the
			// Deployment controller during a rolling update.
			if err != nil && !errors.IsNotFound(err) {
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}
		}
	}

	gomega.Eventually(func() error {
		for _, ns := range namespaces {
			pods, err := h.KubeClient.CoreV1().Pods(ns).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector})
			if err != nil {
				return err
			}
			if len(pods.Items) != nsPodsNum[ns] {
				return fmt.Errorf("waiting for pods restart in namespace %s", ns)
			}
		}
		return nil
	}, 120*time.Second, 1*time.Second).Should(gomega.Succeed())

	h.assertAgentLeaderElection()
}

// assertAgentLeaderElection waits for the klusterlet-agent pod to become the leader.
//
// Steps:
// 1. Wait for pending deployment rollout (10s delay)
// 2. Filter out terminating pods
// 3. Wait for exactly 1 non-terminating pod
// 4. Verify the leader lease HolderIdentity matches the pod name
func (h *Hub) assertAgentLeaderElection() {
	// Wait for any pending deployment rollout to be initiated.
	time.Sleep(10 * time.Second)

	start := time.Now()

	ginkgo.By("Check if klusterlet agent is leader", func() {
		gomega.Eventually(func() error {
			allPods, err := h.KubeClient.CoreV1().Pods(agentNamespace).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector})
			if err != nil {
				return fmt.Errorf("could not get agent pod: %v", err)
			}

			// Filter out terminating pods
			var pods []corev1.Pod
			for _, pod := range allPods.Items {
				if pod.DeletionTimestamp.IsZero() {
					pods = append(pods, pod)
				}
			}

			if len(pods) != 1 {
				return fmt.Errorf("expected 1 non-terminating agent pod, got %d (total including terminating: %d)",
					len(pods), len(allPods.Items))
			}

			lease, err := h.KubeClient.CoordinationV1().Leases(agentNamespace).Get(
				context.TODO(), leaseName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("could not get Lease: %v", err)
			}

			if lease.Spec.HolderIdentity != nil && strings.HasPrefix(*lease.Spec.HolderIdentity, pods[0].Name) {
				return nil
			}

			return fmt.Errorf("klusterlet agent leader is still %s not %s",
				*lease.Spec.HolderIdentity, pods[0].Name)
		}, 180*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
	Logf("assert agent leader election spending time: %.2f seconds", time.Since(start).Seconds())
}
