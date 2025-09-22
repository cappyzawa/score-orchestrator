# Custom Resource Definitions (CRDs)

This document defines the API surface for the Score Orchestrator.  
The public contract is intentionally minimal and runtime-agnostic. Internal resources exist for orchestration and are hidden from users via RBAC.

- **API Group/Version:** `score.dev/v1b1`
- **Kinds (no "Score" prefix):**
  - Public (user-facing): `Workload`
  - Internal (platform-facing): `ResourceClaim`, `WorkloadPlan`

Policy and profile/backends mapping live in an **Orchestrator configuration** artifact (ConfigMap or OCI), not a public CRD.

See also: [`rbac.md`](rbac.md), [`control-plane.md`](control-plane.md), [`lifecycle.md`](lifecycle.md), [`validation.md`](validation.md).

---

## Taxonomy & Visibility

### Public APIs (User-facing)
- **`Workload`** — The only resource users author and read.

### Internal APIs (Platform-facing)
- **`ResourceClaim`** — Contract with **provisioners** for dependency provisioning.
  Spec is created/updated by the Orchestrator; **status is written by Provisioners**.
- **`WorkloadPlan`** — Orchestrator-to-Runtime projection plan (single writer: Orchestrator).  
  Same name/namespace as the target `Workload` (OwnerRef set). Hidden from users.

---

## Workload (`score.dev/v1b1`) — Public (User-facing)

### Purpose
Users author **only** this resource to describe an application using Score semantics.  
Runtime selection and platform details are **not** part of this spec.

### Contract (at a glance)
- Users define app intent: **`containers`** (required), optional **`service`**, optional **`resources`** (abstract dependencies).
- No runtime-specific knobs. No indirection via ConfigMap templates, spec references, or custom includes.
- Readiness and troubleshooting use **abstract** status only (`endpoint`, abstract `conditions`, claim summaries).

#### Required/Optional Summary

**Workload (spec)**

| Field        | Req     | Notes                              |
| ------------ | ------- | ---------------------------------- |
| `containers` | **Yes** | `map<string, ContainerSpec>`       |
| `service`    | No      | `ServiceSpec`                      |
| `resources`  | No      | `map<string, ResourceRequest>`     |

**Workload (status)**

| Field        | Req     | Notes                              |
| ------------ | ------- | ---------------------------------- |
| `endpoint`   | No      | canonical URL if available (format: uri) |
| `conditions` | **Yes** | Kubernetes-style condition array   |
| `claims`     | No      | summary per dependency             |

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
- **`runtime` / `runtimeClass`** — runtime selection belongs to **the Orchestrator configuration + Admission** (not part of this API).
- **`specRef` / `configMapRef` / `template` / `includes`** — indirection/composition is **not part of this API**.  
  Use upstream tooling (Helm/Kustomize) before submitting the CR.

### Status (user-facing, minimal, abstract)
- **`endpoint: string|null`** — canonical URL if available; else `null` (format: uri)
- **`conditions[]`** — Kubernetes-style items with abstract reasons only  
  - **Types:** `Ready`, `ClaimsReady`, `RuntimeReady`, `InputsValid`
  - **Reasons (fixed, abstract):**
    `Succeeded`, `SpecInvalid`, `PolicyViolation`,
    `ClaimPending`, `Claiming`, `ClaimFailed`,
    `ProjectionError`,
    `RuntimeSelecting`, `RuntimeProvisioning`, `RuntimeDegraded`,
    `QuotaExceeded`, `PermissionDenied`, `NetworkUnavailable`
  - **Message:** one neutral sentence; **no runtime-specific nouns**.
- **`claims[]`** — summary per dependency:  
  `key`, `phase (Pending|Claiming|Bound|Failed)`, `reason`, `message`, `outputsAvailable: bool`
- **Readiness rule:** `InputsValid=True AND ClaimsReady=True AND RuntimeReady=True`

### Orchestrator configuration (non-CRD, conceptual)

A single **orchestrator config** (ConfigMap or OCI document) becomes the source of truth for platform mapping:

* `profiles[]` — abstract profile entries (e.g., `web-service`, `batch-job`, `event-consumer`, `function`), each mapping to one or more **backend candidates**:
  * `backendId` (stable identifier, not user-visible)
  * `runtimeClass` (e.g., `kubernetes`, `ecs`, `nomad`)
  * `template` { `kind`: `manifests` | `helm` | `…`, `ref`: immutable reference (OCI digest recommended) }
  * `priority` / `version` / `constraints` (selectors by namespace/labels/region; feature requirements like `scale-to-zero`)
* `provisioners[]` — dependency realization definitions aligned with Score resources.
* `defaults` — profile defaults by selector (namespace/labels), plus a global default.

Templates are declarative (no embedded functions) and should use immutable refs (OCI digest recommended).

---

## ResourceClaim (`score.dev/v1b1`) — Internal (contract with provisioners)

### Purpose
Represents an abstract dependency request derived from a `Workload`.
Provisioners watch this resource, provision/bind concrete services, and publish outputs.

### Ownership & Visibility
- Created/updated (spec) by the **Orchestrator**; OwnerRef points to the Workload.
- **Hidden from users** via RBAC. Provisioners and runtime may read it.

#### Required/Optional Summary

**ResourceClaim (spec)**

| Field                            | Req     | Notes                               |
| -------------------------------- | ------- | ----------------------------------- |
| `workloadRef.name` / `namespace` | **Yes** | points to owning Workload           |
| `key`                            | **Yes** | key under `Workload.spec.resources` |
| `type`                           | **Yes** | abstract type (e.g., `postgresql`)  |
| `class`                          | No      | resolver subclass                   |
| `id`                             | No      | existing instance pin               |
| `params`                         | No      | `JSON` (opaque)                     |
| `deprovisionPolicy`              | No      | Enum (Delete/Retain/Orphan)         |

**ResourceClaim (status)**

| Field                                       | Req     | Notes                                     |
| ------------------------------------------- | ------- | ----------------------------------------- |
| `phase`                                     | **Yes** | `Pending \| Claiming \| Bound \| Failed`   |
| `reason` / `message`                        | No      | abstract                                  |
| `outputs`                                   | No*     | pointer type: nil when unavailable, CEL validates when present |
| `outputsAvailable`                          | **Yes** | boolean gate for consumers                |
| `observedGeneration` / `lastTransitionTime` | No      | bookkeeping                               |

### Spec (conceptual)
- **`workloadRef`**: name/namespace of the owning Workload
- **`key`**: the resource key from `Workload.spec.resources`
- **`type`**: resource type (e.g., `postgres`, `redis`)
- `class` (optional): tier/size
- `id` (optional): bind to an existing instance
- `params` (optional): free-form resolver config
- `deprovisionPolicy` (optional): Enum { **Delete**, **Retain**, **Orphan** }.
  Defines how provisioned resources are handled when the claim is removed.

### Status (written by Provisioners)
- **`phase`**: `Pending → Claiming → (Bound | Failed)` (may re-enter on reconcile)
- **`reason` / `message`**: short, neutral text (no runtime-specific nouns)
- **`outputs` (standardized)**: Shape:
  ```yaml
  outputs:
    secretRef: { name: string }                # optional
    configMapRef: { name: string }             # optional
    uri: string                                # optional (e.g., jdbc:, redis:, https:)
    image: string                              # optional (OCI image reference)
    cert:                                      # optional
      secretName: string?                      # reference to Secret containing material
      data: { <filename>: base64-bytes }?      # inlined certificate/key material
  ```
  
  **Constraint (normative):** at least one of the above must be set.
  
  * CEL (normative example used by the CRD):
    ```
    has(self.secretRef) || has(self.configMapRef) || has(self.uri) || has(self.image) || has(self.cert)
    ```
- `outputsAvailable: bool` MUST be `true` iff the provisioner has published a valid `outputs`
  object (i.e., the CEL condition evaluates to true).
- `observedGeneration`, `lastTransitionTime`

> The Orchestrator aggregates Claim status into `Workload.status.claims[]` and `ClaimsReady`.

---

## WorkloadPlan (`score.dev/v1b1`) — Internal (Orchestrator → Runtime)

### Purpose
A runtime-agnostic **projection plan** that the Runtime consumes to materialize the application.  
It expresses **how to use** dependency outputs, not the outputs themselves.

For example, `imageFrom: { claimKey, outputKey }` may be used to bind an `outputs.image` into the final container image.

### Ownership & Visibility
- Same name/namespace as the target `Workload`; OwnerRef set.
- **Single-writer:** Orchestrator. Runtime is read-only. Hidden from users.

#### Required/Optional Summary

**WorkloadPlan (spec)**

| Field                          | Req     | Notes                                |
| ------------------------------ | ------- | ------------------------------------ |
| `workloadRef.name` / `namespace` | **Yes** | reference to target Workload         |
| `observedWorkloadGeneration`   | **Yes** | tracks Workload changes              |
| `runtimeClass`                 | **Yes** | abstract runtime (e.g., kubernetes) |
| `projection`                   | No      | env/volume mapping rules             |
| `claims`                       | No      | desired dependency summaries         |

**WorkloadPlan (status)**

| Field        | Req     | Notes                              |
| ------------ | ------- | ---------------------------------- |
| `phase`      | **Yes** | runtime execution phase            |
| `conditions` | **Yes** | Kubernetes-style condition array   |
| `endpoint`   | No      | runtime-provided service endpoint  |

### Spec (conceptual)
- **`workloadRef.name`** and **`observedWorkloadGeneration`**
- **`runtimeClass`**: abstract runtime class (e.g., `kubernetes`, `ecs`, `nomad`)
- **`projection`**: minimal rules, e.g.:
  - `env[]: { name, from: { claimKey, outputKey } }`
  - `imageFrom: { claimKey, outputKey }`
  - (optionally) `volumes[]` / `files[]` projection in the same spirit
- **`claims[]`**: desired summaries of each dependency (`key`, `type`, optional `class/params`)

> Separation of concerns:  
> **Plan** carries **how to use** (mapping rules); **ResourceClaim** carries **what to provide** (outputs).

---

## Cross-resource linkage

- **Key-based mapping:** `WorkloadPlan.spec.projection` refers to dependencies by `claimKey` (the key in `Workload.spec.resources`).  
  Each `ResourceClaim` is created for that key; its `status.outputs` provide concrete values.
- **Endpoint propagation:** The Runtime determines an endpoint (if any). The Orchestrator reflects it into `Workload.status.endpoint`.  
  At most one canonical endpoint is exposed to users.

---

## Abstract condition reasons — vocabulary (one-liners)

- **Succeeded** — all requirements satisfied and materialized.
- **SpecInvalid** — schema/CEL violations or unresolved references.
- **PolicyViolation** — violates platform policy (Orchestrator config + Admission).
- **ClaimPending** — dependency provisioning in progress.
- **ClaimFailed** — dependency provisioning failed.
- **ProjectionError** — plan requires outputs that are missing/mismatched.
- **RuntimeSelecting** — runtime class decision pending/deferred.
- **RuntimeProvisioning** — runtime materialization in progress.
- **RuntimeDegraded** — runtime reported unhealthy/degraded state.
- **QuotaExceeded** — quotas/capacity inadequate.
- **PermissionDenied** — missing privileges/credentials.
- **NetworkUnavailable** — endpoints unreachable or blocked.

---

## Types & Conventions

### Opaque JSON fields

Fields that intentionally carry arbitrary JSON **MUST** use the Kubernetes type
`apiextensions.k8s.io/v1.JSON` in the Go schema (and equivalent in OpenAPI).
Do not use `interface{}` / `any` / `runtime.RawExtension` for these fields.

Affected fields:
- `ResourceClaim.spec.params`
- `WorkloadPlan.spec.claims[].params`

---

## Conventions

- **Status subresource:** CRDs expose `status` as a subresource.  
- **Status authorship (Single-writer principle):**  
  - `Workload.status`: written **only** by the Orchestrator.
  - `ResourceClaim.status`: written **only** by Provisioner implementations.
  - `WorkloadPlan.status`: written **only** by Runtime Controllers. Runtime controllers **must not** write to `Workload.status`;
    they publish detailed diagnostics to `WorkloadPlan.status`.
  
  See `docs/spec/rbac.md` for recommended RBAC roles.
- **Owner references:** `ResourceClaim` and `WorkloadPlan` carry OwnerRef to their `Workload` for cascading GC.
- **No runtime nouns in user-facing docs:** Never expose Deployment/Pod/ECS Task names to users.

See: [`control-plane.md`](control-plane.md) for who watches/writes what, and [`validation.md`](validation.md) for schema/CEL invariants.

