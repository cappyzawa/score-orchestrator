# Workload Lifecycle and State Management

This document describes the complete lifecycle of Score workloads through the orchestration system, from initial user submission to runtime deployment and status reflection.

## Lifecycle Overview

The Score Orchestrator manages workloads through a well-defined lifecycle with clear phases, state transitions, and controller responsibilities. The system maintains strong consistency guarantees while providing abstracted status information to users.

```
User applies Workload 
    ↓
Orchestrator reads **Orchestrator Config** and Admission (policy)
    ↓  
Orchestrator generates ResourceClaim resources
    ↓
Provisioner Controllers bind dependencies and provide outputs
    ↓
Orchestrator generates WorkloadPlan with projection rules
    ↓
Runtime Controller materializes workload on target platform
    ↓
Runtime reports status back through internal mechanisms
    ↓
Orchestrator aggregates status and updates Workload.status.endpoint
```

## Phase 1: Workload Submission and Validation

### User Action
User submits a `Workload` resource via `kubectl apply` or equivalent API call.

### Orchestrator Response
1. **Input Validation**: CRD-level validation (OpenAPI + CEL) ensures specification compliance
2. **Policy Application**: Reads **Orchestrator Config** (ConfigMap/OCI) and applies Admission rules
3. **Status Initialization**: Sets initial conditions:
   - `InputsValid=Unknown` (validation in progress)
   - `ClaimsReady=False` (no bindings created yet)  
   - `RuntimeReady=False` (runtime not engaged yet)
   - `Ready=False` (derived from above conditions)

### Possible Outcomes
- **Success**: `InputsValid=True`, proceed to dependency resolution
- **Validation Failure**: `InputsValid=False` with `reason=SpecInvalid`
- **Policy Violation**: `InputsValid=False` with `reason=PolicyViolation`

## Phase 2: Dependency Resolution

### Orchestrator Actions
The orchestrator selects an **abstract profile** (auto; optional abstract hint) and a **backend** deterministically (priority→version→name) before plan generation.

1. **Resource Analysis**: Parse `spec.resources` to identify required dependencies
2. **ResourceClaim Generation**: Create `ResourceClaim` resources for each dependency:
   - Set appropriate `spec.type`, `spec.class`, and `spec.params`
   - Establish OwnerReference to parent Workload
   - Select appropriate provisioner based on Orchestrator configuration

### ResourceClaim Lifecycle
Each `ResourceClaim` follows its own state progression:

```
ResourceClaim Phase Transitions:
Pending → Binding → Bound (success)
                 ↘ Failed (error)
```

#### Pending Phase
- **Trigger**: ResourceClaim created by Orchestrator
- **State**: Waiting for Provisioner Controller to claim the binding
- **Orchestrator Status**: Updates `Workload.status.bindings[].phase=Pending`

#### Binding Phase  
- **Trigger**: Provisioner Controller begins provisioning the resource
- **State**: Active provisioning of the required dependency
- **Orchestrator Status**: Updates `Workload.status.bindings[].phase=Binding`

#### Bound Phase (Success)
- **Trigger**: Provisioner Controller successfully provisions resource and populates `status.outputs`
- **State**: Resource ready for consumption, outputs available
- **Orchestrator Status**: 
  - Updates `Workload.status.bindings[].phase=Bound`
  - Sets `outputsAvailable=true`
  - If all bindings are Bound, sets `ClaimsReady=True`

#### Failed Phase (Error)
- **Trigger**: Provisioner Controller encounters unrecoverable error
- **State**: Resource provisioning failed, outputs unavailable  
- **Orchestrator Status**:
  - Updates `Workload.status.bindings[].phase=Failed`
  - Sets appropriate abstract `reason` (e.g., `QuotaExceeded`, `PermissionDenied`)
  - Sets `ClaimsReady=False`

### ClaimsReady Determination

The Orchestrator sets `ClaimsReady=True` when:
- All required ResourceClaims are in `Bound` phase AND
- All critical ResourceClaims have `outputsAvailable=true` AND
- No ResourceClaims are in `Failed` phase

## Phase 3: Workload Plan Generation

### Trigger Conditions
- `InputsValid=True` (specification is valid)
- `ClaimsReady=True` (all dependencies are bound)

### Orchestrator Actions
1. **Projection Rule Generation**: Create container environment variable mappings:
   ```
   env:
     - name: DATABASE_URL
       from:
         bindingKey: "database"
         outputKey: "connectionString"
   ```

2. **WorkloadPlan Creation**: Generate comprehensive execution plan with:
   - **Values precedence**: **`defaults ⊕ normalize(Workload) ⊕ outputs`** (right-hand wins)
   - **Container Projections**: Environment variables, volume mounts, file injections
   - **Service Configuration**: Port mappings, ingress rules, network policies
   - **Binding Dependencies**: Required outputs and criticality indicators
   - **Runtime Metadata**: Labels, annotations, ownership mode

   Note: `${resources.*}` resolution occurs **after Provision completion** (unresolved placeholders result in `ProjectionError`)

3. **Plan Validation**: Ensure plan completeness and consistency

### Runtime materialization lifecycle (internal)

> These phases are tracked by **runtime-internal resources**; `WorkloadPlan` has **no** `.status`.

```
Runtime (internal) Phase Transitions:
Pending → Planning → Projected (success)
                   ↘ Failed (error)
```

- **Pending**: Plan created, waiting for Runtime Controller
- **Planning**: Runtime Controller processing the plan
- **Projected**: Successfully materialized on target platform
- **Failed**: Runtime Controller encountered errors during materialization

## Phase 4: Runtime Materialization

### Runtime Controller Actions
1. **Plan Consumption**: Read `WorkloadPlan` and referenced `ResourceClaim.status.outputs`
2. **Platform Translation**: Convert abstract plan to platform-specific resources:
   - **Kubernetes**: Generate Deployment, Service, ConfigMap, Secret resources
   - **ECS**: Create Task Definition, Service, Load Balancer configurations
   - **Nomad**: Generate Job specification with appropriate constraints

3. **Resource Provisioning**: Apply platform-specific resources
4. **Status Monitoring**: Monitor platform resources and report aggregated status

### Runtime Status Flow

Runtime Controllers manage platform-specific resources but report status through standardized mechanisms:

1. **Initial Deployment**: `RuntimeReady=Unknown`, `reason=RuntimeSelecting`
2. **Provisioning**: `RuntimeReady=Unknown`, `reason=RuntimeProvisioning`  
3. **Success**: `RuntimeReady=True`, `reason=Succeeded`
4. **Degraded**: `RuntimeReady=True`, `reason=RuntimeDegraded` (functional but suboptimal)
5. **Failure**: `RuntimeReady=False`, `reason=ProjectionError`

## Phase 5: Status Aggregation and Endpoint Reflection

### Orchestrator Status Aggregation

The Orchestrator continuously monitors all dependent resources and maintains the **single source of truth** for `Workload.status`:

#### Ready Computation Logic

```
Ready = InputsValid ∧ ClaimsReady ∧ RuntimeReady

where:
  InputsValid = (spec validation passed ∧ policy compliance verified)
  ClaimsReady = (all ResourceClaims in Bound phase ∧ outputs available)  
  RuntimeReady = (platform materialization successful ∧ workload functional)
```

#### Endpoint Reflection Timing

The `status.endpoint` field follows this update sequence:

1. **Initial State**: `endpoint: null` (no service available)
2. **Runtime Reports Endpoint**: Runtime Controller updates internal status with service endpoint
3. **Orchestrator Aggregation**: Orchestrator reads runtime status and selects primary endpoint
4. **Status Update**: Orchestrator updates `Workload.status.endpoint` with validated URI
5. **User Visibility**: Endpoint becomes available to users via `kubectl get workload`

#### Endpoint Selection Policy

The Orchestrator determines the canonical endpoint using the following priority:

1. **Orchestrator Config template**: If config specifies endpoint template pattern
2. **Service Port Names**: Prefer ports named `web`, `http`, `https`, or `main`
3. **Single Port**: If only one port is exposed, use that port
4. **Platform Defaults**: Runtime-specific selection (external ingress > load balancer > nodeport > clusterip)

**Normalization Rules:**
- Always prefer HTTPS over HTTP when both are available
- Include scheme (http/https) and port only when non-standard (not 80/443)
- Use FQDN when available, fallback to service discovery names
- Never expose internal/debug ports (health checks, metrics, admin interfaces)

Only **one endpoint** is exposed in `status.endpoint` to maintain interface simplicity.

## Error Handling and Recovery

### Transient Errors
- **Network Issues**: `reason=NetworkUnavailable`, automatic retry with exponential backoff
- **Resource Contention**: `reason=RuntimeProvisioning`, wait for platform resource availability
- **Quota Limits**: `reason=QuotaExceeded`, pause until quota available

### Permanent Errors  
- **Specification Errors**: `reason=SpecInvalid`, requires user intervention
- **Policy Violations**: `reason=PolicyViolation`, requires compliance or policy change
- **Resource Binding Failures**: `reason=BindingFailed`, requires dependency resolution

### Recovery Mechanisms

#### Automatic Recovery
- Provisioner Controllers retry failed bindings with exponential backoff
- Runtime Controllers automatically reconcile platform resource drift
- Network partitions trigger re-evaluation of dependent resource status

#### Manual Recovery
- Users can trigger reconciliation by updating Workload metadata annotations
- Platform operators can reset binding states by deleting and recreating ResourceClaims
- Emergency rollback available through Workload generation reversion

## State Consistency and Observability

### Single Writer Guarantee
Only the **Orchestrator Controller** writes to `Workload.status`, ensuring:
- No race conditions between status writers
- Consistent view of workload state across all consumers
- Atomic status updates with proper conflict resolution

### Observability Points

#### User-Facing Observability
```bash
# Primary status check
kubectl get workload myapp

# Detailed status inspection  
kubectl describe workload myapp

# Ready condition monitoring
kubectl wait --for=condition=Ready workload/myapp
```

#### Platform-Facing Observability
```bash  
# ResourceClaim status
kubectl get resourceclaim -l workload=myapp

# WorkloadPlan status
kubectl get workloadplan myapp

# Controller health
kubectl get events --field-selector involvedObject.kind=Workload
```

### Status Consistency Timing

- **Status Updates**: Eventually consistent within 30 seconds under normal conditions
- **Endpoint Reflection**: Within 60 seconds of runtime service availability  
- **Error Propagation**: Within 15 seconds of error detection
- **Recovery Detection**: Within 30 seconds of issue resolution

## Lifecycle State Diagram

```
[Workload Created]
        ↓
[InputsValid Check] → [Failed] → [Manual Fix Required]
        ↓ (True)
[Generate ResourceClaims] 
        ↓
[Wait for All Bindings] → [Binding Failed] → [Error Status + Retry]
        ↓ (All Bound)
[ClaimsReady=True]
        ↓
[Generate WorkloadPlan]
        ↓  
[Runtime Materialization] → [Runtime Failed] → [Error Status + Retry]
        ↓ (Success)
[RuntimeReady=True]
        ↓
[Ready=True + Endpoint Available]
        ↓
[Steady State Monitoring]
```

### Image resolution

- If `containers.*.image != "."`, the value is treated as a concrete OCI reference.
- If `containers.*.image == "."`, the Orchestrator expects an image to be supplied at deploy time:
  - Typically via a `ResourceClaim` of type `image|build|buildpack` resolved by a Provisioner that builds and pushes an image.
  - The `WorkloadPlan` carries a projection such as:
    - `containers[].imageFrom: { bindingKey, outputKey: "image" }`
  - The Runtime consumes the plan plus binding outputs to set the final image used for execution.

### Endpoint population & aggregation

- Runtime determines an endpoint (if any) based on the chosen platform.
- Orchestrator reflects that value into `Workload.status.endpoint`.
- Only the Orchestrator updates `Workload.status`. Runtimes and Provisioners do not write there.

This lifecycle ensures that users have a simple, consistent interface while platforms maintain full control over resource provisioning and workload execution across diverse runtime environments.
