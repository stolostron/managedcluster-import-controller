package source

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	workv1informers "open-cluster-management.io/api/client/work/informers/externalversions/work/v1"

	clientoptions "open-cluster-management.io/sdk-go/pkg/cloudevents/clients/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/source/codec"
	workstore "open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
)

func StartResourceSourceClient(
	ctx context.Context,
	options *options.CloudEventsSourceOptions,
	sourceID string,
	lister *ResourceLister,
) (generic.CloudEventsClient[*store.Resource], error) {
	client, err := generic.NewCloudEventSourceClient(
		ctx,
		options,
		lister,
		StatusHashGetter,
		&ResourceCodec{},
	)
	if err != nil {
		return nil, err
	}

	client.Subscribe(ctx, func(action types.ResourceAction, resource *store.Resource) error {
		return lister.Store.UpdateStatus(resource)
	})

	return client, nil
}

func StartManifestWorkSourceClient(
	ctx context.Context,
	sourceID string,
	config any,
) (*work.ClientHolder, workv1informers.ManifestWorkInformer, error) {
	clientID := fmt.Sprintf("%s-%s", sourceID, rand.String(5))
	watcherStore := workstore.NewSourceInformerWatcherStore(ctx)

	opt := clientoptions.NewGenericClientOptions(config, codec.NewManifestBundleCodec(), clientID).
		WithClientWatcherStore(watcherStore).
		WithSourceID(sourceID)
	clientHolder, err := work.NewSourceClientHolder(ctx, opt)
	if err != nil {
		return nil, nil, err
	}

	factory := workinformers.NewSharedInformerFactoryWithOptions(clientHolder.WorkInterface(), 5*time.Minute)
	informer := factory.Work().V1().ManifestWorks()
	watcherStore.SetInformer(informer.Informer())

	go informer.Informer().Run(ctx.Done())

	return clientHolder, informer, nil
}
