# ADR-0001: Score Orchestrator Architecture

## Status

**ACCEPTED** - 2025-09-06

## Context

Score specification (score.dev) provides a platform-agnostic way to define application workloads, but there's no standardized orchestration system that can:

1. **Abstract platform complexity** from users while maintaining platform flexibility
2. **Provide consistent interfaces** across different runtime environments (Kubernetes, ECS, Nomad, etc.)
3. **Separate concerns** between application specification, resource resolution, and platform execution
4. **Enable governance** without forcing all platforms to implement identical policies

The challenge is designing an architecture that maintains the Score philosophy of simplicity while enabling sophisticated platform-side orchestration and resource management.

## Decision Outcomes

### 1. API Group and Versioning
- **Group**: `score.dev/v1b1`
- **Rationale**: Align with Score project namespace, use beta versioning to indicate evolving API

### 2. CRD Design - Four-Resource Architecture

#### Public APIs (User-facing)
- **`Workload`**: The ONLY resource users directly interact with
- **`PlatformPolicy`**: Platform-applied governance and defaults

#### Internal APIs (Platform-facing)
- **`ResourceBinding`**: Contracts between Orchestrator and Resolver controllers
- **`WorkloadPlan`**: Contracts between Orchestrator and Runtime controllers

### 3. Controller Architecture - Three-Layer Separation

#### Orchestrator Controller (Community)
- **Single writer** of `Workload.status`
- Interprets `Workload` + `PlatformPolicy`
- Generates `ResourceBinding` and `WorkloadPlan`
- Aggregates status from all dependencies

#### Runtime Controller (Platform-specific)
- Consumes `WorkloadPlan` and `ResourceBinding.status.outputs`
- Materializes workloads on target platforms
- Manages internal platform objects (invisible to users)

#### Resolver Controllers (Platform/Vendor-specific)
- Bind `ResourceBinding` resources
- Provide standardized outputs

### 4. Minimal Status Interface
- **`Workload.status.endpoint`**: Single URI, primary user-facing output
- **`Workload.status.conditions`**: Abstract condition types with neutral messaging
- **`Workload.status.bindings`**: High-level dependency summaries
- **No platform-specific terminology** in user-facing status

### 5. Validation Strategy Split
- **CRD OpenAPI + CEL**: Specification-level invariants (community-provided)
- **VAP/OPA/Kyverno**: Organization-specific policies (platform-delegated)

## Considered Alternatives

### Alternative 1: Include WorkloadPlan in Workload.status
**Rejected** because:
- **Abstraction leakage**: Exposes internal orchestration details to users who should only see abstract status
- **Tight coupling**: Creates dependency between user API and runtime implementation, preventing independent evolution
- **Single-writer principle violation**: Multiple components would need to write to `Workload.status`, creating race conditions
- **RBAC complexity**: Users would see platform-internal data, requiring complex permission scoping
- **Status bloat**: WorkloadPlan contains projection rules and binding details irrelevant to user troubleshooting

### Alternative 2: Single Monolithic Controller
**Rejected** because:
- **Platform customization barrier**: Forces all platforms to implement identical dependency management, preventing optimization for specific runtimes
- **Community vs. platform bottleneck**: Creates single point of control for both community features and vendor-specific integrations
- **Scalability concerns**: Single controller would need to handle all resource types across all platforms, creating performance bottlenecks
- **Extension inflexibility**: Reduces ability to add platform-specific resolvers or runtime adaptations
- **Development coordination overhead**: All changes require coordination between community and multiple platform teams

### Alternative 3: Direct Platform Integration (No Abstraction Layer)
**Rejected** because:
- **Platform knowledge burden**: Users must understand platform-specific concepts (Kubernetes Deployments, ECS Tasks, etc.), violating Score's platform-neutrality principle
- **Inconsistent interfaces**: Different platforms would expose different APIs, breaking workload portability
- **Developer complexity**: Application developers forced to maintain platform-specific knowledge and tooling
- **Tooling fragmentation**: Monitoring, debugging, and management tools would need platform-specific implementations
- **Migration barriers**: Moving workloads between platforms requires rewriting operational scripts and processes

### Alternative 4: Rich Status with Platform Details
**Rejected** because:
- **Abstraction violation**: Exposes platform-specific terminology (Pod names, Task ARNs) to users who shouldn't need to know these details
- **Portability compromise**: Status messages containing platform-specific details make workloads less portable and tooling more complex
- **User cognitive overhead**: Forces users to understand multiple platform concepts even when they only care about application-level status
- **Tooling complexity**: Monitoring and alerting systems must handle platform-specific status variations instead of unified abstractions
- **Debugging confusion**: Platform-specific details in user-facing status can mislead users into investigating platform issues instead of application issues

## Rationale

### Single Writer Principle
Making Orchestrator the sole writer of `Workload.status` provides:
- **Consistency**: Users always see coherent, aggregated state
- **Reliability**: No race conditions between multiple status writers
- **Abstraction**: Platform details are filtered through neutral interface
- **Debuggability**: Single source of truth for user-visible state

### Internal CRDs for Platform Contracts
Separate `ResourceBinding` and `WorkloadPlan` CRDs provide:
- **Clear contracts**: Explicit interfaces between controller layers
- **Independent evolution**: Platform controllers can evolve without affecting user API
- **RBAC separation**: Users can't access internal orchestration details
- **Extensibility**: Platforms can add custom fields to internal resources

### Abstract Status Vocabulary
Using platform-neutral condition types and messages:
- **Portability**: Same workload shows consistent status across platforms
- **Maintainability**: Changes in platform internals don't break user interfaces
- **Tool compatibility**: Monitoring/management tools work across platforms
- **User experience**: Developers don't need platform-specific knowledge

## Impact and Consequences

### Positive Consequences
- **Clean separation of concerns** between user, orchestration, and platform layers
- **Platform flexibility** without user complexity
- **Consistent user experience** across different runtime environments
- **Clear extension points** for platform and vendor customization
- **Strong abstraction boundaries** that can evolve independently

### Negative Consequences
- **Additional complexity** in controller implementation
- **Learning curve** for platform implementers
- **Debugging complexity** due to multiple layers of abstraction
- **Potential over-abstraction** if status is too minimal

### Migration Path
- Start with documentation and specifications
- Implement CRD schemas with OpenAPI validation
- Build reference Orchestrator controller
- Create reference Resolver controllers
- Develop platform-specific Runtime controllers
- Establish conformance testing

## Risks and Mitigations

### Risk: Over-abstraction in status
**Mitigation**: Include `conditions` with detailed abstract reasons, allow platform-specific annotations for debugging

### Risk: Complex controller interactions
**Mitigation**: Clear contracts via CRD schemas, comprehensive integration testing, conformance test suites

### Risk: Platform adoption challenges
**Mitigation**: Reference implementations, clear migration documentation, community support

## Related Decisions

This ADR establishes the foundation for:
- ADR-0002: Community Scope - Orchestrator (expands community scope from CRDs-only to include reference Orchestrator)

Future ADRs may cover:
- CRD field specifications and validation rules
- Controller interaction protocols and contracts
- Status and condition semantics refinements
- Platform-specific integration patterns
