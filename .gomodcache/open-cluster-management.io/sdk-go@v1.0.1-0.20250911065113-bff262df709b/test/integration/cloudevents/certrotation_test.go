package cloudevents

import (
	"context"
	"fmt"
	"os"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"

	workv1 "open-cluster-management.io/api/work/v1"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/pkg/testing"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/util"
)

const certDuration = 5 * time.Second

func runCloudeventsCertRotationTest(getAgentOptionsFn GetAgentOptionsFn) func() {
	return func() {
		var ctx context.Context
		var cancel context.CancelFunc

		var agentID string
		var clusterName = "cert-rotation-test"

		var clientCertFile *os.File
		var clientKeyFile *os.File

		ginkgo.BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			agentID = fmt.Sprintf("%s-%s", clusterName, rand.String(5))

			clientCertPairs, err := util.SignClientCert(serverCertPairs.CA, serverCertPairs.CAKey, certDuration)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			clientCertFile, err = testing.WriteToTempFile("client-cert-*.pem", clientCertPairs.ClientCert)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			clientKeyFile, err = testing.WriteToTempFile("client-key-*.pem", clientCertPairs.ClientKey)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.AfterEach(func() {
			cancel()

			if clientCertFile != nil {
				os.Remove(clientCertFile.Name())
			}

			if clientKeyFile != nil {
				os.Remove(clientKeyFile.Name())
			}
		})

		ginkgo.It("Should be able to send events after the client cert renewed", func() {
			ginkgo.By("Create an agent client with short time cert")
			agentOptions := getAgentOptionsFn(ctx, agentID, clusterName, clientCertFile.Name(), clientKeyFile.Name())
			agentClient, err := generic.NewCloudEventAgentClient(
				ctx,
				agentOptions,
				nil,
				nil,
				&resourceCodec{},
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			evtType := types.CloudEventsType{
				CloudEventsDataType: payload.ManifestBundleEventDataType,
				SubResource:         types.SubResourceStatus,
				Action:              types.CreateRequestAction,
			}

			ginkgo.By("Publishes an event")
			err = agentClient.Publish(ctx, evtType, &store.Resource{ResourceID: "test-resource", Namespace: clusterName, ResourceVersion: 1})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("Renew the client cert")
			newClientCertPairs, err := util.SignClientCert(serverCertPairs.CA, serverCertPairs.CAKey, 60*time.Second)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = os.WriteFile(clientCertFile.Name(), newClientCertPairs.ClientCert, 0o644)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = os.WriteFile(clientKeyFile.Name(), newClientCertPairs.ClientKey, 0o644)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("Wait for the first cert to expire (10s)")
			<-time.After(certDuration * 2)

			ginkgo.By("Publishes an event again")
			err = agentClient.Publish(ctx, evtType, &store.Resource{ResourceID: "test-resource", Namespace: clusterName, ResourceVersion: 1})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	}
}

type resourceCodec struct{}

func (c *resourceCodec) EventDataType() types.CloudEventsDataType {
	return payload.ManifestBundleEventDataType
}

func (c *resourceCodec) Encode(source string, eventType types.CloudEventsType, resource *store.Resource) (*cloudevents.Event, error) {
	evt := types.NewEventBuilder(source, eventType).NewEvent()
	evt.SetExtension(types.ExtensionClusterName, resource.Namespace)
	evt.SetExtension(types.ExtensionResourceID, resource.ResourceID)
	evt.SetExtension(types.ExtensionResourceVersion, resource.ResourceVersion)
	manifestBundle := &payload.ManifestBundle{
		Manifests: []workv1.Manifest{
			{
				RawExtension: runtime.RawExtension{
					Object: &resource.Spec,
				},
			},
		},
	}
	if err := evt.SetData(cloudevents.ApplicationJSON, manifestBundle); err != nil {
		return nil, fmt.Errorf("failed to encode manifests to cloud event: %v", err)
	}

	return &evt, nil
}

func (c *resourceCodec) Decode(evt *cloudevents.Event) (*store.Resource, error) {
	// do nothing
	return nil, nil
}
