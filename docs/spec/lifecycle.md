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
   - `ClaimsReady=False` (no claims created yet)  
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
- **State**: Waiting for Provisioner Controller to claim the resource
- **Orchestrator Status**: Updates `Workload.status.claims[].phase=Pending`

#### Binding Phase
- **Trigger**: Provisioner Controller begins provisioning the resource
- **State**: Active provisioning of the required dependency
- **Orchestrator Status**: Updates `Workload.status.claims[].phase=Binding`

#### Bound Phase (Success)
- **Trigger**: Provisioner Controller successfully provisions resource and populates `status.outputs`
- **State**: Resource ready for consumption, outputs available
- **Orchestrator Status**: 
  - Updates `Workload.status.claims[].phase=Bound`
  - Sets `outputsAvailable=true`
  - If all claims are Bound, sets `ClaimsReady=True`

#### Failed Phase (Error)
- **Trigger**: Provisioner Controller encounters unrecoverable error
- **State**: Resource provisioning failed, outputs unavailable  
- **Orchestrator Status**:
  - Updates `Workload.status.claims[].phase=Failed`
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
1. **Values Composition**: Combine values using precedence: **`defaults ⊕ normalize(Workload) ⊕ outputs`** (right-hand wins)

2. **Unresolved Placeholder Detection**: Before emitting the WorkloadPlan, scan composed values for `${...}` patterns:
   - If unresolved placeholders are found, **skip Plan emission**
   - Set `RuntimeReady=False` with `Reason=ProjectionError`
   - Set `Message="One or more required outputs are not resolved."`
   - Requeue for later reconciliation when outputs become available

3. **WorkloadPlan Creation** (only when all placeholders resolved): Generate comprehensive execution plan with:
   - **Projection Rule Generation**: Create container environment variable mappings:
     ```
     env:
       - name: DATABASE_URL
         from:
           claimKey: "database"
           outputKey: "connectionString"
     ```
   - **Container Projections**: Environment variables, volume mounts, file injections
   - **Service Configuration**: Port mappings, ingress rules, network policies
   - **Claim Dependencies**: Required outputs and criticality indicators
   - **Runtime Metadata**: Labels, annotations, ownership mode

4. **Plan Validation**: Ensure plan completeness and consistency

### Runtime materialization lifecycle (internal)

> These phases are tracked by **runtime-internal resources**; `WorkloadPlan.status` is managed by the Runtime Controller.

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
5. **Projection Error**: `RuntimeReady=False`, `reason=ProjectionError` (unresolved placeholders prevent Plan emission)
6. **Other Failures**: `RuntimeReady=False`, various reasons (platform-specific errors)

## Phase 5: Runtime Exposure and Endpoint Reflection

### WorkloadExposure Creation and Management

The WorkloadExposure resource is managed through a dedicated controller flow:

#### WorkloadExposureRegistrar Controller Actions
1. **Workload Monitoring**: Watches `Workload` resources for creation/updates
2. **WorkloadExposure Registration**: For each Workload, creates corresponding `WorkloadExposure` with:
   - `spec.workloadRef`: Reference to target Workload (with UID for identity)
   - `spec.runtimeClass`: Selected runtime (e.g., `kubernetes`, `ecs`, `nomad`)
   - `spec.observedWorkloadGeneration`: Tracks Workload changes for causality
3. **Spec-only Management**: WorkloadExposureRegistrar writes only the spec; status remains empty
4. **Lifecycle Coupling**: Uses OwnerReference for automatic cleanup when Workload is deleted

#### Runtime Controller Actions
Once the Runtime Controller successfully materializes platform resources and determines exposure endpoints:

1. **Exposure Detection**: Monitor platform-specific resources (Ingress, LoadBalancer, Gateway, etc.)
2. **Status Publication**: Update `WorkloadExposure.status` with:
   ```yaml
   status:
     exposures:
       - url: "https://app.example.com"
         priority: 100
         scope: "Public"
         schemeHint: "HTTPS"
         reachable: true
     conditions:
       - type: "ExposureReady"
         status: "True"
         reason: "Succeeded"
   ```
3. **Authority**: Runtime Controller is the **sole writer** of `WorkloadExposure.status`

#### ExposureMirror Controller Actions
The ExposureMirror Controller provides the final step in the endpoint reflection flow:

1. **WorkloadExposure Monitoring**: Watches `WorkloadExposure.status` for Runtime-published endpoints
2. **Endpoint Mirroring**: Mirrors `exposures[0].url` to `Workload.status.endpoint` after validation
3. **Condition Normalization**: Converts Runtime-specific conditions to abstract user-facing conditions
4. **Identity Verification**: Validates `workloadRef.uid` to prevent stale updates from recreated resources
5. **Generation Guards**: Uses `observedWorkloadGeneration` to ignore outdated exposure data

### Orchestrator Status Aggregation

The main Orchestrator Controller and ExposureMirror Controller coordinate to maintain the **single source of truth** for `Workload.status`:

#### Ready Computation Logic

```
Ready = InputsValid ∧ ClaimsReady ∧ RuntimeReady

where:
  InputsValid = (spec validation passed ∧ policy compliance verified)
  ClaimsReady = (all ResourceClaims in Bound phase ∧ outputs available)
  RuntimeReady = (platform materialization successful ∧ workload functional)
```

#### Endpoint Mirror Process

The `status.endpoint` field follows this **mirror-only** update sequence:

1. **Initial State**: `endpoint: null` (no exposure published yet)
2. **Runtime Publishes**: Runtime Controller updates `WorkloadExposure.status.exposures[]`
3. **Orchestrator Mirrors**: Orchestrator reads `WorkloadExposure.status.exposures[0].url`
4. **URI Validation**: Validate URL format and mirror to `Workload.status.endpoint`
5. **Condition Normalization**: Map Runtime conditions to abstract reasons
6. **User Visibility**: Endpoint becomes available via `kubectl get workload`

#### Endpoint Mirror Policy

The Orchestrator **mirrors** `WorkloadExposure.status.exposures[0].url` **as-is**, after URI validation.
- **Generation Guard**: Ignore stale updates via `observedWorkloadGeneration`
- **No Derivation**: Never compute endpoints from platform resources
- **Null**: If no valid exposure exists, `endpoint` remains `null`

**No Observation-Based Derivation:**
- The Orchestrator does **not** observe Services, Ingresses, or LoadBalancers
- The Orchestrator does **not** compute endpoints from platform resources
- Only Runtime-published exposures are mirrored to user status

Only **one endpoint** is exposed in `status.endpoint` to maintain interface simplicity.

## Error Handling and Recovery

### Transient Errors
- **Network Issues**: `reason=NetworkUnavailable`, automatic retry with exponential backoff
- **Resource Contention**: `reason=RuntimeProvisioning`, wait for platform resource availability
- **Quota Limits**: `reason=QuotaExceeded`, pause until quota available

### Permanent Errors
- **Specification Errors**: `reason=SpecInvalid`, requires user intervention
- **Policy Violations**: `reason=PolicyViolation`, requires compliance or policy change
- **Resource Claim Failures**: `reason=ClaimFailed`, requires dependency resolution

### Transient Errors (Automatic Recovery)
- **Projection Errors**: `reason=ProjectionError`, automatic retry when resolver outputs become available

### Recovery Mechanisms

#### Automatic Recovery
- Provisioner Controllers retry failed claims with exponential backoff
- Runtime Controllers automatically reconcile platform resource drift
- Network partitions trigger re-evaluation of dependent resource status

#### Manual Recovery
- Users can trigger reconciliation by updating Workload metadata annotations
- Platform operators can reset claim states by deleting and recreating ResourceClaims
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
[Wait for All Claims] → [Claim Failed] → [Error Status + Retry]
        ↓ (All Bound)
[ClaimsReady=True]
        ↓
[Check for Unresolved Placeholders] → [ProjectionError] → [Requeue + Wait for Outputs]
        ↓ (All Resolved)
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
    - `containers[].imageFrom: { claimKey, outputKey: "image" }`
  - The Runtime consumes the plan plus claim outputs to set the final image used for execution.


## Phase 6: Workload Deletion and Cleanup

### Deletion Flow Overview

When a user deletes a Workload resource, the system follows a controlled cleanup sequence to ensure proper resource deprovisioning and avoid data loss.

```
User deletes Workload
    ↓
Orchestrator adds finalizer (if not present)
    ↓
Orchestrator processes ResourceClaim deletion according to DeprovisionPolicy
    ↓
Wait for ResourceClaim cleanup completion
    ↓
Orchestrator removes finalizer
    ↓
Workload deleted by Kubernetes GC
```

### Finalizer Control

The Orchestrator uses a finalizer (`workloads.score.dev/finalizer`) to control deletion ordering:

1. **Finalizer Addition**: Added automatically when ResourceClaims are created
2. **Deletion Processing**: Processes each ResourceClaim according to its `DeprovisionPolicy`
3. **Cleanup Verification**: Waits for all ResourceClaims with `Delete` policy to be removed
4. **Finalizer Removal**: Removes finalizer only after cleanup completion

### DeprovisionPolicy Behavior

Each ResourceClaim can specify how it should be handled during deletion:

#### Delete Policy (Default)
- **Behavior**: Standard deletion via OwnerReference
- **Wait Condition**: Orchestrator waits for complete removal
- **Use Case**: Temporary resources that should be cleaned up

#### Retain Policy
- **Behavior**: Remove OwnerReference, keep ResourceClaim
- **Wait Condition**: No waiting required
- **Use Case**: Persistent resources that should survive Workload deletion

#### Orphan Policy
- **Behavior**: Leave ResourceClaim unchanged
- **Wait Condition**: No waiting required
- **Use Case**: Shared resources managed independently

### Deletion State Transitions

```
[User deletes Workload]
        ↓
[Orchestrator detects deletion]
        ↓
[Process each ResourceClaim] → [Apply DeprovisionPolicy]
        ↓                              ↓
[Count claims needing deletion] ← [Delete/Retain/Orphan]
        ↓
[claimsToWaitFor > 0?] → Yes → [Requeue and wait]
        ↓ No                        ↓
[Remove finalizer]                   [Check again later]
        ↓
[Workload deleted]
```

### Error Handling During Deletion

- **DeprovisionPolicy Processing Errors**: Log error and requeue for retry
- **Finalizer Removal Errors**: Log error and requeue for retry
- **ResourceClaim Enumeration Errors**: Return error and requeue

This lifecycle ensures that users have a simple, consistent interface while platforms maintain full control over resource provisioning and workload execution across diverse runtime environments.
