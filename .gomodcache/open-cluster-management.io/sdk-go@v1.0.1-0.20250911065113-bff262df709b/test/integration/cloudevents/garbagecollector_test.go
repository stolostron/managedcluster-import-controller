package cloudevents

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/metadata"
	fakemetadata "k8s.io/client-go/metadata/fake"

	workv1informers "open-cluster-management.io/api/client/work/informers/externalversions/work/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/agent/codec"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/garbagecollector"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/agent"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/source"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("Garbage Collector Test", func() {
	ginkgo.Context("Publish a manifestwork with owner reference", func() {
		var err error

		var ctx context.Context
		var cancel context.CancelFunc

		var sourceID string
		var clusterName string
		var workName1 string
		var workName2 string

		var sourceClientHolder *work.ClientHolder
		var agentClientHolder *work.ClientHolder
		var informer workv1informers.ManifestWorkInformer
		var metadataClient metadata.Interface

		ginkgo.BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			sourceID = fmt.Sprintf("gc-test-%s", rand.String(5))
			clusterName = fmt.Sprintf("gc-%s", rand.String(5))
			workName1 = "test1"
			workName2 = "test2"

			scheme := fakemetadata.NewTestScheme()
			err = metav1.AddMetaToScheme(scheme)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			cm := &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: clusterName,
					UID:       "123",
					Labels:    map[string]string{"test": "test"},
				},
			}
			srt := &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: clusterName,
					UID:       "456",
					Labels:    map[string]string{"test": "test"},
				},
			}
			metadataClient = fakemetadata.NewSimpleMetadataClient(scheme, cm, srt)

			sourceMQTTOptions := util.NewMQTTSourceOptionsWithSourceBroadcast(mqttBrokerHost, sourceID)
			sourceClientHolder, informer, err = source.StartManifestWorkSourceClient(ctx, sourceID, sourceMQTTOptions)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			listOptions := &metav1.ListOptions{
				LabelSelector: "test=test",
				FieldSelector: "metadata.name=test",
			}
			ownerGVRFilters := map[schema.GroupVersionResource]*metav1.ListOptions{
				corev1.SchemeGroupVersion.WithResource("configmaps"): listOptions,
				corev1.SchemeGroupVersion.WithResource("secrets"):    listOptions,
			}
			garbageCollector := garbagecollector.NewGarbageCollector(
				sourceClientHolder,
				informer,
				metadataClient,
				ownerGVRFilters,
			)
			go garbageCollector.Run(ctx, 1)

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

		ginkgo.It("CRUD a manifestwork with manifestwork source client and agent client", func() {
			cmVGR := corev1.SchemeGroupVersion.WithResource("configmaps")
			srtGVR := corev1.SchemeGroupVersion.WithResource("secrets")
			cmObj, err := metadataClient.Resource(corev1.SchemeGroupVersion.WithResource("configmaps")).
				Namespace(clusterName).
				Get(ctx, "test", metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			srtObj, err := metadataClient.Resource(corev1.SchemeGroupVersion.WithResource("secrets")).
				Namespace(clusterName).
				Get(ctx, "test", metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			work1 := util.NewManifestWork(clusterName, workName1, true)
			work2 := util.NewManifestWork(clusterName, workName2, true)
			pTrue := true
			ownerReference1 := metav1.OwnerReference{
				APIVersion:         corev1.SchemeGroupVersion.String(),
				Kind:               "ConfigMap",
				Name:               cmObj.Name,
				UID:                cmObj.UID,
				BlockOwnerDeletion: &pTrue,
			}
			ownerReference2 := metav1.OwnerReference{
				APIVersion:         corev1.SchemeGroupVersion.String(),
				Kind:               "Secret",
				Name:               srtObj.Name,
				UID:                srtObj.UID,
				BlockOwnerDeletion: &pTrue,
			}
			work1.SetOwnerReferences([]metav1.OwnerReference{ownerReference1})
			work2.SetOwnerReferences([]metav1.OwnerReference{ownerReference1, ownerReference2})

			ginkgo.By("create work with owner by source client", func() {
				_, err := sourceClientHolder.ManifestWorks(clusterName).Create(ctx, work1, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				_, err = sourceClientHolder.ManifestWorks(clusterName).Create(ctx, work2, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("agent update the work status", func() {
				gomega.Eventually(func() error {
					workClient := agentClientHolder.ManifestWorks(clusterName)
					if err := util.AddWorkFinalizer(ctx, workClient, workName1); err != nil {
						return err
					}

					if err := util.UpdateWorkStatus(ctx, workClient, workName1, util.WorkCreatedCondition); err != nil {
						return err
					}

					if err := util.AddWorkFinalizer(ctx, workClient, workName2); err != nil {
						return err
					}

					return util.UpdateWorkStatus(ctx, workClient, workName2, util.WorkCreatedCondition)
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			ginkgo.By("source check the work status", func() {
				gomega.Eventually(func() error {
					workClient := sourceClientHolder.ManifestWorks(clusterName)
					if err := util.AssertWorkStatus(ctx, workClient, workName1, util.WorkCreatedCondition); err != nil {
						return err
					}

					return util.AssertWorkStatus(ctx, workClient, workName2, util.WorkCreatedCondition)
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			ginkgo.By("delete namespace-scoped owner of work from source", func() {
				// envtest does't have GC controller
				err := metadataClient.Resource(cmVGR).Namespace(clusterName).Delete(ctx, cmObj.Name, metav1.DeleteOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("agent delete the first work with single owner", func() {
				gomega.Eventually(func() error {
					return util.RemoveWorkFinalizer(ctx, agentClientHolder.ManifestWorks(clusterName), workName1)
				}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			ginkgo.By("source check the work deletion with single owner", func() {
				gomega.Eventually(func() error {
					work1, err = sourceClientHolder.WorkInterface().WorkV1().ManifestWorks(clusterName).Get(ctx, workName1, metav1.GetOptions{})
					if err == nil || !errors.IsNotFound(err) {
						return fmt.Errorf("the work %s/%s is not deleted", work1.GetNamespace(), work1.GetName())
					}

					work2, err = sourceClientHolder.WorkInterface().WorkV1().ManifestWorks(clusterName).Get(ctx, workName2, metav1.GetOptions{})
					if err != nil {
						return err
					}
					if len(work2.GetOwnerReferences()) != 1 {
						return fmt.Errorf("unexpected owner references (%v) for the work %s/%s", work2.GetOwnerReferences(), work2.GetNamespace(), work2.GetName())
					}

					return nil
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			ginkgo.By("delete cluster-scoped owner of work from source", func() {
				// envtest does't have GC controller
				err = metadataClient.Resource(srtGVR).Namespace(clusterName).Delete(ctx, srtObj.Name, metav1.DeleteOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("agent delete the work with two owners", func() {
				gomega.Eventually(func() error {
					return util.RemoveWorkFinalizer(ctx, agentClientHolder.ManifestWorks(clusterName), workName2)
				}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			ginkgo.By("source check the work deletion with two owners", func() {
				gomega.Eventually(func() error {
					work2, err = sourceClientHolder.WorkInterface().WorkV1().ManifestWorks(clusterName).Get(ctx, workName2, metav1.GetOptions{})
					if err == nil || !errors.IsNotFound(err) {
						return fmt.Errorf("the work %s/%s is not deleted", work2.GetNamespace(), work2.GetName())
					}
					return nil
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})
		})
	})
})
