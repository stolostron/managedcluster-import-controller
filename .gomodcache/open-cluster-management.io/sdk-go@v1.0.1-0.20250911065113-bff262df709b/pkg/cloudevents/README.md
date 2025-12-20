# Cloudevents Clients

We have implemented the [cloudevents](https://cloudevents.io/)-based clients in this package to assist developers in
easily implementing the [Event Based Manifestwork](https://github.com/open-cluster-management-io/enhancements/tree/main/enhancements/sig-architecture/224-event-based-manifestwork)
proposal.

## Generic Clients

The generic client (`generic.CloudEventsClient`) is used to resync/publish/subscribe resource objects between sources
and agents with cloudevents.

A resource object can be any object that implements the `generic.ResourceObject` interface.

### Building a generic client on a source

Developers can use `generic.NewCloudEventSourceClient` method to build a generic client on the source. To build this
client the developers need to provide

1. A cloudevents source options (`options.CloudEventsSourceOptions`), this options have two parts
    -  `sourceID`, it is a unique identifier for a source, for example, it can generate a source ID by hashing the hub
    cluster URL and appending the controller name. Similarly, a RESTful service can select a unique name or generate a
    unique ID in the associated database for its source identification.
    - `CloudEventsOptions`, it provides cloudevents clients to send/receive cloudevents based on different event
    protocol or driver implementations. Check the [Supported Protocols and Drivers](#supported-protocols-and-drivers) for more details.

2. A resource lister (`generic.Lister`), it is used to list the resource objects on the source when resyncing the
resources between sources and agents, for example, a hub controller can list the resources from the resource informers,
and a RESTful service can list its resources from a database.

3. A resource status hash getter method (`generic.StatusHashGetter`), this method will be used to calculate the resource
status hash when resyncing the resource status between sources and agents.

4. Codec (`generic.Codec`) is used to encode a resource object into a cloudevent and decode a cloudevent into a resource object based on the given cloudevent data type. We provide a codec with the cloudevent data type `io.open-cluster-management.works.v1alpha1.manifestbundles`, which contains a list of resource object(s) in the CloudEvent payload for `ManifestWork`. This codec is available in the `work/payload` package. For event schemas and examples, refer to [this doc](https://github.com/open-cluster-management-io/enhancements/tree/main/enhancements/sig-architecture/224-event-based-manifestwork).

5. Resource handler methods (`generic.ResourceHandler`), they are used to handle the resources status after the client
received the resources status from agents.

for example, build a generic client on the source using MQTT protocol with the following code:

```golang
// build a client for the source1
client, err := generic.NewCloudEventSourceClient[*CustomerResource](
        ctx,
        mqtt.NewSourceOptions(mqtt.BuildMQTTOptionsFromFlags("path/to/mqtt-config.yaml"), "client1", "source1"),
        customerResourceLister,
		customerResourceStatusHashGetter,
		customerResourceCodec,
	)

// subscribe to a broker to receive the resources status from agents
client.Subscribe(ctx, customerResourceHandler)

// start a go routine to receive client reconnect signal
go func() {
    for {
        select {
        case <-cloudEventsClient.ReconnectedChan():
            // handle the cloudevents reconnect
        }
    }
}()
```

### Building a generic client on a manged cluster

Developers can use `generic.NewCloudEventAgentClient` method to build a generic client on a managed cluster. To build
this client the developers need to provide

1. A cloudevents agent options (`options.CloudEventsAgentOptions`), this options have three parts
    -  `agentID`, it is a unique identifier for an agent, for example, it can consist of a managed cluster name and an
    agent name.
    - `clusterName`, it is the name of a managed cluster on which the agent runs.
    - `CloudEventsOptions`, it provides cloudevents clients to send/receive cloudevents based on different event
    protocol or driver implementations. Check the [Supported Protocols and Drivers](#supported-protocols-and-drivers) for more details.

2. A resource lister (`generic.Lister`), it is used to list the resource objects on a managed cluster when resyncing the
resources between sources and agents, for example, a work agent can list its works from its work informers.

3. A resource status hash getter method (`generic.StatusHashGetter`), this method will be used to calculate the resource
status hash when resyncing the resource status between sources and agents.

4. Codec (`generic.Codec`) is used to encode a resource object into a cloudevent and decode a cloudevent into a resource object based on the given cloudevent data type. We provide a codec with the cloudevent data type `io.open-cluster-management.works.v1alpha1.manifestbundles`, which contains a list of resource object(s) in the CloudEvent payload for `ManifestWork`. This codec is available in the `work/payload` package. For event schemas and examples, refer to [this doc](https://github.com/open-cluster-management-io/enhancements/tree/main/enhancements/sig-architecture/224-event-based-manifestwork).

5. Resource handler methods (`generic.ResourceHandler`), they are used to handle the resources after the client received
the resources from sources.

for example, build a generic client on the source using MQTT protocol with the following code:

```golang
// build a client for a work agent on the cluster1
client, err := generic.NewCloudEventAgentClient[*CustomerResource](
        ctx,
        mqtt.NewAgentOptions(mqtt.BuildMQTTOptionsFromFlags("path/to/mqtt-config.yaml"), "cluster1", "cluster1-work-agent"),
        &ManifestWorkLister{},
		ManifestWorkStatusHash,
		&ManifestBundleCodec{},
	)

// subscribe to a broker to receive the resources from sources
client.Subscribe(ctx, NewManifestWorkAgentHandler())

// start a go routine to receive client reconnect signal
go func() {
    for {
        select {
        case <-cloudEventsClient.ReconnectedChan():
            // handle the cloudevents reconnect
        }
    }
}()
```

## Supported Protocols and Drivers

Currently, the CloudEvents options supports the following protocols/drivers:

- [MQTT Protocol/Driver](./generic/options/mqtt)
- [gRPC Protocol/Driver](./generic/options/grpc)
- [Kafka Protocol/Driver](./generic/options/kafka)

To create CloudEvents source/agent options for these supported protocols/drivers, developers need to provide configuration specific to the protocol/driver. The configuration format resembles the kubeconfig for the Kubernetes client-go but has a different schema.

### MQTT Protocol/Driver

Below is an example of a YAML configuration for the MQTT protocol:

```yaml
broker: broker.example.com:1883
username: maestro
password: password
topics:
  sourceEvents: sources/maestro/consumers/+/sourceevents
  agentEvents: $share/statussubscribers/sources/maestro/consumers/+/agentevents
```

For detailed configuration options for the MQTT driver, refer to the [MQTT driver options package](https://github.com/open-cluster-management-io/sdk-go/blob/00a94671ced1c17d2ca2b5fad2f4baab282a7d3c/pkg/cloudevents/generic/options/mqtt/options.go#L46-L76).

### gRPC Protocol/Driver

Here's an example of a YAML configuration for the gRPC protocol:

```yaml
url: grpc.example.com:8443
caFile: /certs/ca.crt
clientCertFile: /certs/client.crt
clientKeyFile: /certs/client.key
```

For detailed configuration options for the gRPC driver, refer to the [gRPC driver options package](https://github.com/open-cluster-management-io/sdk-go/blob/00a94671ced1c17d2ca2b5fad2f4baab282a7d3c/pkg/cloudevents/generic/options/grpc/options.go#L30-L40).

### Kafka Protocol/Driver

Kafka Protocol/Drive is not enabled by default. To enable it, add the `-tags=kafka` flag before the build.

Here’s a sample configuration for Kafka in YAML:

```yaml
bootstrapServer: kafka.example.com:9092
caFile: /certs/ca.crt
clientCertFile: /certs/client.crt
clientKeyFile: /certs/client.key
```

For detailed configuration options for the Kafka driver, refer to the [Kafka driver options package](./generic/options/kafka/options.go). You can also add advanced configurations supported by librdkafka. For instance, to set the maximum Kafka protocol request message size to 200,000 bytes, use the following configuration in YAML:

```yaml
bootstrapServer: kafka.example.com:9092
caFile: /certs/ca.crt
clientCertFile: /certs/client.crt
clientKeyFile: /certs/client.key
message.copy.max.bytes: 200000
```

For the complete list of supported configurations, refer to the [librdkafka documentation](https://github.com/confluentinc/librdkafka/blob/master/CONFIGURATION.md).

## Work Clients

### Building a ManifestWorkSourceClient on the hub cluster with SourceLocalWatcherStore

```golang
sourceID := "example-controller"

// Building the clients based on cloudevents with MQTT
config := mqtt.BuildMQTTOptionsFromFlags("path/to/mqtt-config.yaml")
// Define a function to list works for initializing the store
listLocalWorksFunc :=func(ctx context.Context) (works []*workv1.ManifestWork, err error) {
    // list the works ...
    return works, err
}

// New a SourceLocalWatcherStore
watcherStore, err := workstore.NewSourceLocalWatcherStore(ctx, listLocalWorksFunc)
if err != nil {
	return err
}

clientHolder, err := work.NewClientHolderBuilder(config).
    WithClientID(fmt.Sprintf("%s-client", sourceID)).
    WithSourceID(sourceID).
    WithCodec(codec.NewManifestBundleCodec()).
    WithWorkClientWatcherStore(watcherStore).
    NewSourceClientHolder(ctx)
if err != nil {
	return err
}

manifestWorkClient := clientHolder.ManifestWorks(metav1.NamespaceAll)

// Use the manifestWorkClient to create/update/delete/watch manifestworks...
```

### Building a ManifestWorkSourceClient on the hub cluster with SourceInformerWatcherStore

```golang
sourceID := "example-controller"

// Building the clients based on cloudevents with MQTT
config := mqtt.BuildMQTTOptionsFromFlags("path/to/mqtt-config.yaml")
// New a SourceInformerWatcherStore
watcherStore := workstore.NewSourceInformerWatcherStore(ctx)

clientHolder, err := work.NewClientHolderBuilder(config).
		WithClientID(fmt.Sprintf("%s-%s", sourceID, rand.String(5))).
		WithSourceID(sourceID).
		WithCodec(codec.NewManifestBundleCodec()).
		WithWorkClientWatcherStore(watcherStore).
		NewSourceClientHolder(ctx)
	if err != nil {
		return nil, nil, err
	}

factory := workinformers.NewSharedInformerFactoryWithOptions(clientHolder.WorkInterface(), 5*time.Minute)
informer := factory.Work().V1().ManifestWorks()

// Use the informer's store as the SourceInformerWatcherStore's store
watcherStore.SetStore(informer.Informer().GetStore())

// Building controllers with ManifestWork informer ...

go informer.Informer().Run(ctx.Done())

// Use the manifestWorkClient to create/update/delete manifestworks
```

### Building a ManifestWorkAgentClient on the managed cluster with AgentInformerWatcherStore

```golang
clusterName := "cluster1"

// Building the clients based on cloudevents with MQTT
config := mqtt.BuildMQTTOptionsFromFlags("path/to/mqtt-config.yaml")
// New a AgentInformerWatcherStore
watcherStore := store.NewAgentInformerWatcherStore()

clientHolder, err := work.NewClientHolderBuilder(config).
    WithClientID(fmt.Sprintf("%s-work-agent", clusterName)).
    WithClusterName(clusterName).
    WithCodec(codec.NewManifestBundleCodec()).
    WithWorkClientWatcherStore(watcherStore).
    NewAgentClientHolder(ctx)
if err != nil {
	return err
}

manifestWorkClient := clientHolder.ManifestWorks(clusterName)

factory := workinformers.NewSharedInformerFactoryWithOptions(
		clientHolder.WorkInterface(),
		5*time.Minute,
		workinformers.WithNamespace(clusterName),
)
informer := factory.Work().V1().ManifestWorks()

// Use the informer's store as the AgentInformerWatcherStore's store
watcherStore.SetStore(informer.Informer().GetStore())

// Building controllers with ManifestWork informer ...

// Start the ManifestWork informer
go manifestWorkInformer.Informer().Run(ctx.Done())

// Use the manifestWorkClient to update work status
```

### Building garbage collector for work controllers on the hub cluster

The garbage collector is an optional component running alongside the `ManifestWork` source client. It is used to clean up `ManifestWork` resources owned by other resources. For example, in the addon-framework, `ManifestWork` resources, created by addon controllers, have owner references pointing to `ManagedClusterAddon` resources, and when the owner resources are deleted, the `ManifestWork` resources should be deleted as well.

Developers need to provide the following to build and run the garbage collector:
1. Client Holder (`work.ClientHolder`): This contains the `ManifestWork` client and informer, which can be built using the builder mentioned in last two sections.
2. Metadata Client (`metadata.Interface`): This is used to retrieve the owner resources of the `ManifestWork` resources.
3. Owner resource filters map (`map[schema.GroupVersionResource]*metav1.ListOptions`): This is used to filter the owner resources of the `ManifestWork` resources.

```golang
listOptions := &metav1.ListOptions{
    FieldSelector: fmt.Sprintf("metadata.name=%s", addonName),
}
ownerGVRFilters := map[schema.GroupVersionResource]*metav1.ListOptions{
    // ManagedClusterAddon is the owner of the ManifestWork resources filtered by the addon name
    addonv1alpha1.SchemeGroupVersion.WithResource("managedclusteraddons"): listOptions,
}
// Initialize the garbage collector
garbageCollector := garbagecollector.NewGarbageCollector(clientHolder, workInformer, metadataClient, ownerGVRFilters)
// Run the garbage collector with 1 worker to handle the garbage collection
go garbageCollector.Run(ctx, 1)
```
