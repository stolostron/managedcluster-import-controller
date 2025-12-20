package agent

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"

	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	workv1informers "open-cluster-management.io/api/client/work/informers/externalversions/work/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/options"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work"
	workstore "open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
)

func StartWorkAgent(ctx context.Context,
	clusterName string,
	config any,
	codec generic.Codec[*workv1.ManifestWork],
) (*work.ClientHolder, workv1informers.ManifestWorkInformer, error) {
	clientID := clusterName + "-" + rand.String(5)
	watcherStore := workstore.NewAgentInformerWatcherStore()
	opt := options.NewGenericClientOptions(config, codec, clientID).
		WithClientWatcherStore(watcherStore).
		WithClusterName(clusterName)
	clientHolder, err := work.NewAgentClientHolder(ctx, opt)
	if err != nil {
		return nil, nil, err
	}

	factory := workinformers.NewSharedInformerFactoryWithOptions(
		clientHolder.WorkInterface(),
		5*time.Minute,
		workinformers.WithNamespace(clusterName),
	)
	informer := factory.Work().V1().ManifestWorks()
	watcherStore.SetInformer(informer.Informer())

	go informer.Informer().Run(ctx.Done())

	return clientHolder, informer, nil
}
