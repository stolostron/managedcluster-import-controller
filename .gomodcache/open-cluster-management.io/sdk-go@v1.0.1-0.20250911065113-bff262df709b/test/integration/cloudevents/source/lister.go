package source

import (
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"

	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
)

type ResourceLister struct {
	Store *store.MemoryStore
}

var _ generic.Lister[*store.Resource] = &ResourceLister{}

func NewResourceLister(store *store.MemoryStore) *ResourceLister {
	return &ResourceLister{
		Store: store,
	}
}

func (l *ResourceLister) List(listOpts types.ListOptions) ([]*store.Resource, error) {
	return l.Store.List(listOpts.ClusterName), nil
}
