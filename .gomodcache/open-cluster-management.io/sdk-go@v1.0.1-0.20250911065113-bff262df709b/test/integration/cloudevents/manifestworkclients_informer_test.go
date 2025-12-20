package cloudevents

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/agent/codec"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/agent"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/source"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("ManifestWork Clients Test - Informer based", func() {
	var err error

	var ctx context.Context
	var cancel context.CancelFunc

	var sourceID string
	var clusterName string
	var workName string

	var sourceClientHolder *work.ClientHolder
	var agentClientHolder *work.ClientHolder

	ginkgo.BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		sourceID = fmt.Sprintf("mw-test-%s", rand.String(5))
		clusterName = fmt.Sprintf("cluster-%s", rand.String(5))
		workName = fmt.Sprintf("work-%s", rand.String(5))

		sourceMQTTOptions := util.NewMQTTSourceOptionsWithSourceBroadcast(mqttBrokerHost, sourceID)
		sourceClientHolder, _, err = source.StartManifestWorkSourceClient(ctx, sourceID, sourceMQTTOptions)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		// wait for cache ready
		<-time.After(time.Second)

		agentMqttOptions := util.NewMQTTAgentOptionsWithSourceBroadcast(mqttBrokerHost, sourceID, clusterName)
		agentClientHolder, _, err = agent.StartWorkAgent(ctx, clusterName, agentMqttOptions, codec.NewManifestBundleCodec())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		// wait for cache ready
		<-time.After(time.Second)
	})

	ginkgo.AfterEach(func() {
		// cancel the context to stop the source client gracefully
		cancel()
	})

	ginkgo.Context("PubSub manifestworks", func() {
		ginkgo.It("CRUD manifestworks by manifestwork clients", func() {
			crudManifestWork(
				ctx,
				sourceClientHolder,
				agentClientHolder,
				sourceID,
				clusterName,
				workName,
				true,
			)
		})
	})

	ginkgo.Context("PubSub a manifestwork without version", func() {
		ginkgo.It("CRUD none-version manifestworks by manifestwork clients", func() {
			crudManifestWork(
				ctx,
				sourceClientHolder,
				agentClientHolder,
				sourceID,
				clusterName,
				workName,
				false,
			)
		})
	})
})

func crudManifestWork(
	ctx context.Context,
	sourceClientHolder *work.ClientHolder,
	agentClientHolder *work.ClientHolder,
	sourceID, clusterName, workName string,
	withVersion bool,
) {
	ginkgo.By("create a work with source client", func() {
		work := util.NewManifestWork(clusterName, workName, withVersion)
		_, err := sourceClientHolder.ManifestWorks(clusterName).Create(ctx, work, metav1.CreateOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.By("agent update the work status", func() {
		gomega.Eventually(func() error {
			workClient := agentClientHolder.ManifestWorks(clusterName)

			if err := util.AddWorkFinalizer(ctx, workClient, workName); err != nil {
				return err
			}

			return util.UpdateWorkStatus(ctx, workClient, workName, util.WorkCreatedCondition)
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("source update the work again", func() {
		gomega.Eventually(func() error {
			workClient := sourceClientHolder.ManifestWorks(clusterName)
			if err := util.AssertWorkStatus(ctx, workClient, workName, util.WorkCreatedCondition); err != nil {
				return err
			}

			return util.UpdateWork(ctx, workClient, workName, withVersion)
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("agent update the work status again", func() {
		gomega.Eventually(func() error {
			workClient := agentClientHolder.ManifestWorks(clusterName)
			if err := util.AssertUpdatedWork(ctx, workClient, workName); err != nil {
				return err
			}

			return util.UpdateWorkStatus(ctx, workClient, workName, util.WorkUpdatedCondition)
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("source mark the work is deleting", func() {
		gomega.Eventually(func() error {
			workClient := sourceClientHolder.ManifestWorks(clusterName)
			if err := util.AssertWorkStatus(ctx, workClient, workName, util.WorkUpdatedCondition); err != nil {
				return err
			}

			return workClient.Delete(ctx, workName, metav1.DeleteOptions{})
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("agent delete the work", func() {
		gomega.Eventually(func() error {
			workClient := agentClientHolder.ManifestWorks(clusterName)
			return util.RemoveWorkFinalizer(ctx, workClient, workName)
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
	})

	ginkgo.By("source delete the work", func() {
		gomega.Eventually(func() error {
			work, err := sourceClientHolder.WorkInterface().WorkV1().ManifestWorks(clusterName).Get(ctx, workName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}

			if err != nil {
				return err
			}

			return fmt.Errorf("the work is not deleted, %v", work.Status)
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
	})
}
