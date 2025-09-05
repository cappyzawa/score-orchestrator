# Custom Resource Definitions (CRDs)

This document defines the API surface for the Score Orchestrator.  
The public contract is intentionally minimal and runtime-agnostic. Internal resources exist for orchestration and are hidden from users via RBAC.

- **API Group/Version:** `score.dev/v1b1`
- **Kinds (no "Score" prefix):**
  - Public (user-facing): `Workload`
  - PF-facing (hidden from users): `PlatformPolicy`
  - Internal (platform-facing): `ResourceBinding`, `WorkloadPlan`

See also: [`rbac.md`](rbac.md), [`control-plane.md`](control-plane.md), [`lifecycle.md`](lifecycle.md), [`validation.md`](validation.md).

---

## Taxonomy & Visibility

### Public APIs (User-facing)
- **`Workload`** — The only resource users author and read.

### PF-facing (hidden from users via RBAC)
- **`PlatformPolicy`** — Platform-applied governance and defaults (cluster-scoped). No list/get/watch for tenants.

### Internal APIs (Platform-facing)
- **`ResourceBinding`** — Contract with resolvers for dependency provisioning.  
  Spec is created/updated by the Orchestrator; **status is written by Resolvers**.
- **`WorkloadPlan`** — Orchestrator-to-Runtime projection plan (single writer: Orchestrator).  
  Same name/namespace as the target `Workload` (OwnerRef set). Hidden from users.

**Single-writer principle:** Only the Orchestrator writes `Workload.status`. Runtime and Resolvers never update it.

---

## Workload (`score.dev/v1b1`) — Public (User-facing)

### Purpose
Users author **only** this resource to describe an application using Score semantics.  
Runtime selection and platform details are **not** part of this spec.

### Contract (at a glance)
- Users define app intent: **`containers`** (required), optional **`service`**, optional **`resources`** (abstract dependencies).
- No runtime-specific knobs. No indirection via ConfigMap templates, spec references, or custom includes.
- Readiness and troubleshooting use **abstract** status only (`endpoint`, abstract `conditions`, binding summaries).

### Spec — Top-level fields (and only these)
- **`containers`** (required): `map<string, ContainerSpec>`
- **`service`** (optional): `ServiceSpec`
- **`resources`** (optional): `map<string, ResourceRequest>`

> The shapes below are **conceptual** and align with Score v1b1. Exact OpenAPI/CEL live in `validation.md`.

#### ContainerSpec (conceptual)
- **`image`** (required): string
- `command` (optional): string[]
- `args` (optional): string[]
- `variables` (optional): `map<string,string>`  
  Values may include Score-style placeholders (e.g., `${resources.<key>.outputs.<name>}`).
- `files` (optional): `FileSpec[]`  
  Each file has `path` (required), optional `mode`, and **exactly one** of:
  - `content` (string), or
  - `binaryContent` (base64), or
  - `source` (string/URI-like; resolver-agnostic)
- `volumes` (optional): `VolumeSpec[]`  
  Each mount has `target` (path, required), optional `readOnly`, and an abstract `source` (resolved by platform/resolvers).
- `probes` (optional): liveness/readiness/startup (abstract)  
  For HTTP, expect at least `path` and `port` (scheme/headers optional).

#### ServiceSpec (conceptual)
- `ports` (optional): `PortSpec[]`  
  Each port: **`port`** (required, int), optional `name`, `protocol` (defaults to TCP), `targetPort` (defaults to `port`).

#### ResourceRequest (conceptual)
- **`type`** (required): string (e.g., `postgres`, `redis`, `s3`, …)
- `class` (optional): string (implementation-defined tier/size)
- `id` (optional): string (bind to existing instance)
- `params` (optional): object (free-form, resolver-defined)
- `metadata` (optional): object (labels/hints; non-functional)

### Out of scope (MUST NOT appear in `spec`)
- **`runtime` / `runtimeClass`** — runtime selection belongs to **`PlatformPolicy`**.
- **`specRef` / `configMapRef` / `template` / `includes`** — indirection/composition is **not part of this API**.  
  Use upstream tooling (Helm/Kustomize) before submitting the CR.

### Status (user-facing, minimal, abstract)
- **`endpoint: string|null`** — canonical URL if available; else `null`
- **`conditions[]`** — Kubernetes-style items with abstract reasons only  
  - **Types:** `Ready`, `BindingsReady`, `RuntimeReady`, `InputsValid`
  - **Reasons (fixed, abstract):**  
    `Succeeded`, `SpecInvalid`, `PolicyViolation`,  
    `BindingPending`, `BindingFailed`,  
    `ProjectionError`,  
    `RuntimeSelecting`, `RuntimeProvisioning`, `RuntimeDegraded`,  
    `QuotaExceeded`, `PermissionDenied`, `NetworkUnavailable`
  - **Message:** one neutral sentence; **no runtime-specific nouns**.
- **`bindings[]`** — summary per dependency:  
  `key`, `phase (Pending|Binding|Bound|Failed)`, `reason`, `message`, `outputsAvailable: bool`
- **Readiness rule:** `InputsValid=True AND BindingsReady=True AND RuntimeReady=True`

---

## PlatformPolicy (`score.dev/v1b1`) — PF-facing (hidden from users)

### Purpose
PF operators declare how `Workload` should be materialized: runtime class selection, defaults, and resolver routing.  
Tenants do not read or write this resource.

### Scope & Visibility
- **Cluster-scoped** (recommended). Hidden from users (no list/get/watch).

### Spec (conceptual)
- **Targeting**: label/namespace selectors to scope the policy to certain Workloads
- **Runtime class**: abstract runtime class to use (e.g., `kubernetes`, `ecs`, `nomad`)
- **Defaults**: platform-level defaults (e.g., replica policy, exposure patterns) within the abstract model
- **Resolver routing**: mapping from `ResourceRequest.type` → resolver class/config
- **Projection defaults**: default env/volume mapping rules if not specified by plan generation
- **Endpoint policy**: how to derive a single canonical endpoint (e.g., host/path templates)

> Implementation details are platform-specific and must not leak to users.  
> The Orchestrator interprets this policy; it is not directly visible in `Workload.status`.

---

## ResourceBinding (`score.dev/v1b1`) — Internal (contract with resolvers)

### Purpose
Represents an abstract dependency request derived from a `Workload`.  
Resolvers watch this resource, provision/bind concrete services, and publish outputs.

### Ownership & Visibility
- Created/updated (spec) by the **Orchestrator**; OwnerRef points to the Workload.
- **Hidden from users** via RBAC. Resolvers and runtime may read it.

### Spec (conceptual)
- **`workloadRef`**: name/namespace of the owning Workload
- **`key`**: the resource key from `Workload.spec.resources`
- **`type`**: resource type (e.g., `postgres`, `redis`)
- `class` (optional): tier/size
- `id` (optional): bind to an existing instance
- `params` (optional): free-form resolver config
- `deprovisionPolicy` (optional): `Delete | Retain | Orphan`

### Status (written by Resolvers)
- **`phase`**: `Pending → Binding → (Bound | Failed)` (may re-enter on reconcile)
- **`reason` / `message`**: short, neutral text (no runtime-specific nouns)
- **`outputs`**: at least one of:
  - `secretRef: { name }` (recommended for credentials)
  - `configMapRef: { name }`
  - `uri: string`
  - `cert: { secretName | data }`
- `outputsAvailable: bool` (convenience)
- `observedGeneration`, `lastTransitionTime`

> The Orchestrator aggregates Binding status into `Workload.status.bindings[]` and `BindingsReady`.

---

## WorkloadPlan (`score.dev/v1b1`) — Internal (Orchestrator → Runtime)

### Purpose
A runtime-agnostic **projection plan** that the Runtime consumes to materialize the application.  
It expresses **how to use** dependency outputs, not the outputs themselves.

### Ownership & Visibility
- Same name/namespace as the target `Workload`; OwnerRef set.
- **Single writer:** Orchestrator. Runtime is read-only. Hidden from users.

### Spec (conceptual)
- **`workloadRef.name`** and **`observedWorkloadGeneration`**
- **`runtimeClass`**: abstract runtime class (e.g., `kubernetes`, `ecs`, `nomad`)
- **`projection`**: minimal rules, e.g.:
  - `env[]: { name, from: { bindingKey, outputKey } }`
  - (optionally) `volumes[]` / `files[]` projection in the same spirit
- **`bindings[]`**: desired summaries of each dependency (`key`, `type`, optional `class/params`)

> Separation of concerns:  
> **Plan** carries **how to use** (mapping rules); **ResourceBinding** carries **what to provide** (outputs).

---

## Cross-resource linkage

- **Key-based mapping:** `WorkloadPlan.spec.projection` refers to dependencies by `bindingKey` (the key in `Workload.spec.resources`).  
  Each `ResourceBinding` is created for that key; its `status.outputs` provide concrete values.
- **Endpoint propagation:** The Runtime determines an endpoint (if any). The Orchestrator reflects it into `Workload.status.endpoint`.  
  At most one canonical endpoint is exposed to users.

---

## Abstract condition reasons — vocabulary (one-liners)

- **Succeeded** — all requirements satisfied and materialized.
- **SpecInvalid** — schema/CEL violations or unresolved references.
- **PolicyViolation** — violates `PlatformPolicy`.
- **BindingPending** — dependency provisioning in progress.
- **BindingFailed** — dependency provisioning failed.
- **ProjectionError** — plan requires outputs that are missing/mismatched.
- **RuntimeSelecting** — runtime class decision pending/deferred.
- **RuntimeProvisioning** — runtime materialization in progress.
- **RuntimeDegraded** — runtime reported unhealthy/degraded state.
- **QuotaExceeded** — quotas/capacity inadequate.
- **PermissionDenied** — missing privileges/credentials.
- **NetworkUnavailable** — endpoints unreachable or blocked.

---

## Conventions

- **Status subresource:** CRDs expose `status` as a subresource.  
- **Single-writer rules:**  
  - `Workload.status` — Orchestrator only  
  - `ResourceBinding.status` — Resolvers only  
  - `WorkloadPlan.spec` — Orchestrator only
- **Owner references:** `ResourceBinding` and `WorkloadPlan` carry OwnerRef to their `Workload` for cascading GC.
- **No runtime nouns in user-facing docs:** Never expose Deployment/Pod/ECS Task names to users.

See: [`control-plane.md`](control-plane.md) for who watches/writes what, and [`validation.md`](validation.md) for schema/CEL invariants.

