// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusternamespacedeletion

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	clustercontroller "github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var _ = Describe("cluster namespace deletion controller", func() {
	var cluster *clusterv1.ManagedCluster
	const timeout = time.Second * 30
	const interval = time.Second * 1

	BeforeEach(func() {
		clusterName := "cluster-" + utilrand.String(5)
		cluster = &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterName,
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: true,
			},
		}
		err := runtimeClient.Create(context.Background(), cluster)
		Expect(err).ToNot(HaveOccurred())

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterName,
				Labels: map[string]string{
					clustercontroller.ClusterLabel: clusterName,
				},
			},
		}
		_, err = k8sClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
	})

	Context("cluster finalizer", func() {
		It("Should not delete ns when cluster has multiple finalizers", func() {
			By("add finalizers to clusters")
			Eventually(func() error {
				currentcluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: cluster.Name}, currentcluster)
				if err != nil {
					return err
				}

				currentcluster.Finalizers = []string{
					constants.ImportFinalizer,
					"test.open-cluster-management.io/test",
				}

				return runtimeClient.Update(context.TODO(), currentcluster)
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("delete cluster")
			err := runtimeClient.Delete(context.TODO(), cluster)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ns.DeletionTimestamp.IsZero()).Should(BeTrue())

			By("remove finalizers to clusters")
			Eventually(func() error {
				currentcluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: cluster.Name}, currentcluster)
				if err != nil {
					return err
				}

				currentcluster.Finalizers = []string{
					constants.ImportFinalizer,
				}

				return runtimeClient.Update(context.TODO(), currentcluster)
			}, timeout, interval).ShouldNot(HaveOccurred())

			Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}

				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(HaveOccurred())

		})

		It("Should delete ns when cluster has no finalizers", func() {
			By("delete cluster")
			err := runtimeClient.Delete(context.TODO(), cluster)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(HaveOccurred())
		})
	})

	Context("check other dependencies", func() {
		It("Should not delete ns when cluster has addon", func() {
			By("create addon")
			addon := &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: cluster.Name,
				},
			}
			err := runtimeClient.Create(context.TODO(), addon)
			Expect(err).ToNot(HaveOccurred())

			By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			Expect(err).ToNot(HaveOccurred())

			By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ns.DeletionTimestamp.IsZero()).Should(BeTrue())

			err = runtimeClient.Delete(context.TODO(), addon)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(HaveOccurred())
		})

		It("Should not delete ns when cluster has clusterDeployment", func() {
			By("create clusterdeployment")
			err := util.CreateClusterDeployment(hubDynamicClient, cluster.Name)
			Expect(err).ToNot(HaveOccurred())

			By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			Expect(err).ToNot(HaveOccurred())

			By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ns.DeletionTimestamp.IsZero()).Should(BeTrue())

			err = util.DeleteClusterDeployment(hubDynamicClient, cluster.Name)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(HaveOccurred())
		})

		It("Should not delete ns when cluster has infraenv", func() {
			By("create infra env")
			infra := &asv1beta1.InfraEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-infra",
					Namespace: cluster.Name,
				},
				Spec: asv1beta1.InfraEnvSpec{
					PullSecretRef: &corev1.LocalObjectReference{
						Name: "test",
					},
				},
			}
			err := runtimeClient.Create(context.TODO(), infra)
			Expect(err).ToNot(HaveOccurred())

			By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			Expect(err).ToNot(HaveOccurred())

			By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ns.DeletionTimestamp.IsZero()).Should(BeTrue())

			err = runtimeClient.Delete(context.TODO(), infra)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(HaveOccurred())
		})

		It("Should not delete ns when cluster has infraenv", func() {
			By("create pods")
			prehookpod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      postHookJobPrefix + "test",
					Namespace: cluster.Name,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "hook", Image: "test"},
					},
				},
			}

			otherpod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "othertest",
					Namespace: cluster.Name,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "hook", Image: "test"},
					},
				},
			}

			_, err := k8sClient.CoreV1().Pods(cluster.Name).Create(context.TODO(), prehookpod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			_, err = k8sClient.CoreV1().Pods(cluster.Name).Create(context.TODO(), otherpod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			By("update pods")
			Eventually(func() error {
				pod, err := k8sClient.CoreV1().Pods(cluster.Name).Get(context.TODO(), prehookpod.Name, metav1.GetOptions{})

				if err != nil {
					return err
				}

				pod.Status.Phase = corev1.PodRunning
				_, err = k8sClient.CoreV1().Pods(cluster.Name).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
				return err
			}, timeout, interval).ShouldNot(HaveOccurred())

			By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			Expect(err).ToNot(HaveOccurred())

			By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ns.DeletionTimestamp.IsZero()).Should(BeTrue())

			err = k8sClient.CoreV1().Pods(cluster.Name).Delete(context.TODO(), otherpod.Name, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				pod, err := k8sClient.CoreV1().Pods(cluster.Name).Get(context.TODO(), prehookpod.Name, metav1.GetOptions{})

				if err != nil {
					return err
				}

				pod.Status.Phase = corev1.PodSucceeded
				_, err = k8sClient.CoreV1().Pods(cluster.Name).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
				return err
			}, timeout, interval).ShouldNot(HaveOccurred())

			Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(HaveOccurred())
		})
	})
})
