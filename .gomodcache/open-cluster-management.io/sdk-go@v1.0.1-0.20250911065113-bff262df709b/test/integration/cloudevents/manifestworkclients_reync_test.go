package cloudevents

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/common"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/utils"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/agent/codec"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/agent"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/source"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("ManifestWork Clients Test - Resync", func() {
	ginkgo.Context("Resync manifestworks", func() {
		var ctx context.Context
		var cancel context.CancelFunc

		var sourceID string
		var clusterName string
		var workNamePrefix string

		ginkgo.BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			sourceID = fmt.Sprintf("mw-resync-%s", rand.String(5))
			clusterName = fmt.Sprintf("resync-%s", rand.String(5))
			workNamePrefix = "resync"

			mqttOptions := util.NewMQTTAgentOptionsWithSourceBroadcast(mqttBrokerHost, sourceID, clusterName)
			_, informer, err := agent.StartWorkAgent(ctx, clusterName, mqttOptions, codec.NewManifestBundleCodec())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// wait for informer started
			<-time.After(time.Second)

			// add two works in the agent cache
			store := informer.Informer().GetStore()
			work1Name := fmt.Sprintf("%s-1", workNamePrefix)
			work1UID := utils.UID(sourceID, common.ManifestWorkGR.String(), clusterName, work1Name)
			work1 := util.NewManifestWorkWithStatus(clusterName, work1Name)
			work1.UID = apitypes.UID(work1UID)
			work1.ResourceVersion = "1"
			work1.Labels = map[string]string{common.CloudEventsOriginalSourceLabelKey: sourceID}
			work1.Annotations = map[string]string{common.CloudEventsDataTypeAnnotationKey: payload.ManifestBundleEventDataType.String()}
			gomega.Expect(store.Add(work1)).ToNot(gomega.HaveOccurred())

			work2Name := fmt.Sprintf("%s-2", workNamePrefix)
			work2UID := utils.UID(sourceID, common.ManifestWorkGR.String(), clusterName, work2Name)
			work2 := util.NewManifestWorkWithStatus(clusterName, work2Name)
			work2.UID = apitypes.UID(work2UID)
			work2.ResourceVersion = "1"
			work2.Labels = map[string]string{common.CloudEventsOriginalSourceLabelKey: sourceID}
			work2.Annotations = map[string]string{common.CloudEventsDataTypeAnnotationKey: payload.ManifestBundleEventDataType.String()}
			gomega.Expect(store.Add(work2)).ToNot(gomega.HaveOccurred())

			// wait for cache ready
			<-time.After(time.Second)
		})

		ginkgo.AfterEach(func() {
			// cancel the context to stop the source client gracefully
			cancel()
		})

		ginkgo.It("resync manifestworks with manifestwork source client", func() {

			mqttOptions := util.NewMQTTSourceOptionsWithSourceBroadcast(mqttBrokerHost, sourceID)

			// simulate a source client restart, recover two works
			sourceClientHolder, _, err := source.StartManifestWorkSourceClient(ctx, sourceID, mqttOptions)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = sourceClientHolder.ManifestWorks(clusterName).Create(
				ctx,
				util.NewManifestWork(clusterName, fmt.Sprintf("%s-1", workNamePrefix), true),
				metav1.CreateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = sourceClientHolder.ManifestWorks(clusterName).Create(
				ctx,
				util.NewManifestWork(clusterName, fmt.Sprintf("%s-2", workNamePrefix), true),
				metav1.CreateOptions{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("the manifestworks are synced", func() {
				gomega.Eventually(func() error {
					work1, err := sourceClientHolder.ManifestWorks(clusterName).Get(
						ctx, fmt.Sprintf("%s-1", workNamePrefix), metav1.GetOptions{})
					if err != nil {
						return err
					}
					if !meta.IsStatusConditionTrue(work1.Status.Conditions, "Created") {
						return fmt.Errorf("unexpected status %v", work1.Status.Conditions)
					}

					work2, err := sourceClientHolder.ManifestWorks(clusterName).Get(
						ctx, fmt.Sprintf("%s-2", workNamePrefix), metav1.GetOptions{})
					if err != nil {
						return err
					}
					if !meta.IsStatusConditionTrue(work2.Status.Conditions, "Created") {
						return fmt.Errorf("unexpected status %v", work2.Status.Conditions)
					}

					return nil
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})
		})
	})

})
