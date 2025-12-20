package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	cloudeventstypes "github.com/cloudevents/sdk-go/v2/types"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	pbv1 "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc/protobuf/v1"
	grpcprotocol "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc/protocol"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/broker/services"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
)

type resourceHandler func(res *store.Resource) error

type GRPCServer struct {
	pbv1.UnimplementedCloudEventServiceServer
	serverStore     *store.MemoryStore
	resourceService *services.ResourceService
	handlers        map[string]map[string]resourceHandler
}

// func NewGRPCServer(eventBroadcaster *store.EventBroadcaster) *GRPCServer {
func NewGRPCServer(serverStore *store.MemoryStore) *GRPCServer {
	return &GRPCServer{
		serverStore: serverStore,
		handlers:    make(map[string]map[string]resourceHandler), // source -> dataType -> handler
	}
}

func (svr *GRPCServer) Publish(ctx context.Context, pubReq *pbv1.PublishRequest) (*emptypb.Empty, error) {
	// WARNING: don't use "evt, err := pb.FromProto(pubReq.Event)" to convert protobuf to cloudevent
	evt, err := binding.ToEvent(ctx, grpcprotocol.NewMessage(pubReq.Event))
	if err != nil {
		return nil, fmt.Errorf("failed to convert protobuf to cloudevent: %v", err)
	}

	res, err := decode(evt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cloudevent: %v", err)
	}

	svr.serverStore.UpSert(res)
	if err := svr.resourceService.UpdateResourceSpec(res); err != nil {
		klog.Errorf("failed to update resource spec: %v", err)
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (svr *GRPCServer) Subscribe(subReq *pbv1.SubscriptionRequest, subServer pbv1.CloudEventService_SubscribeServer) error {
	if _, ok := svr.handlers[subReq.Source]; !ok {
		svr.handlers[subReq.Source] = make(map[string]resourceHandler)
	}
	svr.handlers[subReq.Source][subReq.DataType] = func(res *store.Resource) error {
		evt, err := encode(res)
		if err != nil {
			return fmt.Errorf("failed to encode resource %s to cloudevent: %v", res.ResourceID, err)
		}

		// WARNING: don't use "pbEvt, err := pb.ToProto(evt)" to convert cloudevent to protobuf
		pbEvt := &pbv1.CloudEvent{}
		if err = grpcprotocol.WritePBMessage(context.TODO(), binding.ToMessage(evt), pbEvt); err != nil {
			return fmt.Errorf("failed to convert cloudevent to protobuf: %v", err)
		}

		// send the cloudevent to the subscriber
		// TODO: error handling to address errors beyond network issues.
		if err := subServer.Send(pbEvt); err != nil {
			klog.Errorf("failed to send grpc event, %v", err)
		}

		return nil
	}

	<-subServer.Context().Done()

	return nil
}

func (svr *GRPCServer) Start(addr string, serverOpts []grpc.ServerOption) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("failed to listen: %v", err)
		return err
	}
	grpcServer := grpc.NewServer(serverOpts...)
	pbv1.RegisterCloudEventServiceServer(grpcServer, svr)
	return grpcServer.Serve(lis)
}

func (svr *GRPCServer) GetStore() *store.MemoryStore {
	return svr.serverStore
}

func (svr *GRPCServer) SetResourceService(svc *services.ResourceService) {
	svr.resourceService = svc
}

func (svr *GRPCServer) UpdateResourceStatus(resource *store.Resource) error {
	handleFn, ok := svr.handlers[resource.Source][payload.ManifestBundleEventDataType.String()]
	if !ok {
		// there is no handler registered, do nothing, for publish only case
		return nil
	}

	if err := handleFn(resource); err != nil {
		return err
	}

	return svr.serverStore.UpdateStatus(resource)
}

func encode(resource *store.Resource) (*cloudevents.Event, error) {
	source := "test-source"
	eventType := types.CloudEventsType{
		CloudEventsDataType: payload.ManifestBundleEventDataType,
		SubResource:         types.SubResourceStatus,
		Action:              "status_update",
	}

	eventBuilder := types.NewEventBuilder(source, eventType).
		WithResourceID(resource.ResourceID).
		WithResourceVersion(resource.ResourceVersion).
		WithClusterName(resource.Namespace)

	evt := eventBuilder.NewEvent()

	if err := evt.SetData(cloudevents.ApplicationJSON, &payload.ManifestBundleStatus{Conditions: resource.Status.Conditions}); err != nil {
		return nil, fmt.Errorf("failed to encode manifest status to cloud event: %v", err)
	}

	return &evt, nil
}

func decode(evt *cloudevents.Event) (*store.Resource, error) {
	eventType, err := types.ParseCloudEventsType(evt.Type())
	if err != nil {
		return nil, fmt.Errorf("failed to parse cloud event type %s, %v", evt.Type(), err)
	}

	if eventType.CloudEventsDataType != payload.ManifestBundleEventDataType {
		return nil, fmt.Errorf("unsupported cloudevents data type %s", eventType.CloudEventsDataType)
	}

	evtExtensions := evt.Context.GetExtensions()

	resourceID, err := cloudeventstypes.ToString(evtExtensions[types.ExtensionResourceID])
	if err != nil {
		return nil, fmt.Errorf("failed to get resourceid extension: %v", err)
	}

	resourceVersion, err := cloudeventstypes.ToInteger(evtExtensions[types.ExtensionResourceVersion])
	if err != nil {
		return nil, fmt.Errorf("failed to get resourceversion extension: %v", err)
	}

	clusterName, err := cloudeventstypes.ToString(evtExtensions[types.ExtensionClusterName])
	if err != nil {
		return nil, fmt.Errorf("failed to get clustername extension: %v", err)
	}

	manifestBundle := &payload.ManifestBundle{}
	if err := evt.DataAs(manifestBundle); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event data %s, %v", string(evt.Data()), err)
	}

	resource := &store.Resource{
		Source:          evt.Source(),
		ResourceID:      resourceID,
		ResourceVersion: int64(resourceVersion),
		Namespace:       clusterName,
	}

	if deletionTimestampValue, exists := evtExtensions[types.ExtensionDeletionTimestamp]; exists {
		deletionTimestamp, err := cloudeventstypes.ToTime(deletionTimestampValue)
		if err != nil {
			return nil, fmt.Errorf("failed to convert deletion timestamp %v to time.Time: %v", deletionTimestampValue, err)
		}
		resource.DeletionTimestamp = &metav1.Time{Time: deletionTimestamp}
	} else {
		var objMap map[string]interface{}
		if err := json.Unmarshal(manifestBundle.Manifests[0].RawExtension.Raw, &objMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal raw extension to object: %v", err)
		}
		resource.Spec = unstructured.Unstructured{Object: objMap}
	}

	return resource, nil
}
