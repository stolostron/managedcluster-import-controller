package metrics

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	k8smetrics "k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	k8smetricstest "k8s.io/component-base/metrics/testutil"
	"k8s.io/klog/v2"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	pbv1 "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc/protobuf/v1"
	grpcprotocol "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc/protocol"
	cetypes "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/server"
	cegrpc "open-cluster-management.io/sdk-go/pkg/cloudevents/server/grpc"

	cemetrics "open-cluster-management.io/sdk-go/pkg/cloudevents/server/grpc/metrics"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func startBufServer(t *testing.T) *grpc.Server {
	promMiddleware := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets(k8smetrics.ExponentialBuckets(10e-7, 10, 10)),
		),
	)

	server := grpc.NewServer(
		grpc.UnaryInterceptor(NewGRPCMetricsUnaryInterceptor(promMiddleware)),
		grpc.StreamInterceptor(NewGRPCMetricsStreamInterceptor(promMiddleware)),
		grpc.StatsHandler(NewGRPCMetricsHandler()),
	)

	grpcBroker := cegrpc.NewGRPCBroker()
	grpcBroker.RegisterService(payload.ManifestBundleEventDataType, newMockWorkService())
	pbv1.RegisterCloudEventServiceServer(server, grpcBroker)

	RegisterGRPCMetrics(promMiddleware, cemetrics.CloudEventsGRPCMetrics()...)
	promMiddleware.InitializeMetrics(server)

	lis = bufconn.Listen(bufSize)
	go func() {
		if err := server.Serve(lis); err != nil {
			klog.Fatalf("Server exited with error: %v", err)
		}
	}()
	return server
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func TestGRPCMetricsInterceptor(t *testing.T) {
	server := startBufServer(t)
	defer server.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", //nolint:staticcheck // DialContext is deprecated but fine for tests
		grpc.WithContextDialer(bufDialer),
		grpc.WithInsecure(), //nolint:staticcheck // WithInsecure is deprecated but fine for tests
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	client := pbv1.NewCloudEventServiceClient(conn)
	_, err = client.Subscribe(ctx, &pbv1.SubscriptionRequest{ClusterName: "cluster1", DataType: "io.open-cluster-management.works.v1alpha1.manifestbundles"})
	if err != nil {
		t.Fatalf("failed to call Subscribe: %v", err)
	}

	evt := &cloudevents.Event{}
	if err := json.Unmarshal([]byte(testCloudEventJSON), evt); err != nil {
		t.Fatalf("failed to unmarshal cloud event: %v", err)
	}

	pbEvt := &pbv1.CloudEvent{}
	if err = grpcprotocol.WritePBMessage(ctx, binding.ToMessage(evt), pbEvt); err != nil {
		t.Fatalf("failed to convert spec from cloudevent to protobuf: %v", err)
	}

	if _, err = client.Publish(ctx, &pbv1.PublishRequest{Event: pbEvt}); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	gRPCMetrics := []string{
		"grpc_server_active_connections",
		"grpc_server_started_total",
		"grpc_server_msg_received_total",
		"grpc_server_msg_sent_total",
		"grpc_server_msg_received_bytes_total",
		"grpc_server_msg_sent_bytes_total",
		"grpc_server_handled_total",
		"grpc_server_ce_called_total",
		"grpc_server_ce_msg_received_total",
		"grpc_server_ce_msg_sent_total",
		"grpc_server_ce_processed_total",
	}

	// assert gaugge and counter metrics
	if err := k8smetricstest.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(expectedMetrics), gRPCMetrics...); err != nil {
		t.Errorf("unexpected collecting result:\n%s", err)
	}

	// assert histogram metrics for only count
	k8smetricstest.AssertHistogramTotalCount(t, "grpc_server_handling_seconds", map[string]string{"grpc_method": "Publish", "grpc_service": "io.cloudevents.v1.CloudEventService", "grpc_type": "unary"}, 1)
	k8smetricstest.AssertHistogramTotalCount(t, "grpc_server_ce_processing_duration_seconds", map[string]string{"data_type": "io.open-cluster-management.works.v1alpha1.manifestbundles", "method": "Publish", "grpc_code": "OK"}, 1)
}

var expectedMetrics = `# HELP grpc_server_active_connections [ALPHA] Current number of active gRPC server connections.
# TYPE grpc_server_active_connections gauge
grpc_server_active_connections{local_addr="bufconn",remote_addr="bufconn"} 1
# HELP grpc_server_ce_called_total [ALPHA] Total number of RPC requests for cloudevents called on the grpc server.
# TYPE grpc_server_ce_called_total counter
grpc_server_ce_called_total{cluster="cluster1",data_type="io.open-cluster-management.works.v1alpha1.manifestbundles",method="Publish"} 1
grpc_server_ce_called_total{cluster="cluster1",data_type="io.open-cluster-management.works.v1alpha1.manifestbundles",method="Subscribe"} 1
# HELP grpc_server_ce_msg_received_total [ALPHA] Total number of messages for cloudevents received on the gRPC server.
# TYPE grpc_server_ce_msg_received_total counter
grpc_server_ce_msg_received_total{cluster="cluster1",data_type="io.open-cluster-management.works.v1alpha1.manifestbundles",method="Publish"} 1
grpc_server_ce_msg_received_total{cluster="cluster1",data_type="io.open-cluster-management.works.v1alpha1.manifestbundles",method="Subscribe"} 1
# HELP grpc_server_ce_msg_sent_total [ALPHA] Total number of messages for cloudevents sent by the gRPC server.
# TYPE grpc_server_ce_msg_sent_total counter
grpc_server_ce_msg_sent_total{cluster="cluster1",data_type="io.open-cluster-management.works.v1alpha1.manifestbundles",method="Publish"} 1
# HELP grpc_server_ce_processed_total [ALPHA] Total number of RPC requests for cloudevents processed on the server, regardless of success or failure.
# TYPE grpc_server_ce_processed_total counter
grpc_server_ce_processed_total{cluster="cluster1",data_type="io.open-cluster-management.works.v1alpha1.manifestbundles",grpc_code="OK",method="Publish"} 1
# HELP grpc_server_msg_received_bytes_total [ALPHA] Total number of bytes received on the gRPC server.
# TYPE grpc_server_msg_received_bytes_total counter
grpc_server_msg_received_bytes_total{grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 2286
grpc_server_msg_received_bytes_total{grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 69
# HELP grpc_server_msg_received_total Total number of RPC stream messages received on the server.
# TYPE grpc_server_msg_received_total counter
grpc_server_msg_received_total{grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 1
grpc_server_msg_received_total{grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 1
# HELP grpc_server_msg_sent_bytes_total [ALPHA] Total number of bytes sent by the gRPC server.
# TYPE grpc_server_msg_sent_bytes_total counter
grpc_server_msg_sent_bytes_total{grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
# HELP grpc_server_msg_sent_total Total number of gRPC stream messages sent by the server.
# TYPE grpc_server_msg_sent_total counter
grpc_server_msg_sent_total{grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 1
grpc_server_msg_sent_total{grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
# HELP grpc_server_started_total Total number of RPCs started on the server.
# TYPE grpc_server_started_total counter
grpc_server_started_total{grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 1
grpc_server_started_total{grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 1
# HELP grpc_server_handled_total Total number of RPCs completed on the server, regardless of success or failure.
# TYPE grpc_server_handled_total counter
grpc_server_handled_total{grpc_code="Aborted",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="Aborted",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="AlreadyExists",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="AlreadyExists",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="Canceled",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="Canceled",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="DataLoss",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="DataLoss",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="DeadlineExceeded",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="DeadlineExceeded",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="FailedPrecondition",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="FailedPrecondition",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="Internal",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="Internal",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="InvalidArgument",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="InvalidArgument",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="NotFound",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="NotFound",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="OK",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 1
grpc_server_handled_total{grpc_code="OK",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="OutOfRange",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="OutOfRange",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="PermissionDenied",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="PermissionDenied",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="ResourceExhausted",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="ResourceExhausted",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="Unauthenticated",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="Unauthenticated",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="Unavailable",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="Unavailable",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="Unimplemented",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="Unimplemented",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
grpc_server_handled_total{grpc_code="Unknown",grpc_method="Publish",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="unary"} 0
grpc_server_handled_total{grpc_code="Unknown",grpc_method="Subscribe",grpc_service="io.cloudevents.v1.CloudEventService",grpc_type="server_stream"} 0
`

var _ server.Service = &mockWorkService{}

type mockWorkService struct{}

func newMockWorkService() *mockWorkService {
	return &mockWorkService{}
}

func (s *mockWorkService) Get(ctx context.Context, resourceID string) (*cloudevents.Event, error) {
	return nil, nil
}

func (s *mockWorkService) List(listOpts cetypes.ListOptions) ([]*cloudevents.Event, error) {
	return nil, nil
}

func (s *mockWorkService) HandleStatusUpdate(ctx context.Context, evt *cloudevents.Event) error {
	return nil
}

func (s *mockWorkService) RegisterHandler(handler server.EventHandler) {}

var testCloudEventJSON = `{
    "specversion": "1.0",
    "id": "0192bd68-8444-4743-b02b-4a6605ec0413",
    "type": "io.open-cluster-management.works.v1alpha1.manifestbundles.spec.create_request",
    "source": "test",
    "clustername": "cluster1",
    "resourceid": "68ebf474-6709-48bb-b760-386181268064",
    "resourceversion": 1,
    "datacontenttype": "application/json",
    "data": {
        "manifests": [
            {
                "apiVersion": "apps/v1",
                "kind": "Deployment",
                "metadata": {
                    "name": "nginx",
                    "namespace": "default"
                },
                "spec": {
                    "replicas": 1,
                    "selector": {
                        "matchLabels": {
                            "app": "nginx"
                        }
                    },
                    "template": {
                        "metadata": {
                            "labels": {
                                "app": "nginx"
                            }
                        },
                        "spec": {
                            "containers": [
                                {
                                    "image": "nginxinc/nginx-unprivileged",
                                    "imagePullPolicy": "IfNotPresent",
                                    "name": "nginx"
                                }
                            ]
                        }
                    }
                }
            }
        ],
        "deleteOption": {
            "propagationPolicy": "Foreground"
        },
        "manifestConfigs": [
            {
                "resourceIdentifier": {
                    "group": "apps",
                    "resource": "deployments",
                    "namespace": "default",
                    "name": "nginx"
                },
                "feedbackRules": [
                    {
                        "type": "JSONPaths",
                        "jsonPaths": [
                            {
                                "name": "status",
                                "path": ".status"
                            }
                        ]
                    }
                ],
                "updateStrategy": {
                    "type": "ServerSideApply"
                }
            }
        ]
    }
}`
