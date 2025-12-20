package cloudevents

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/agent/codec"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/agent"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/source"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"

	workv1client "open-cluster-management.io/api/client/work/clientset/versioned/typed/work/v1"
)

var _ = ginkgo.Describe("CloudEvents Options Racing Test - GRPC", func() {
	ginkgo.Context("GRPC options racing test", func() {
		var err error
		var ctx context.Context
		var cancel context.CancelFunc
		var sourceID string
		var resourceName string
		var agentOptions *grpc.GRPCOptions
		var sourceStore *store.MemoryStore
		var sourceCloudEventsClient generic.CloudEventsClient[*store.Resource]

		ginkgo.BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			sourceID = fmt.Sprintf("cloudevents-test-%s", rand.String(5))
			resourceName = fmt.Sprintf("resource-%s", rand.String(5))
			agentOptions = util.NewGRPCAgentOptions(certPool, grpcBrokerHost, tokenFile)
			sourceStore = store.NewMemoryStore()

			sourceOptions := grpc.NewSourceOptions(util.NewGRPCSourceOptions(grpcServerHost), sourceID)
			sourceCloudEventsClient, err = source.StartResourceSourceClient(
				ctx,
				sourceOptions,
				sourceID,
				source.NewResourceLister(sourceStore),
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

		})
		ginkgo.AfterEach(func() {
			// cancel the context to gracefully shutdown the agent
			cancel()
		})

		ginkgo.It("Start two work agents to test the racing condition of grpc connection", func() {
			var agentWorkClient1, agentWorkClient2 workv1client.ManifestWorkInterface
			clusterName1 := fmt.Sprintf("cluster1-%s", rand.String(5))
			clusterName2 := fmt.Sprintf("cluster2-%s", rand.String(5))
			ctx1, cancel1 := context.WithCancel(ctx)
			go func() {
				ginkgo.By("start a work agent")
				clientHolder, _, err := agent.StartWorkAgent(ctx1, clusterName1, agentOptions, codec.NewManifestBundleCodec())
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				agentWorkClient1 = clientHolder.ManifestWorks(clusterName1)
			}()

			go func() {
				ginkgo.By("start another work agent")
				clientHolder, _, err := agent.StartWorkAgent(ctx, clusterName2, agentOptions, codec.NewManifestBundleCodec())
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				agentWorkClient2 = clientHolder.ManifestWorks(clusterName2)
			}()

			time.Sleep(3 * time.Second) // sleep for the agents are subscribing to the broker

			ginkgo.By("create resource from source")
			resourceVersion := 1
			resource1 := store.NewResource(clusterName1, resourceName, int64(resourceVersion))
			sourceStore.Add(resource1)
			err = sourceCloudEventsClient.Publish(ctx, createRequest, resource1)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			resource2 := store.NewResource(clusterName2, resourceName, int64(resourceVersion))
			sourceStore.Add(resource2)
			err = sourceCloudEventsClient.Publish(ctx, createRequest, resource2)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("ensure the work can be got by work agent")
			gomega.Eventually(func() error {
				workName1 := store.ResourceID(clusterName1, resourceName)
				_, err := agentWorkClient1.Get(ctx, workName1, metav1.GetOptions{})
				if err != nil {
					return err
				}

				workName2 := store.ResourceID(clusterName2, resourceName)
				_, err = agentWorkClient2.Get(ctx, workName2, metav1.GetOptions{})
				if err != nil {
					return err
				}

				return nil
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("close the first work agent")
			cancel1()
			time.Sleep(1 * time.Second) // sleep for grpc connection is closed

			ginkgo.By("update the resource status by the second work agent")
			gomega.Eventually(func() error {
				workName2 := store.ResourceID(clusterName2, resourceName)
				if err := util.AddWorkFinalizer(ctx, agentWorkClient2, workName2); err != nil {
					return err
				}

				if err := util.AssertWorkFinalizers(ctx, agentWorkClient2, workName2); err != nil {
					return err
				}

				if err := util.UpdateWorkStatus(ctx, agentWorkClient2, workName2, util.WorkCreatedCondition); err != nil {
					return err
				}

				return nil
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("ensure the work status subscribed by source")
			gomega.Eventually(func() error {
				resource2, err = sourceStore.Get(store.ResourceID(clusterName2, resourceName))
				if err != nil {
					return err
				}

				if !meta.IsStatusConditionTrue(resource2.Status.Conditions, "Created") {
					return fmt.Errorf("unexpected status %v", resource2.Status.Conditions)
				}

				return nil
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("mark the resource deleting by source")
			resource2, err = sourceStore.Get(store.ResourceID(clusterName2, resourceName))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			resource2.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			err = sourceStore.Update(resource2)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			err = sourceCloudEventsClient.Publish(ctx, deleteRequest, resource2)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("delete the resource from agent")
			gomega.Eventually(func() error {
				workName := store.ResourceID(clusterName2, resourceName)
				return util.RemoveWorkFinalizer(ctx, agentWorkClient2, workName)
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("delete the resource from source")
			gomega.Eventually(func() error {
				resourceID := store.ResourceID(clusterName2, resourceName)
				resource2, err = sourceStore.Get(resourceID)
				if err != nil {
					return err
				}

				if meta.IsStatusConditionTrue(resource2.Status.Conditions, "Deleted") {
					sourceStore.Delete(resourceID)
				}

				return nil
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
		})
	})
})
