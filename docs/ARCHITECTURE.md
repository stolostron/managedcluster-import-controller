# Architecture

## System Boundary

`managedcluster-import-controller` runs in the hub cluster and reconciles Open Cluster Management resources associated with importing and detaching managed clusters. It owns hub-side bootstrap, registration, status, cleanup, and propagation resources. It does not run the Klusterlet on managed clusters; it creates or updates the resources and `ManifestWork` payloads that cause the managed-cluster-side components to be installed or changed.

## Process Startup

`cmd/manager/main.go` obtains Kubernetes, OpenShift route, API extension, OCM, work, operator, and KlusterletConfig clients. It creates a controller-runtime manager with leader election and metrics, then creates filtered shared informers for import secrets, auto-import secrets, Klusterlet `ManifestWork`, hosted-cluster `ManifestWork`, `ManagedCluster`, and `KlusterletConfig` resources. Indexers map managed clusters to related KlusterletConfig, bootstrap kubeconfig Secret, and customized CA ConfigMap changes.

The manager registers controllers through `pkg/controller/controller.go`, starts the legacy informers, waits for cache synchronization, starts FlightCtl reconciliation, and then starts the controller-runtime manager. The optional agent-registration server and pprof server are started as separate goroutines. TLS profile watching is configured before the manager starts; it is non-fatal on vanilla Kubernetes but fatal on OpenShift when setup fails.

## Reconciliation Flows

- `managedcluster` reacts to `ManagedCluster` lifecycle and metadata changes and coordinates import/detach-related state.
- `importconfig` watches `ManagedCluster`, `KlusterletConfig`, bootstrap Secrets, CA ConfigMaps, and generated RBAC resources. It renders and maintains the bootstrap and Klusterlet configuration required for a managed cluster.
- `manifestwork` and `hosted` observe OCM `ManifestWork` resources and reconcile the Klusterlet and hosted-cluster payloads delivered to managed clusters.
- `autoimport` handles auto-import Secrets; `clusterdeployment` handles Hive `ClusterDeployment`-based imports; `selfmanagedcluster` handles self-managed import behavior.
- `importstatus` updates import status, while `resourcecleanup` and `clusternamespacedeletion` remove hub-side resources during detach or deletion.
- `csr` handles managed-cluster certificate signing requests and auto-approval conditions. FlightCtl devices receive additional identification and reconciliation through `pkg/controller/flightctl`.

Controllers use predicates and custom informer sources to limit reconciles to relevant labels, annotations, lifecycle transitions, Secret data changes, and `ManifestWork` content or status changes. `pkg/helpers` centralizes client access, event recording, bootstrap rendering, import configuration, TLS behavior, image registry access, and common resource operations.

## Deployment and Testing

`deploy/` contains the Kustomize base and feature-specific overlays/resources, including RBAC and CRDs. The `build/` scripts assemble images and provision kind/OCM environments for e2e tests. Unit and controller tests use Go testing, Ginkgo/Gomega where appropriate, fake clients, and controller-runtime envtest. End-to-end tests in `test/e2e` exercise import modes against provisioned clusters and bundled CRD fixtures.
