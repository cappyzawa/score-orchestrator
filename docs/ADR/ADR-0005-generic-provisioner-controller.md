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

### Phase 1: Core Provisioner Controller
- [ ] Implement unified provisioner controller
- [ ] Support Helm strategy
- [ ] Support manifests strategy
- [ ] Template engine for variable substitution
- [ ] Basic output generation (URI, secretRef)

### Phase 2: Enhanced Features
- [ ] External API strategy
- [ ] Advanced templating functions
- [ ] Conditional logic in templates
- [ ] Multi-resource provisioning

### Phase 3: Advanced Capabilities
- [ ] Built-in common provisioning patterns (PostgreSQL, Redis, etc.)
- [ ] Custom validation for provisioning configs
- [ ] Performance optimizations and caching

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