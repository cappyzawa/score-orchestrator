
# Controllers & Control Plane Responsibilities

This document defines who watches what and who writes what in the Score Orchestrator control plane.  
It complements:
- `docs/spec/crds.md` (API shapes & visibility)
- `docs/spec/rbac.md` (who may read/write)
- `docs/spec/lifecycle.md` (time-ordered flow)

## Controller-centric responsibilities (watch vs. write)

### Orchestrator (reference project, independent from Score Official)
- **Watches:** `Workload`, `ResourceClaim`, `WorkloadPlan`
- **Reads:** **Orchestrator Config** (ConfigMap/OCI) and applies Admission
- **Creates/updates (spec):**
  - `ResourceClaim` — one per `Workload.spec.resources.<key>` (OwnerRef = Workload)
  - `WorkloadPlan` — same name as the target Workload (OwnerRef = Workload)
- **Updates (status):**
  - **`Workload.status`** — the *only* writer (exposes `endpoint`, abstract `conditions`, claim summaries)
- **Finalization:**
  - Adds a finalizer to `Workload` to ensure `ResourceClaim` deprovision completes before removal
  - Processes `ResourceClaim` deletion according to `DeprovisionPolicy` before removing Workload finalizer

### Provisioner (PF/vendor)
- **Watches:** `ResourceClaim` for its `spec.type`; own `Secret/ConfigMap`; external service APIs as needed
- **Creates/updates (objects):** `Secret/ConfigMap` with credentials/config (same namespace)
- **Updates (status):** `ResourceClaim.status` (`phase`, `reason`, `message`, `outputs`, timestamps)
- **Produces image outputs (when applicable):** Provisioners for `image|build|buildpack` types publish an OCI reference as `ResourceClaim.status.outputs.image`.
- **Plan linkage:** The Orchestrator emits a `WorkloadPlan` projection that binds that output into the final container image, e.g.:
  - `containers[].imageFrom: { claimKey, outputKey: "image" }`
- **Finalization:** On `ResourceClaim` deletion, respects `DeprovisionPolicy`:
  - `Delete` (default): deprovision external resources / Secrets, then remove finalizer
  - `Retain`: keep provisioned resources but unbind from Workload
  - `Orphan`: leave resources as-is without cleanup

### Runtime Controller (PF)
- **Watches:** `WorkloadPlan` (primary), `ResourceClaim` (consume `status.outputs`), `Workload` (labels/metadata)
- **Creates/updates (objects):** runtime-specific child resources (e.g., Deployments/Services/etc. on Kubernetes)
- **Updates (status):** *Does not write* `Workload.status`; may write its own internal report CR
- **Finalization:** Cleans up runtime children when `WorkloadPlan` changes or is deleted

## Resource-centric matrix (who reads/writes what)

| Resource (`score.dev/v1b1`) | Read/watch                                 | Write (spec/create)           | Write (status)            | Notes |
|---|---|---|---|---|
| `Workload` (public)         | Orchestrator, Runtime                      | Users only                    | **Orchestrator only**     | Orchestrator attaches a finalizer to control deletion order |
| `ResourceClaim` (internal) | Orchestrator, Provisioner, Runtime       | **Orchestrator**              | **Provisioner**           | One per `resources.<key>` |
| `WorkloadPlan` (internal)   | Runtime, Orchestrator                      | **Orchestrator** (single writer) | —                      | Same name as Workload; OwnerRef = Workload |
| `Secret/ConfigMap` (outputs) | Runtime (read), Orchestrator (not required) | **Provisioner**              | —                         | Same namespace; hidden from users |

## Claim ↔ Plan linkage (how they meet)
- **Key-based mapping:** `WorkloadPlan.spec.projection` refers to dependencies by `claimKey` (the key in `Workload.spec.resources`).  
  Each `ResourceClaim` is created for that key; its `status.outputs` provide the concrete values.
- **Separation of concerns:** Plan carries **how to use** (projection rules), Claim carries **what to provide** (outputs).

**Example: Image resolution flow:**
```yaml
# ResourceClaim.status.outputs (by Provisioner)
outputs:
  image: "registry.example.com/myapp:v1.2.3"

# WorkloadPlan.spec.projection (by Orchestrator)  
projection:
  imageFrom: { claimKey: "build-tool", outputKey: "image" }
```

## Orchestrator Plan Emission Gate

Before emitting a `WorkloadPlan`, the Orchestrator performs **unresolved placeholder detection**:

1. **Values Composition**: Combine template defaults, normalized Workload spec, and ResourceClaim outputs
2. **Placeholder Scanning**: Traverse all string fields in composed values to detect `${...}` patterns
3. **Emission Control**: If unresolved placeholders found, skip Plan creation and set `RuntimeReady=False` with `Reason=ProjectionError`
4. **Recovery Path**: Automatic reconciliation when ResourceClaim outputs become available

This ensures no unresolved placeholders reach the runtime while providing clear, abstract feedback to users.

## Error mapping (abstract reasons for `Workload.status`)
- Claim in progress/failure → `ClaimPending` / `ClaimFailed`
- Unresolved placeholders prevent plan emission → `ProjectionError`
- Runtime health/materialization issues → `RuntimeDegraded` (no runtime-specific nouns in messages)
