
# Controllers & Control Plane Responsibilities

This document defines who watches what and who writes what in the Score Orchestrator control plane.  
It complements:
- `docs/spec/crds.md` (API shapes & visibility)
- `docs/spec/rbac.md` (who may read/write)
- `docs/spec/lifecycle.md` (time-ordered flow)

## Controller-centric responsibilities (watch vs. write)

### Orchestrator (community)
- **Watches:** `Workload`, `PlatformPolicy`, `ResourceBinding`, `WorkloadPlan`
- **Creates/updates (spec):**
  - `ResourceBinding` — one per `Workload.spec.resources.<key>` (OwnerRef = Workload)
  - `WorkloadPlan` — same name as the target Workload (OwnerRef = Workload)
- **Updates (status):**
  - **`Workload.status`** — the *only* writer (exposes `endpoint`, abstract `conditions`, binding summaries)
- **Finalization:**
  - Adds a finalizer to `Workload` to ensure `ResourceBinding` deprovision completes before removal

### Resolver (PF/vendor)
- **Watches:** `ResourceBinding` for its `spec.type`; own `Secret/ConfigMap`; external service APIs as needed
- **Creates/updates (objects):** `Secret/ConfigMap` with credentials/config (same namespace)
- **Updates (status):** `ResourceBinding.status` (`phase`, `reason`, `message`, `outputs`, timestamps)
- **Finalization:** On `ResourceBinding` deletion, deprovision external resources / Secrets, then remove finalizer

### Runtime Controller (PF)
- **Watches:** `WorkloadPlan` (primary), `ResourceBinding` (consume `status.outputs`), `Workload` (labels/metadata)
- **Creates/updates (objects):** runtime-specific child resources (e.g., Deployments/Services/etc. on Kubernetes)
- **Updates (status):** *Does not write* `Workload.status`; may write its own internal report CR
- **Finalization:** Cleans up runtime children when `WorkloadPlan` changes or is deleted

## Resource-centric matrix (who reads/writes what)

| Resource (`score.dev/v1b1`) | Read/watch                                 | Write (spec/create)           | Write (status)            | Notes |
|---|---|---|---|---|
| `Workload` (public)         | Orchestrator, Runtime                      | Users only                    | **Orchestrator only**     | Orchestrator attaches a finalizer to control deletion order |
| `PlatformPolicy` (PF, hidden) | Orchestrator                              | PF operators                  | —                         | Cluster-scoped; hidden from users via RBAC |
| `ResourceBinding` (internal) | Orchestrator, Resolver, Runtime           | **Orchestrator**              | **Resolver**              | One per `resources.<key>` |
| `WorkloadPlan` (internal)   | Runtime, Orchestrator                      | **Orchestrator** (single writer) | —                      | Same name as Workload; OwnerRef = Workload |
| `Secret/ConfigMap` (outputs) | Runtime (read), Orchestrator (not required) | **Resolver**                 | —                         | Same namespace; hidden from users |

## Binding ↔ Plan linkage (how they meet)
- **Key-based mapping:** `WorkloadPlan.spec.projection` refers to dependencies by `bindingKey` (the key in `Workload.spec.resources`).  
  Each `ResourceBinding` is created for that key; its `status.outputs` provide the concrete values.
- **Separation of concerns:** Plan carries **how to use** (projection rules), Binding carries **what to provide** (outputs).

## Error mapping (abstract reasons for `Workload.status`)
- Binding in progress/failure → `BindingPending` / `BindingFailed`
- Outputs missing/mismatched vs plan → `ProjectionError`
- Runtime health/materialization issues → `RuntimeDegraded` (no runtime-specific nouns in messages)
