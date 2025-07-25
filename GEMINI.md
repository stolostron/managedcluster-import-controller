# Gemini AI Assistant Configuration for managedcluster-import-controller

This file provides project-specific context and guidelines for the Gemini AI assistant when reviewing code in the managedcluster-import-controller repository.

## Project Overview

The `managedcluster-import-controller` is a Kubernetes controller for Open Cluster Management (OCM) that handles the import, detach, and lifecycle management of managed clusters. It supports various cluster types including:

- Hive-provisioned OpenShift clusters
- Self-managed clusters  
- Hosted clusters (Klusterlet in hosted mode)
- FlightCTL managed devices
- Cluster API clusters
- ROSA clusters

## Architecture & Core Components

### Controller Architecture
The project follows a modular controller architecture with specialized controllers:

- **ManagedCluster Controller**: Handles metadata and namespace management
- **SelfManagedCluster Controller**: Manages self-importing clusters
- **Hosted Controller**: Handles hosted mode clusters with ManifestWork
- **FlightCTL Controller**: Manages FlightCTL device clusters
- **ImportConfig Controller**: Manages import configurations
- **ManifestWork Controller**: Handles ManifestWork resources
- **CSR Controller**: Manages certificate signing requests
- **AutoImport Controller**: Handles automatic cluster imports

### Key Packages
- `pkg/controller/`: Core controller implementations
- `pkg/helpers/`: Utility functions and client holders
- `pkg/constants/`: Project constants and configurations
- `pkg/features/`: Feature gate management
- `pkg/source/`: Informer and source management

## Code Review Guidelines

### Go Best Practices
- Follow standard Go conventions and idioms
- Use proper error handling with context
- Implement proper logging with structured logging (logr)
- Use controller-runtime patterns correctly
- Ensure proper resource cleanup and finalizer handling

### Kubernetes Controller Patterns
- Controllers should be idempotent and handle reconciliation properly
- Use proper predicates for event filtering
- Implement proper status conditions and reporting
- Handle resource ownership and garbage collection correctly
- Use proper RBAC annotations

### OCM Integration Standards
- Follow OCM API conventions and patterns
- Use proper labels and annotations as defined in OCM
- Ensure compatibility with OCM hub-spoke architecture
- Handle ManifestWork resources according to OCM patterns
- Use proper cluster lifecycle management

### Security Considerations
- Never log sensitive information (tokens, certificates, passwords)
- Validate all external inputs and configurations
- Use proper RBAC and service account permissions
- Handle secrets and credentials securely
- Implement proper certificate validation

### Performance & Reliability
- Use proper rate limiting and backoff strategies
- Implement efficient resource watching and caching
- Handle large-scale cluster management scenarios
- Use proper timeouts and context cancellation
- Optimize for minimal resource usage

### Testing Standards
- Unit tests should cover controller logic thoroughly
- Integration tests should verify controller behavior
- E2E tests should validate end-to-end workflows
- Mock external dependencies appropriately
- Test error conditions and edge cases

### Documentation Requirements
- Update relevant documentation in `docs/` directory
- Maintain accurate API documentation
- Update README.md for significant changes
- Document new features and configuration options
- Include examples for new functionality

## Common Issues to Watch For

### Controller-Specific Issues
- Missing or incorrect finalizer handling
- Improper status condition updates
- Resource leaks or orphaned resources
- Incorrect event filtering predicates
- Missing RBAC permissions

### OCM Integration Issues
- Incorrect ManifestWork resource handling
- Missing or incorrect cluster labels/annotations
- Improper hub-spoke communication patterns
- Incompatible API usage

### Kubernetes Best Practices
- Missing context propagation
- Improper error handling and logging
- Resource version conflicts
- Missing owner references
- Incorrect namespace handling

## Feature Gates
The project uses feature gates for experimental features. When reviewing code:
- Ensure new features are properly gated
- Verify feature gate usage is consistent
- Check that disabled features don't break functionality

## Dependencies
- Uses controller-runtime for Kubernetes controller framework
- Integrates with OCM APIs and components
- Supports OpenShift and vanilla Kubernetes
- Uses Hive for cluster provisioning integration

## Review Focus Areas
When reviewing PRs, pay special attention to:
1. Controller reconciliation logic correctness
2. Resource lifecycle management
3. Error handling and recovery
4. Security implications
5. Performance impact
6. OCM compatibility
7. Test coverage adequacy
8. Documentation completeness
