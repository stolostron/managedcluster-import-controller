package cloudevents

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	workv1 "open-cluster-management.io/api/work/v1"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/common"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/utils"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work"
	agentcodec "open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/agent/codec"
	sourcecodec "open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/source/codec"
	workstore "open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/mqtt"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/agent"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

var _ = ginkgo.Describe("ManifestWork Clients Test - Watch Only", func() {
	var ctx context.Context
	var cancel context.CancelFunc

	var sourceOptions *mqtt.MQTTOptions
	var sourceClient *work.ClientHolder

	var sourceID string
	var clusterName string
	var workName string

	ginkgo.BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		sourceID = fmt.Sprintf("watch-test-%s", rand.String(5))
		clusterName = fmt.Sprintf("watch-test-cluster-%s", rand.String(5))
		workName = fmt.Sprintf("watch-test-work-%s", rand.String(5))

		sourceOptions = util.NewMQTTSourceOptionsWithSourceBroadcast(mqttBrokerHost, sourceID)
	})

	ginkgo.AfterEach(func() {
		// cancel the context to stop the source client gracefully
		cancel()
	})

	ginkgo.Context("CRUD the manifestworks with source client", func() {
		ginkgo.JustBeforeEach(func() {
			watcherStore, err := workstore.NewSourceLocalWatcherStore(ctx, func(ctx context.Context) ([]*workv1.ManifestWork, error) {
				return []*workv1.ManifestWork{}, nil
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			clientID := fmt.Sprintf("%s-%s", sourceID, rand.String(5))
			opt := options.NewGenericClientOptions(sourceOptions, sourcecodec.NewManifestBundleCodec(), clientID).
				WithClientWatcherStore(watcherStore).
				WithSourceID(sourceID)
			sourceClient, err = work.NewSourceClientHolder(ctx, opt)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.It("create/update/delete a manifestwork with source client", func() {
			work := util.NewManifestWork(clusterName, workName, true)
			ginkgo.By("create a work with source client", func() {
				_, err := sourceClient.ManifestWorks(clusterName).Create(ctx, work, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = sourceClient.ManifestWorks(clusterName).Get(ctx, workName, metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("update the work with source client", func() {
				err := util.UpdateWork(ctx, sourceClient.ManifestWorks(clusterName), workName, true)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				updatedWork, err := sourceClient.ManifestWorks(clusterName).Get(ctx, workName, metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(len(updatedWork.Spec.Workload.Manifests)).To(gomega.BeEquivalentTo(2))
			})

			ginkgo.By("delete the work with source client", func() {
				// delete the work from source
				err := sourceClient.ManifestWorks(clusterName).Delete(ctx, workName, metav1.DeleteOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = sourceClient.ManifestWorks(clusterName).Get(ctx, workName, metav1.GetOptions{})
				gomega.Expect(errors.IsNotFound(err)).To(gomega.BeTrue())
			})
		})
	})

	ginkgo.Context("Watching the manifestworks with source client", func() {
		// this store save the works that are watched by source client
		var localStore cache.Store

		var agentClient *work.ClientHolder

		ginkgo.JustBeforeEach(func() {
			localStore = cache.NewStore(cache.MetaNamespaceKeyFunc)

			watcherStore, err := workstore.NewSourceLocalWatcherStore(ctx, func(ctx context.Context) ([]*workv1.ManifestWork, error) {
				return []*workv1.ManifestWork{}, nil
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			clientID := fmt.Sprintf("%s-%s", sourceID, rand.String(5))
			opt := options.NewGenericClientOptions(sourceOptions, sourcecodec.NewManifestBundleCodec(), clientID).
				WithClientWatcherStore(watcherStore).
				WithSourceID(sourceID)
			sourceClient, err = work.NewSourceClientHolder(ctx, opt)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			agentOptions := util.NewMQTTAgentOptionsWithSourceBroadcast(mqttBrokerHost, sourceID, clusterName)
			agentClient, _, err = agent.StartWorkAgent(ctx, clusterName, agentOptions, agentcodec.NewManifestBundleCodec())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			watcher, err := sourceClient.ManifestWorks(metav1.NamespaceAll).Watch(ctx, metav1.ListOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			go func() {
				ch := watcher.ResultChan()
				for {
					select {
					case <-ctx.Done():
						return
					case event, ok := <-ch:
						if !ok {
							return
						}

						switch event.Type {
						case watch.Added:
							err := localStore.Add(event.Object)
							gomega.Expect(err).ToNot(gomega.HaveOccurred())
						case watch.Modified:
							err := localStore.Update(event.Object)
							gomega.Expect(err).ToNot(gomega.HaveOccurred())
						case watch.Deleted:
							err := localStore.Delete(event.Object)
							gomega.Expect(err).ToNot(gomega.HaveOccurred())
						}
					}
				}
			}()
		})

		ginkgo.It("watch the Added/Modified/Deleted events", func() {
			work := util.NewManifestWork(clusterName, workName, true)
			ginkgo.By("create a work with source client", func() {
				_, err := sourceClient.ManifestWorks(clusterName).Create(ctx, work, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = sourceClient.ManifestWorks(clusterName).Get(ctx, workName, metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				// this created work should be watched
				gomega.Eventually(func() error {
					_, exists, err := localStore.GetByKey(clusterName + "/" + workName)
					if err != nil {
						return err
					}
					if !exists {
						return fmt.Errorf("the new work is not watched")
					}

					return nil
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			ginkgo.By("update a work with source client", func() {
				err := util.UpdateWork(ctx, sourceClient.ManifestWorks(clusterName), workName, true)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				gomega.Eventually(func() error {
					workClient := agentClient.ManifestWorks(clusterName)

					if err := util.AddWorkFinalizer(ctx, workClient, workName); err != nil {
						return err
					}

					return util.UpdateWorkStatus(ctx, workClient, workName, util.WorkCreatedCondition)
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

				// this updated work should be watched
				gomega.Eventually(func() error {
					work, exists, err := localStore.GetByKey(clusterName + "/" + workName)
					if err != nil {
						return err
					}
					if !exists {
						return fmt.Errorf("the work does not exist")
					}

					if len(work.(*workv1.ManifestWork).Spec.Workload.Manifests) != 2 {
						return fmt.Errorf("the updated work is not watched")
					}

					if len(work.(*workv1.ManifestWork).Status.Conditions) == 0 {
						return fmt.Errorf("the updated work status is not watched")
					}

					return nil
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})

			ginkgo.By("delete the work with source client", func() {
				// delete the work from source
				err := sourceClient.ManifestWorks(clusterName).Delete(ctx, workName, metav1.DeleteOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				// agent delete the work and send deleted status to source
				gomega.Eventually(func() error {
					workClient := agentClient.ManifestWorks(clusterName)
					return util.RemoveWorkFinalizer(ctx, workClient, workName)
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

				// the deleted work should be watched
				gomega.Eventually(func() error {
					_, exists, err := localStore.GetByKey(clusterName + "/" + workName)
					if err != nil {
						return err
					}
					if exists {
						return fmt.Errorf("the work is not deleted")
					}

					return nil
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})
		})
	})

	ginkgo.Context("Resync the manifestworks by agent", func() {
		ginkgo.JustBeforeEach(func() {
			watcherStore, err := workstore.NewSourceLocalWatcherStore(ctx, func(ctx context.Context) ([]*workv1.ManifestWork, error) {
				work := util.NewManifestWork(clusterName, workName, false)
				work.UID = apitypes.UID(utils.UID(sourceID, common.ManifestWorkGR.String(), clusterName, workName))
				work.ResourceVersion = "0"
				work.Spec.Workload.Manifests = []workv1.Manifest{util.NewManifest("test1")}
				work.Status.Conditions = []metav1.Condition{{Type: "Created", Status: metav1.ConditionTrue}}

				if err := utils.EncodeManifests(work); err != nil {
					return nil, err
				}
				return []*workv1.ManifestWork{work}, nil
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			clientID := fmt.Sprintf("%s-%s", sourceID, rand.String(5))
			opt := options.NewGenericClientOptions(sourceOptions, sourcecodec.NewManifestBundleCodec(), clientID).
				WithClientWatcherStore(watcherStore).
				WithSourceID(sourceID)
			_, err = work.NewSourceClientHolder(ctx, opt)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.It("agent resync the source manifestworks", func() {
			agentOptions := util.NewMQTTAgentOptionsWithSourceBroadcast(mqttBrokerHost, sourceID, clusterName)
			agentClient, _, err := agent.StartWorkAgent(ctx, clusterName, agentOptions, agentcodec.NewManifestBundleCodec())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("get the work from agent", func() {
				gomega.Eventually(func() error {
					_, err := agentClient.ManifestWorks(clusterName).Get(
						ctx, workName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					return nil
				}, 10*time.Second, 1*time.Second).Should(gomega.Succeed())
			})
		})
	})

	ginkgo.Context("Using the source client with customized lister", func() {
		// simulate a server to receive the works from source and works status from work agent
		var server cache.Store
		var serverListFn workstore.ListLocalWorksFunc

		var sourceClient *work.ClientHolder

		var manifestWork *workv1.ManifestWork

		ginkgo.BeforeEach(func() {
			server = cache.NewStore(cache.MetaNamespaceKeyFunc)

			serverListFn = func(ctx context.Context) ([]*workv1.ManifestWork, error) {
				works := []*workv1.ManifestWork{}
				for _, obj := range server.List() {
					if work, ok := obj.(*workv1.ManifestWork); ok {
						if len(work.Status.Conditions) != 0 {
							works = append(works, work)
						}
					}
				}
				return works, nil
			}

			manifestWork = util.NewManifestWork(clusterName, workName, true)
		})

		ginkgo.It("restart a source client with customized lister", func() {
			ginkgo.By("start a source client with customized lister", func() {
				ctx, cancel = context.WithCancel(context.Background())

				watcherStore, err := workstore.NewSourceLocalWatcherStore(ctx, serverListFn)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				clientID := fmt.Sprintf("%s-%s", sourceID, rand.String(5))
				opt := options.NewGenericClientOptions(sourceOptions, sourcecodec.NewManifestBundleCodec(), clientID).
					WithClientWatcherStore(watcherStore).
					WithSourceID(sourceID)
				sourceClient, err = work.NewSourceClientHolder(ctx, opt)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("create a work by source client", func() {
				created, err := sourceClient.ManifestWorks(clusterName).Create(ctx, manifestWork, metav1.CreateOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				// simulate the server receive this work
				err = server.Add(created)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("stop the source client", func() {
				cancel()
			})

			ginkgo.By("update the work status on the server after source client is down", func() {
				// simulate the server receive this work status from agent
				obj, exists, err := server.GetByKey(clusterName + "/" + workName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(exists).To(gomega.BeTrue())

				work, ok := obj.(*workv1.ManifestWork)
				gomega.Expect(ok).To(gomega.BeTrue())

				work.Status.Conditions = []metav1.Condition{{Type: "Created", Status: metav1.ConditionTrue}}
				err = server.Update(work)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			// start the source client again
			ginkgo.By("restart a source client with customized lister", func() {
				ctx, cancel = context.WithCancel(context.Background())

				watcherStore, err := workstore.NewSourceLocalWatcherStore(ctx, serverListFn)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				clientID := fmt.Sprintf("%s-%s", sourceID, rand.String(5))
				opt := options.NewGenericClientOptions(sourceOptions, sourcecodec.NewManifestBundleCodec(), clientID).
					WithClientWatcherStore(watcherStore).
					WithSourceID(sourceID)
				sourceClient, err = work.NewSourceClientHolder(ctx, opt)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("the source client is able to get the latest work status", func() {
				found, err := sourceClient.ManifestWorks(clusterName).Get(ctx, workName, metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(meta.IsStatusConditionTrue(found.Status.Conditions, "Created")).To(gomega.BeTrue())
			})
		})
	})
})
