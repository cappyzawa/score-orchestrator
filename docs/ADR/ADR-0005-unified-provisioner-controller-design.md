# ADR-0005: Unified Provisioner Controller Design

**Status**: Accepted
**Date**: 2025-09-19
**Authors**: Score Orchestrator Team

## Context

During the initial implementation investigation, we discovered that the current provisioner architecture requires platform operators to implement separate controller binaries for each resource type (e.g., `postgres-operator`, `redis-operator`, `mongodb-operator`). This approach has several drawbacks:

1. **High Implementation Barrier**: Each provisioner requires Go controller implementation
2. **Operational Complexity**: Multiple controller deployments to manage
3. **Limited Accessibility**: Non-Go developers cannot easily extend provisioning capabilities
4. **Convention-based Routing**: ResourceClaim routing relies on naming conventions rather than explicit mechanisms

The original design assumed that provisioner controllers would be implemented as separate binaries that watch ResourceClaims by `spec.type`, similar to how runtime controllers work. However, this proved impractical for the majority of use cases.

## Decision

We will implement a **Unified Provisioner Controller** that handles provisioning through declarative YAML configuration rather than requiring custom controller implementations.

### Core Design Principles

1. **Single Controller**: One `provisioner` controller handles all configured resource types
2. **Configuration-Driven**: Provisioning logic defined in YAML/JSON configuration
3. **Template-Based**: Support multiple provisioning strategies (Helm, Manifests, External APIs)
4. **Unified Architecture**: No distinction between built-in and custom provisioning - all use same controller

### Architecture

```yaml
# Orchestrator Config
provisioners:
- type: postgres
  config:
    strategy: helm
    helm:
      chart: bitnami/postgresql
      repository: https://charts.bitnami.com/bitnami
      values:
        auth:
          postgresPassword: "{{.secret.password}}"
        primary:
          resources:
            requests:
              cpu: "{{.class.cpu}}"
              memory: "{{.class.memory}}"
    outputs:
      uri: "postgresql://postgres:{{.secret.password}}@{{.service.name}}:5432/postgres"
      secretRef: "{{.secret.name}}"

- type: redis
  config:
    strategy: manifests
    manifests:
      - apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: "{{.claimName}}-redis"
        spec:
          template:
            spec:
              containers:
              - name: redis
                image: redis:7-alpine
    outputs:
      uri: "redis://{{.service.name}}:6379"

- type: mongodb-atlas
  config:
    strategy: external-api
    externalApi:
      endpoint: "https://cloud.mongodb.com/api/atlas/v1.0"
      auth:
        type: api-key
        secretRef: mongodb-atlas-credentials
    outputs:
      uri: "{{.response.connectionString}}"
```

### Supported Strategies

1. **Helm Strategy**: Deploy Helm charts with parameterized values
2. **Manifests Strategy**: Apply Kubernetes manifests with template substitution
3. **External API Strategy**: Call external APIs for cloud resource provisioning

### Template Variables

The generic provisioner provides these template variables:

- `{{.claimName}}`: ResourceClaim name
- `{{.claimKey}}`: Resource key from Workload spec
- `{{.namespace}}`: Target namespace
- `{{.class.*}}`: Class-specific parameters
- `{{.params.*}}`: Custom parameters from ResourceClaim
- `{{.secret.*}}`: Generated secrets (passwords, keys, etc.)
- `{{.service.*}}`: Generated service information

## Implementation Plan

### Phase 1: Core Provisioner Controller Infrastructure
Focus on implementing the fundamental Provisioner Controller that can watch and manage ResourceClaim lifecycle according to the control plane specifications.

#### Phase 1.1: Controller Foundation
- [ ] Implement basic Provisioner Controller structure
  - [ ] Watch ResourceClaim resources filtered by supported types
  - [ ] Implement controller-runtime based reconciler
  - [ ] Set up proper RBAC for ResourceClaim read/write access
  - [ ] Add finalizer management for proper cleanup

#### Phase 1.2: ResourceClaim Status Management
- [ ] Implement ResourceClaim status updates according to spec
  - [ ] Support phase transitions: `Pending → Binding → (Bound | Failed)`
  - [ ] Update `reason` and `message` with abstract vocabulary
  - [ ] Implement `outputsAvailable` boolean gate
  - [ ] Track `observedGeneration` and `lastTransitionTime`

#### Phase 1.3: Basic Provisioning Strategy Framework
- [ ] Define provisioning strategy interface
  - [ ] Abstract strategy interface for different provisioning methods
  - [ ] Configuration loading from Orchestrator Config
  - [ ] Strategy selection based on ResourceClaim type
  - [ ] Error handling and retry mechanisms

#### Phase 1.4: Minimal Output Generation
- [ ] Implement standardized output format according to spec
  - [ ] Support `secretRef` outputs (reference to generated Secrets)
  - [ ] Support `configMapRef` outputs (reference to generated ConfigMaps)
  - [ ] Support `uri` outputs (connection strings, endpoints)
  - [ ] Ensure CEL validation constraint compliance (at least one output field)

#### Phase 1.5: Integration Testing
- [ ] Test ResourceClaim lifecycle management
- [ ] Verify status updates and phase transitions
- [ ] Test finalizer behavior and cleanup
- [ ] Validate output format compliance
- [ ] Integration with Orchestrator's ResourceClaim creation

### Phase 2: Concrete Provisioning Strategies
After Phase 1 establishes the controller foundation, implement specific provisioning methods.

#### Phase 2.1: Secret-based Strategy
- [ ] Implement simple Secret generation strategy
- [ ] Template-based Secret creation with variable substitution
- [ ] Support for passwords, tokens, and credentials

#### Phase 2.2: ConfigMap-based Strategy
- [ ] Implement ConfigMap generation strategy
- [ ] Support for configuration files and non-sensitive data

#### Phase 2.3: Ephemeral Strategy
- [ ] Implement in-memory/ephemeral resource strategy
- [ ] Useful for testing and development scenarios

### Phase 3: Advanced Provisioning Strategies
Implement more sophisticated provisioning methods after core functionality is stable.

#### Phase 3.1: Helm Strategy
- [ ] Helm chart deployment strategy
- [ ] Template variable substitution for Helm values
- [ ] Chart repository management

#### Phase 3.2: Manifest Strategy
- [ ] Kubernetes manifest application strategy
- [ ] Template engine for manifest customization
- [ ] Multi-resource manifest handling

#### Phase 3.3: External API Strategy
- [ ] External service API integration
- [ ] Authentication and credential management
- [ ] Response mapping to ResourceClaim outputs

## Consequences

### Positive
- **Lower Entry Barrier**: Platform operators can add provisioners via YAML
- **Simplified Operations**: Single controller to deploy and maintain
- **Faster Development**: No Go knowledge required for basic provisioning
- **Consistent Patterns**: Standardized provisioning interface across resource types

### Negative
- **Complexity Limits**: Very complex provisioning logic may require elaborate YAML configurations
- **Template Maintenance**: YAML configurations may become complex for advanced use cases
- **Learning Curve**: Platform operators need to learn templating syntax and provisioning strategies

### Mitigation
- Provide built-in common patterns for popular resource types
- Comprehensive documentation and examples for each strategy
- Template validation and debugging tools

## Alternatives Considered

1. **Keep Current Architecture**: Rejected due to high implementation barrier
2. **Webhook-based Provisioning**: Rejected due to complexity and reliability concerns
3. **External Provisioning Service**: Rejected due to operational overhead
4. **Hybrid Model (Generic + Custom Controllers)**: Rejected for simplicity - unified approach is more maintainable

## References

- Original provisioner investigation findings
- Kubernetes Controller pattern documentation
- Helm template documentation
- Similar patterns in ArgoCD ApplicationSets and Crossplane Compositions