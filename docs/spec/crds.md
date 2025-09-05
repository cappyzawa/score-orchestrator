# Custom Resource Definitions Specification

This document defines the conceptual structure and fields for all Custom Resource Definitions (CRDs) in the Score Orchestrator system.

## Overview

The Score Orchestrator defines four CRDs in the `score.dev/v1b1` API group:

- **Public APIs** (User-facing): `Workload`, `PlatformPolicy`
- **Internal APIs** (Platform-facing): `ResourceBinding`, `WorkloadPlan`

## Public APIs

### Workload

The `Workload` is the **only** CRD that users directly interact with. It represents a Score workload specification adapted for Kubernetes-native orchestration.

#### Spec Fields

```
spec:
  source:                              # Source Score specification
    inline: <score-spec>               # Inline Score YAML content
    # OR
    configMapRef:                      # Reference to Score spec in ConfigMap
      name: string
      key: string
  
  runtimeClass: string                 # Target runtime (e.g., "kubernetes", "ecs", "nomad")
  
  resources:                           # Score resources section
    <resource-name>:
      type: string                     # Resource type (required)
      class: string                    # Resource class (optional)
      params: object                   # Type-specific parameters
  
  containers:                          # Score containers section
    <container-name>:
      image: string                    # Container image (required)
      command: []string
      args: []string
      env: map[string]string
      files: []object
      volumes: []object
  
  service:                             # Score service section (optional)
    ports: map[string]object           # Port configurations
```

### Workload.status (user-facing, minimal, abstract)

- **endpoint: string|null** — primary user output; one canonical URL if available, otherwise `null`.
- **conditions[]** — Kubernetes-style condition objects. Use **abstract reasons only**:
  - **Types:** `Ready`, `BindingsReady`, `RuntimeReady`, `InputsValid`
  - **Reasons (fixed set):**
    - `Succeeded` — all requirements satisfied and materialized
    - `SpecInvalid` — schema/CEL violations or unresolved references
    - `PolicyViolation` — violates `PlatformPolicy`
    - `BindingPending` — dependency provisioning in progress
    - `BindingFailed` — dependency provisioning failed
    - `ProjectionError` — dependency outputs missing/mismatched
    - `RuntimeSelecting` — runtime class decision pending/deferred
    - `RuntimeProvisioning` — runtime materialization in progress
    - `RuntimeDegraded` — runtime reported unhealthy/degraded state
    - `QuotaExceeded` — quota/capacity limitations
    - `PermissionDenied` — missing privileges/credentials
    - `NetworkUnavailable` — connectivity constraints/unreachable endpoints
  - **Message rule:** one neutral sentence; do **not** include runtime-specific nouns (e.g., "Deployment/Pod/ECS Task").
- **bindings[]** — per dependency summary:
  - `key`, `phase (Pending|Binding|Bound|Failed)`, `reason`, `message`, `outputsAvailable: bool`.
- **Readiness rule:** `InputsValid=True AND BindingsReady=True AND RuntimeReady=True`.

#### Reason Code Definitions

| Reason | When to Use | Implementation Guidance |
|--------|-------------|------------------------|
| `Succeeded` | All requirements satisfied and materialized | Final success state; workload fully operational |
| `SpecInvalid` | Schema/CEL violations or unresolved references | User must fix workload specification |
| `PolicyViolation` | Violates `PlatformPolicy` constraints | User or admin must address policy compliance |
| `BindingPending` | Dependency provisioning in progress | Wait for resolver controllers to complete |
| `BindingFailed` | Dependency provisioning failed | Check resolver controller logs and resource availability |
| `ProjectionError` | Dependency outputs missing/mismatched | Runtime should not use this; orchestrator-internal issue |
| `RuntimeSelecting` | Runtime class decision pending/deferred | Platform determining optimal execution strategy |
| `RuntimeProvisioning` | Runtime materialization in progress | Platform resources being created/updated |
| `RuntimeDegraded` | Runtime reported unhealthy/degraded state | Workload functional but suboptimal; investigate platform |
| `QuotaExceeded` | Quota/capacity limitations | Platform resource limits reached; retry or increase quotas |
| `PermissionDenied` | Missing privileges/credentials | RBAC or service account configuration issue |
| `NetworkUnavailable` | Connectivity constraints/unreachable endpoints | Network policies or infrastructure connectivity issue |

#### Condition Types

- **`Ready`**: Overall workload readiness (derived from other conditions)
- **`InputsValid`**: Workload specification validity
- **`BindingsReady`**: All resource dependencies are bound and ready
- **`RuntimeReady`**: Runtime platform has successfully materialized the workload

#### Abstract Reason Vocabulary

**Success States:**
- `Succeeded`: Operation completed successfully

**Input/Validation Issues:**
- `SpecInvalid`: Workload specification contains errors
- `PolicyViolation`: Workload violates platform policy

**Binding/Dependency Issues:**
- `BindingPending`: Resource binding is in progress
- `BindingFailed`: Resource binding encountered an error

**Runtime Issues:**
- `ProjectionError`: Error projecting workload to runtime
- `RuntimeSelecting`: Runtime is selecting optimal deployment strategy
- `RuntimeProvisioning`: Runtime is provisioning resources
- `RuntimeDegraded`: Runtime reports degraded but functional state

**Infrastructure Issues:**
- `QuotaExceeded`: Platform resource quotas exceeded
- `PermissionDenied`: Insufficient permissions for operation
- `NetworkUnavailable`: Network connectivity issues

#### Ready Computation Logic

`Ready=True` when:
- `InputsValid=True` AND
- `BindingsReady=True` AND  
- `RuntimeReady=True`

### PlatformPolicy

Platform policies define governance rules, defaults, and constraints applied to workloads.

- Scope: cluster-scoped. Managed by platform operators.
- Visibility: hidden from users (no list/get/watch).

#### Spec Fields

```
spec:
  selector:                           # Workload selection criteria
    matchLabels: map[string]string
    matchExpressions: []object
  
  defaults:                           # Default values to apply
    runtimeClass: string
    resources: map[string]object      # Default resource configurations
    annotations: map[string]string
    labels: map[string]string
  
  constraints:                        # Validation constraints
    allowedRuntimeClasses: []string
    resourceTypes:
      allowed: []string
      denied: []string
    images:
      allowedRegistries: []string
      requiredTags: []string
  
  priority: int32                     # Policy application priority (higher wins)
```

## Internal APIs

### ResourceBinding

Represents the contract between Orchestrator and Resolver controllers for managing resource dependencies.

- Phase model: `Pending → Binding → (Bound | Failed)`; may re-enter on reconcile.
- Outputs: standardized shapes (e.g., `secretRef`, `configMapRef`, `uri`, `cert`) consumed by the Orchestrator/Runtime.
- Visibility: internal (hidden from users).

#### Spec Fields

```
spec:
  workloadRef:                        # Reference to owning Workload
    name: string
    uid: string
  
  resourceKey: string                 # Key from Workload.spec.resources
  type: string                        # Resource type (from Workload spec)
  class: string                       # Resource class (optional)
  params: object                      # Type-specific parameters
  
  provisioner: string                 # Resolver controller identifier
```

#### Status Fields

```
status:
  phase: string                       # "Pending", "Binding", "Bound", "Failed"
  
  conditions: []object               # Standard conditions
    - type: string                   # "Ready", "Provisioned", "OutputsAvailable"
      status: string
      reason: string
      message: string
      lastTransitionTime: timestamp
  
  outputs: object                    # Standardized outputs for consumption
    # Common output patterns:
    connectionString: string         # Database/service connection info
    endpoint: string                 # Service endpoint URI
    credentials:                     # Credential references
      secretRef:
        name: string
        key: string
    certificates:                    # Certificate references
      configMapRef:
        name: string
        key: string
    metadata: map[string]string      # Additional metadata
  
  bindingTime: timestamp             # When binding was established
  provisionerInfo:                   # Provisioner-specific information
    controller: string
    version: string
    metadata: map[string]string
```

### WorkloadPlan

Represents the execution plan generated by the Orchestrator for consumption by Runtime controllers.

- Ownership: same namespace/name as the target `Workload` (OwnerReference for cascading GC).
- Single writer: Orchestrator only. Runtime is read-only.
- Conceptual fields: `workloadRef`, `observedWorkloadGeneration`, `runtimeClass`, `projection (minimal env mapping rules)`, `bindings (desired summaries)`.
- Visibility: internal (hidden from users).

#### Spec Fields

```
spec:
  workloadRef:                       # Reference to source Workload
    name: string
    uid: string
    generation: int64                # Observed Workload generation
  
  runtimeClass: string               # Target runtime platform
  
  projection:                        # Workload projection configuration
    containers: map[string]object    # Container projection rules
      # Example container projection:
      <container-name>:
        image: string
        env: []object                # Environment variable projections
          - name: string
            value: string            # Static value
            # OR
            from:                    # Dynamic value from binding
              bindingKey: string     # ResourceBinding key  
              outputKey: string      # Output field path
        volumes: []object            # Volume mount projections
        files: []object              # File mount projections
    
    service: object                  # Service projection (if defined)
      ports: map[string]object
      annotations: map[string]string
    
    networking: object               # Network configuration
      ingress: []object              # Ingress rules (if applicable)
      policies: []object             # Network policies
  
  bindings: []object                 # Required dependency summary
    - key: string                    # ResourceBinding key
      type: string                   # Resource type
      outputs: []string              # Required output fields
      critical: boolean              # Whether binding is critical for startup
  
  metadata:                          # Additional runtime metadata
    labels: map[string]string
    annotations: map[string]string
    ownershipMode: string            # "controller", "user", "shared"
```

#### Status Fields

```
status:
  phase: string                      # "Pending", "Planning", "Projected", "Failed"
  
  conditions: []object               # Standard conditions
  
  runtimeInfo:                       # Runtime-specific status (opaque to Orchestrator)
    controller: string
    state: string
    metadata: map[string]string
  
  lastProjectedTime: timestamp       # Last successful projection
```

## Field Validation Summary

### Required Fields
- `Workload.spec.containers[].image`
- `Workload.spec.service.ports[].port` (when service is defined)
- `Workload.spec.resources[].type` (when resources are defined)
- `ResourceBinding.spec.type`
- `WorkloadPlan.spec.runtimeClass`

### OneOf Constraints
- `Workload.spec.source`: Must specify either `inline` OR `configMapRef`
- `Workload.spec.containers[].files[]`: Must specify exactly one source type

### Format Validations
- `Workload.status.endpoint`: Must be valid URI format when present
- All timestamp fields: Must be valid RFC3339 format
- UID references: Must be valid Kubernetes UID format

## Ownership and References

- `ResourceBinding` resources are owned by their corresponding `Workload` (OwnerReference)
- `WorkloadPlan` resources are owned by their corresponding `Workload` (OwnerReference) 
- All cross-references use both `name` and `uid` for strong consistency
- Garbage collection follows standard Kubernetes cascading deletion patterns

## RBAC Implications

- **Users**: Read/write access to `Workload` and `PlatformPolicy` only
- **Orchestrator**: Read/write access to all CRDs, single writer of `Workload.status`
- **Runtime Controllers**: Read access to `WorkloadPlan` and `ResourceBinding.status`
- **Resolver Controllers**: Read/write access to `ResourceBinding.status` only