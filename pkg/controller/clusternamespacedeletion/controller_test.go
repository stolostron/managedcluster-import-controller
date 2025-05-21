// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clusternamespacedeletion

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	asv1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	clustercontroller "github.com/stolostron/managedcluster-import-controller/pkg/controller/managedcluster"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	siteconfigv1alpha1 "github.com/stolostron/siteconfig/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var _ = ginkgo.Describe("cluster namespace deletion controller", func() {
	var cluster *clusterv1.ManagedCluster
	const timeout = time.Second * 30
	const interval = time.Second * 1

	ginkgo.BeforeEach(func() {
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
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

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
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.Context("cluster finalizer", func() {
		ginkgo.It("Should not delete ns when cluster has multiple finalizers", func() {
			ginkgo.By("add finalizers to clusters")
			gomega.Eventually(func() error {
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
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err := runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			ginkgo.By("remove finalizers to clusters")
			gomega.Eventually(func() error {
				currentcluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(context.TODO(), types.NamespacedName{Name: cluster.Name}, currentcluster)
				if err != nil {
					return err
				}

				currentcluster.Finalizers = []string{
					constants.ImportFinalizer,
				}

				return runtimeClient.Update(context.TODO(), currentcluster)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}

				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

		})

		ginkgo.It("Should delete ns when cluster has no finalizers", func() {
			ginkgo.By("delete cluster")
			err := runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})
	})

	ginkgo.Context("check other dependencies", func() {
		ginkgo.It("Should not delete ns when cluster has addon", func() {
			ginkgo.By("create addon")
			addon := &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: cluster.Name,
				},
			}
			err := runtimeClient.Create(context.TODO(), addon)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			err = runtimeClient.Delete(context.TODO(), addon)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should not delete ns when cluster has hosted clusters", func() {
			ginkgo.By("set the  hostedClusterRequeuePeriod to 1 second")
			hostedClusterRequeuePeriod = 1 * time.Second
			ginkgo.By("create hostedcluster")
			err := util.CreateHostedCluster(hubDynamicClient, cluster.Name, cluster.Name)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// Use the runtime client, the same client the controller uses, to Wait for hostedcluster to exist,
			// otherwise this may fail occasionally
			gomega.Eventually(func() error {
				// _, err := util.GetHostedCluster(hubDynamicClient, cluster.Name, cluster.Name)
				// if err != nil {
				// 	return err
				// }
				hostedCluster := hyperv1beta1.HostedCluster{}
				return runtimeClient.Get(context.TODO(),
					types.NamespacedName{Name: cluster.Name, Namespace: cluster.Name}, &hostedCluster)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			err = util.DeleteHostedCluster(hubDynamicClient, cluster.Name, cluster.Name)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should not delete ns when cluster has clusterDeployment", func() {
			ginkgo.By("create clusterdeployment")
			err := util.CreateClusterDeployment(hubDynamicClient, cluster.Name)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// Wait for clusterDeployment to exist, otherwise this may fail occasionally in the local environment
			gomega.Eventually(func() error {
				_, err := util.GetClusterDeployment(hubDynamicClient, cluster.Name)
				return err
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			err = util.DeleteClusterDeployment(hubDynamicClient, cluster.Name)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should not delete ns when cluster has capi clusters", func() {
			ginkgo.By("create capi cluster")
			err := util.CreateCapiCluster(hubDynamicClient, cluster.Name, cluster.Name)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// Use the runtime client, the same client the controller uses, to Wait for hostedcluster to exist,
			// otherwise this may fail occasionally
			gomega.Eventually(func() error {
				capiCluster := capiv1beta1.Cluster{}
				return runtimeClient.Get(context.TODO(),
					types.NamespacedName{Name: cluster.Name, Namespace: cluster.Name}, &capiCluster)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			err = util.DeleteCapiCluster(hubDynamicClient, cluster.Name, cluster.Name)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should not delete ns when cluster has infraenv", func() {
			ginkgo.By("create infra env")
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
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			err = runtimeClient.Delete(context.TODO(), infra)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should not delete ns when cluster has clusterInstance", func() {
			ginkgo.By("create clusterInstance")
			clusterInstance := &siteconfigv1alpha1.ClusterInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-instance",
					Namespace: cluster.Name,
				},
				Spec: siteconfigv1alpha1.ClusterInstanceSpec{
					ClusterName:            cluster.Name,
					PullSecretRef:          corev1.LocalObjectReference{Name: "fake"},
					ClusterImageSetNameRef: "fake",
					BaseDomain:             "fake",
					Nodes: []siteconfigv1alpha1.NodeSpec{{
						BmcAddress:         "fake",
						BmcCredentialsName: siteconfigv1alpha1.BmcCredentialsName{Name: "fake"},
						BootMACAddress:     "AA:BB:CC:DD:EE:11",
						HostName:           "fake",
						TemplateRefs:       []siteconfigv1alpha1.TemplateRef{{Name: "fake", Namespace: "fake"}},
					}},
					TemplateRefs: []siteconfigv1alpha1.TemplateRef{{Name: "fake", Namespace: "fake"}},
				},
			}
			err := runtimeClient.Create(context.TODO(), clusterInstance)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			err = runtimeClient.Delete(context.TODO(), clusterInstance)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Should not delete ns when cluster has pods", func() {
			ginkgo.By("create pods")
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
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			_, err = k8sClient.CoreV1().Pods(cluster.Name).Create(context.TODO(), otherpod, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("update pods")
			gomega.Eventually(func() error {
				pod, err := k8sClient.CoreV1().Pods(cluster.Name).Get(context.TODO(), prehookpod.Name, metav1.GetOptions{})

				if err != nil {
					return err
				}

				pod.Status.Phase = corev1.PodRunning
				_, err = k8sClient.CoreV1().Pods(cluster.Name).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
				return err
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

			ginkgo.By("delete cluster")
			err = runtimeClient.Delete(context.TODO(), cluster)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("wait for 5 seconds and ns should not be deleted")
			time.Sleep(5 * time.Second)
			ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(ns.DeletionTimestamp.IsZero()).Should(gomega.BeTrue())

			err = k8sClient.CoreV1().Pods(cluster.Name).Delete(context.TODO(), otherpod.Name, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				pod, err := k8sClient.CoreV1().Pods(cluster.Name).Get(context.TODO(), prehookpod.Name, metav1.GetOptions{})

				if err != nil {
					return err
				}

				pod.Status.Phase = corev1.PodSucceeded
				_, err = k8sClient.CoreV1().Pods(cluster.Name).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
				return err
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				ns, err := k8sClient.CoreV1().Namespaces().Get(context.TODO(), cluster.Name, metav1.GetOptions{})

				if err == nil && !ns.DeletionTimestamp.IsZero() {
					return nil
				}

				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("namespace still exist with err: %v", err)
			}, timeout, interval).ShouldNot(gomega.HaveOccurred())
		})
	})
})
