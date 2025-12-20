package source

import (
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cloudeventstypes "github.com/cloudevents/sdk-go/v2/types"

	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
)

type ResourceCodec struct{}

var _ generic.Codec[*store.Resource] = &ResourceCodec{}

func (c *ResourceCodec) EventDataType() types.CloudEventsDataType {
	return payload.ManifestBundleEventDataType
}

func (c *ResourceCodec) Encode(source string, eventType types.CloudEventsType, resource *store.Resource) (*cloudevents.Event, error) {
	if resource.Source != "" {
		source = resource.Source
	}

	if eventType.CloudEventsDataType != payload.ManifestBundleEventDataType {
		return nil, fmt.Errorf("unsupported cloudevents data type %s", eventType.CloudEventsDataType)
	}

	eventBuilder := types.NewEventBuilder(source, eventType).
		WithResourceID(resource.ResourceID).
		WithResourceVersion(resource.ResourceVersion).
		WithClusterName(resource.Namespace)

	if !resource.GetDeletionTimestamp().IsZero() {
		evt := eventBuilder.WithDeletionTimestamp(resource.GetDeletionTimestamp().Time).NewEvent()
		return &evt, nil
	}

	evt := eventBuilder.NewEvent()
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

func (c *ResourceCodec) Decode(evt *cloudevents.Event) (*store.Resource, error) {
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

	originalSource, err := cloudeventstypes.ToString(evtExtensions[types.ExtensionOriginalSource])
	if err != nil {
		return nil, fmt.Errorf("failed to get originalsource extension: %v", err)
	}

	manifestBundleStatus := &payload.ManifestBundleStatus{}
	if err := evt.DataAs(manifestBundleStatus); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event data %s, %v", string(evt.Data()), err)
	}

	resource := &store.Resource{
		Source:          originalSource,
		ResourceID:      resourceID,
		ResourceVersion: int64(resourceVersion),
		Namespace:       clusterName,
		Status: store.ResourceStatus{
			Conditions: manifestBundleStatus.Conditions,
		},
	}

	return resource, nil
}
