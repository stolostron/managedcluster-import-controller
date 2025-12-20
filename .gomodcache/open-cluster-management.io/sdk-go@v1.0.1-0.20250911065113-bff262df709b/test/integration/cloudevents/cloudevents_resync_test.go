package cloudevents

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/rand"

	workv1 "open-cluster-management.io/api/work/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/agent/codec"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/mqtt"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/agent"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/source"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("CloudEvents Clients Test - RESYNC", func() {
	var err error

	var ctx context.Context
	var cancel context.CancelFunc

	var sourceStore *store.MemoryStore

	var sourceID string
	var clusterName string
	var resourceName string

	var mqttOptions *mqtt.MQTTOptions

	var sourceCloudEventsClient generic.CloudEventsClient[*store.Resource]

	ginkgo.BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		sourceID = fmt.Sprintf("cloudevents-resync-%s", rand.String(5))
		clusterName = fmt.Sprintf("cluster-%s", rand.String(5))
		resourceName = fmt.Sprintf("resource-%s", rand.String(5))

		sourceStore = store.NewMemoryStore()

		mqttOptions = util.NewMQTTAgentOptions(mqttBrokerHost, sourceID, clusterName)

		sourceCloudEventsClient, err = source.StartResourceSourceClient(
			ctx,
			mqtt.NewSourceOptions(
				util.NewMQTTSourceOptions(mqttBrokerHost, sourceID),
				fmt.Sprintf("%s-client", sourceID),
				sourceID,
			),
			sourceID,
			source.NewResourceLister(sourceStore),
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		// cancel the context to gracefully shutdown the agent
		cancel()
	})

	ginkgo.Context("Resync resources", func() {
		ginkgo.It("resync resources between source and agent", func() {
			ginkgo.By("create a resource by source client")
			res := store.NewResource(clusterName, resourceName, 1)
			sourceStore.Add(res)
			err := sourceCloudEventsClient.Publish(ctx, createRequest, res)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("start a work agent to resync the resources from agent")
			clientHolder, informer, err := agent.StartWorkAgent(ctx, clusterName, mqttOptions, codec.NewManifestBundleCodec())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			lister := informer.Lister().ManifestWorks(clusterName)
			agentWorkClient := clientHolder.ManifestWorks(clusterName)

			ginkgo.By("ensure the resources is synced on the agent")
			gomega.Eventually(func() error {
				list, err := lister.List(labels.Everything())
				if err != nil {
					return err
				}

				// ensure there is only one work was synced on the cluster1
				if len(list) != 1 {
					return fmt.Errorf("unexpected work list %v", list)
				}

				return nil
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("update the status on the agent")
			gomega.Eventually(func() error {
				workName := store.ResourceID(clusterName, resourceName)
				work, err := agentWorkClient.Get(ctx, workName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				newWork := work.DeepCopy()
				newWork.Status = workv1.ManifestWorkStatus{Conditions: []metav1.Condition{util.WorkCreatedCondition}}

				// only update the status on the agent local part
				store := informer.Informer().GetStore()
				if err := store.Update(newWork); err != nil {
					return err
				}

				return nil
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("resync the status from source")
			err = sourceCloudEventsClient.Resync(ctx, clusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("ensure the resource status is synced on the source")
			gomega.Eventually(func() error {
				resource, err := sourceStore.Get(store.ResourceID(clusterName, resourceName))
				if err != nil {
					return err
				}

				if !meta.IsStatusConditionTrue(resource.Status.Conditions, "Created") {
					return fmt.Errorf("unexpected status %v", resource.Status.Conditions)
				}

				return nil
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
		})
	})
})
