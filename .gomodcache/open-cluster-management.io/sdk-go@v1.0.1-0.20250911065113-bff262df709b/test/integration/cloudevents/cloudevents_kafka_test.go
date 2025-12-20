//go:build kafka

package cloudevents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	kafkav2 "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"

	workv1 "open-cluster-management.io/api/work/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/agent/codec"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	workstore "open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/kafka"
	kafkaoptions "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/kafka"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/source"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
)

var _ = ginkgo.Describe("CloudEvents Clients Test - Kafka", func() {
	var err error

	var ctx context.Context
	var cancel context.CancelFunc

	var kafkaCluster *confluentkafka.MockCluster
	var kafkaOptions *kafka.KafkaOptions

	ginkgo.BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		kafkaCluster, err = kafkav2.NewMockCluster(1)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		kafkaOptions = &kafka.KafkaOptions{
			ConfigMap: confluentkafka.ConfigMap{
				"bootstrap.servers": kafkaCluster.BootstrapServers(),
			},
		}
	})

	ginkgo.AfterEach(func() {
		cancel()

		kafkaCluster.Close()
	})

	ginkgo.It("publish event from source to agent", func() {
		ginkgo.By("Start an agent on cluster1")
		clusterName := "cluster1"
		agentCtx, agentCancel := context.WithCancel(context.Background())
		agentID := clusterName + "-" + rand.String(5)
		watcherStore := workstore.NewAgentInformerWatcherStore()

		opt := options.NewGenericClientOptions(kafkaOptions, codec.NewManifestBundleCodec(), agentID).
			WithClientWatcherStore(watcherStore).
			WithClusterName(clusterName)
		agentClientHolder, err := work.NewAgentClientHolder(agentCtx, opt)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		factory := workinformers.NewSharedInformerFactoryWithOptions(
			agentClientHolder.WorkInterface(),
			5*time.Minute,
			workinformers.WithNamespace(clusterName),
		)
		informer := factory.Work().V1().ManifestWorks()
		watcherStore.SetInformer(informer.Informer())
		go informer.Informer().Run(ctx.Done())

		agentManifestClient := agentClientHolder.ManifestWorks(clusterName)

		ginkgo.By("Start a source cloudevent client")
		sourceStoreLister := NewResourceLister()
		sourceOptions := &kafka.KafkaOptions{
			ConfigMap: kafkav2.ConfigMap{
				"bootstrap.servers": kafkaCluster.BootstrapServers(),
			},
		}
		sourceCloudEventClient, err := generic.NewCloudEventSourceClient[*store.Resource](
			ctx,
			kafkaoptions.NewSourceOptions(sourceOptions, "source1"),
			sourceStoreLister,
			source.StatusHashGetter,
			&source.ResourceCodec{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By("Subscribe agent topics to update resource status")
		sourceCloudEventClient.Subscribe(ctx, func(action types.ResourceAction, resource *store.Resource) error {
			return sourceStoreLister.store.UpdateStatus(resource)
		})

		ginkgo.By("Publish manifest from source to agent")
		var manifestWork *workv1.ManifestWork
		var resourceName1 string
		gomega.Eventually(func() error {
			ginkgo.By("Create the manifest resource and publish it to agent")
			resourceName := "resource-" + rand.String(5)
			newResource := store.NewResource(clusterName, resourceName, 1)
			err = sourceCloudEventClient.Publish(ctx, types.CloudEventsType{
				CloudEventsDataType: payload.ManifestBundleEventDataType,
				SubResource:         types.SubResourceSpec,
				Action:              "test_create_request",
			}, newResource)
			if err != nil {
				return err
			}

			// wait until the agent receive manifestworks
			time.Sleep(2 * time.Second)

			// ensure the work can be get by work client
			workName := store.ResourceID(clusterName, resourceName)
			manifestWork, err = agentManifestClient.Get(ctx, workName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			// add to the source store if the resource is synced successfully,
			sourceStoreLister.store.Add(newResource)
			resourceName1 = resourceName

			return nil
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

		ginkgo.By("Update the resource status on the agent cluster")
		newWork := manifestWork.DeepCopy()
		newWork.Status = workv1.ManifestWorkStatus{
			Conditions: []metav1.Condition{{
				Type:   "Created",
				Status: metav1.ConditionTrue,
			}},
		}

		oldData, err := json.Marshal(manifestWork)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		newData, err := json.Marshal(newWork)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By("Report(updating) the resource status from agent cluster to source cluster")
		_, err = agentManifestClient.Patch(ctx, manifestWork.Name, apitypes.MergePatchType, patchBytes, metav1.PatchOptions{}, "status")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		ginkgo.By("Verify the resource status is synced to the source cluster")
		gomega.Eventually(func() error {
			storeResource, err := sourceStoreLister.store.Get(manifestWork.Name)
			if err != nil {
				return err
			}
			if !meta.IsStatusConditionTrue(storeResource.Status.Conditions, "Created") {
				return fmt.Errorf("unexpected status %v", storeResource.Status.Conditions)
			}
			return nil
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

		ginkgo.By("Resync resource from source with a new agent instance")
		agentCancel()
		// wait until the consumer is closed
		time.Sleep(2 * time.Second)

		newAgentCtx, newAgentCancel := context.WithCancel(context.Background())
		defer newAgentCancel()
		// Note: Different configuration for the new agent will have different behavior
		//   Case1: new agentID(group.id) + "auto.offset.reset": latest
		//        The agent has to wait until the consumer is ready to send message, like time.Sleep(5 * time.Second)
		//   Case2: keep the same agentID for the new agent, it will ignore the "auto.offset.reset" automatically
		//        Then we don't need wait the consumer ready, cause it will consume message from last committed
		//        But the agent will wait a long time(test result is 56 seconds) to receive the message
		agentID = clusterName + "-" + rand.String(5)
		_ = kafkaOptions.ConfigMap.SetKey("group.id", agentID)
		watcherStore = workstore.NewAgentInformerWatcherStore()
		opt = options.NewGenericClientOptions(kafkaOptions, codec.NewManifestBundleCodec(), agentID).
			WithClientWatcherStore(watcherStore).
			WithClusterName(clusterName)
		newAgentHolder, err := work.NewAgentClientHolder(newAgentCtx, opt)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		factory = workinformers.NewSharedInformerFactoryWithOptions(
			agentClientHolder.WorkInterface(),
			5*time.Minute,
			workinformers.WithNamespace(clusterName),
		)
		informer = factory.Work().V1().ManifestWorks()

		watcherStore.SetInformer(informer.Informer())

		// case1: wait until the consumer is ready
		time.Sleep(5 * time.Second)
		go informer.Informer().Run(newAgentCtx.Done())
		newAgentManifestClient := newAgentHolder.ManifestWorks(clusterName)

		gomega.Eventually(func() error {
			workName1 := store.ResourceID(clusterName, resourceName1)
			if _, err := newAgentManifestClient.Get(ctx, workName1, metav1.GetOptions{}); err != nil {
				return err
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
})

type resourceLister struct {
	store *store.MemoryStore
}

var _ generic.Lister[*store.Resource] = &resourceLister{}

func NewResourceLister() *resourceLister {
	return &resourceLister{
		store: store.NewMemoryStore(),
	}
}

func (resLister *resourceLister) List(listOpts types.ListOptions) ([]*store.Resource, error) {
	return resLister.store.List(listOpts.ClusterName), nil
}
