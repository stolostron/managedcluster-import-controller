package services

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/server"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/source"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
)

type ResourceStatusHandler func(res *store.Resource) error

type ResourceService struct {
	handler       server.EventHandler
	statusHandler ResourceStatusHandler
	serverStore   *store.MemoryStore
	codec         *source.ResourceCodec
}

func NewResourceService(statusHandler ResourceStatusHandler, serverStore *store.MemoryStore) *ResourceService {
	return &ResourceService{
		statusHandler: statusHandler,
		serverStore:   serverStore,
		codec:         &source.ResourceCodec{},
	}
}

func (s *ResourceService) Get(_ context.Context, resourceID string) (*cloudevents.Event, error) {
	resource, err := s.serverStore.Get(resourceID)
	if err != nil {
		return nil, err
	}

	return s.codec.Encode(resource.Source, types.CloudEventsType{
		CloudEventsDataType: payload.ManifestBundleEventDataType,
	}, resource)
}

func (s *ResourceService) List(listOpts types.ListOptions) ([]*cloudevents.Event, error) {
	resources := s.serverStore.List(listOpts.ClusterName)
	events := make([]*cloudevents.Event, 0, len(resources))
	for _, resource := range resources {
		evt, err := s.codec.Encode(resource.Source, types.CloudEventsType{}, resource)
		if err != nil {
			return nil, err
		}
		events = append(events, evt)
	}

	return events, nil
}

func (s *ResourceService) HandleStatusUpdate(ctx context.Context, evt *cloudevents.Event) error {
	resource, err := s.codec.Decode(evt)
	if err != nil {
		return err
	}

	return s.statusHandler(resource)
}

func (s *ResourceService) RegisterHandler(handler server.EventHandler) {
	s.handler = handler
}

func (s *ResourceService) UpdateResourceSpec(resource *store.Resource) error {
	if !resource.DeletionTimestamp.IsZero() {
		return s.handler.OnDelete(context.Background(), payload.ManifestBundleEventDataType, resource.ResourceID)
	}

	return s.handler.OnUpdate(context.Background(), payload.ManifestBundleEventDataType, resource.ResourceID)
}
