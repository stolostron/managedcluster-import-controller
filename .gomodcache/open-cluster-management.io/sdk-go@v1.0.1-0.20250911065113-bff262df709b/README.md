# Open Cluster Management SDK for Go

A Go SDK providing libraries and utilities for building applications that integrate with Open Cluster Management (OCM). This SDK enables developers to build controllers, agents, and other components that work with OCM's multi-cluster management capabilities.

## Overview

The OCM SDK for Go provides essential building blocks for:

- Building controllers and agents for multi-cluster management
- Implementing event-based communication using CloudEvents
- Managing ManifestWork resources across clusters
- Certificate management and rotation
- Resource patching and manipulation
- Testing OCM-related components

## Main Components

### CloudEvents Clients

The SDK includes comprehensive CloudEvents-based clients for implementing event-driven communication between hub clusters and managed clusters. This supports the Event Based ManifestWork architecture.

**Supported Protocols:**
- MQTT Protocol/Driver
- gRPC Protocol/Driver  
- Kafka Protocol/Driver

**Key Features:**
- Generic CloudEvents clients for custom resources
- Specialized ManifestWork clients
- Source and agent client implementations
- Automatic reconnection handling
- Resource synchronization capabilities

### Base Controller Utilities

Foundation components for building Kubernetes controllers that integrate with OCM:

- Controller factories and base implementations
- Event handling utilities
- Common controller patterns

### API Definitions

Go types and clients for OCM APIs:

- Cluster management APIs
- WorkV1 APIs for ManifestWork resources
- Typed clients for OCM resources

### Helper Utilities

Common utilities for OCM development:

- Resource application helpers
- Client utilities
- Certificate Signing Request (CSR) utilities

### Additional Components

- **Certificate Rotation**: Automated certificate lifecycle management
- **Resource Patchers**: Utilities for modifying Kubernetes resources
- **Testing Utilities**: Helper functions and mocks for testing OCM components
- **Serving Certificates**: Certificate management for webhooks and APIs

## Getting Started

### Installation

```bash
go get open-cluster-management.io/sdk-go
```

### Basic Usage

#### CloudEvents Client Example

```go
import (
    "open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
    "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/mqtt"
)

// Create a CloudEvents source client
client, err := generic.NewCloudEventSourceClient[*YourResource](
    ctx,
    mqtt.NewSourceOptions(mqttConfig, "client-id", "source-id"),
    resourceLister,
    statusHashGetter,
    resourceCodec,
)

// Subscribe to receive events
client.Subscribe(ctx, resourceHandler)
```

#### ManifestWork Client Example

```go
import (
    "open-cluster-management.io/sdk-go/pkg/cloudevents/work"
    "open-cluster-management.io/sdk-go/pkg/cloudevents/work/codec"
)

// Build a ManifestWork client
clientHolder, err := work.NewClientHolderBuilder(config).
    WithClientID("controller-client").
    WithSourceID("controller").
    WithCodec(codec.NewManifestBundleCodec()).
    NewSourceClientHolder(ctx)

manifestWorkClient := clientHolder.ManifestWorks(namespace)
```

## Documentation

- **CloudEvents**: See [pkg/cloudevents/README.md](pkg/cloudevents/README.md) for detailed CloudEvents client documentation
- **API Reference**: Go package documentation available via `go doc`
- **Examples**: Check the test directories for usage examples

## Development

### Prerequisites

- Go 1.24.0 or later
- Access to a Kubernetes cluster (for testing)

### Building

```bash
make build
```

### Testing

```bash
make test
```

## Contributing

We welcome contributions! Please see:

- [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards
- [DCO](DCO) for sign-off requirements

### Development Certificate of Origin

All commits must be signed off to indicate agreement with the Developer Certificate of Origin:

```bash
git commit --signoff
```

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Community

Open Cluster Management is a CNCF project. For more information:

- [OCM Website](https://open-cluster-management.io/)
- [OCM GitHub Organization](https://github.com/open-cluster-management-io)
- [Community Meetings and Resources](https://github.com/open-cluster-management-io/community)

## Related Projects

- [OCM Core](https://github.com/open-cluster-management-io/ocm) - Core OCM components
- [OCM API](https://github.com/open-cluster-management-io/api) - OCM API definitions
- [Addon Framework](https://github.com/open-cluster-management-io/addon-framework) - Framework for building OCM addons
