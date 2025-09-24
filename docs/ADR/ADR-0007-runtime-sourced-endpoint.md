# ADR-0007 — Runtime-sourced Endpoint (mirror-only, no fallback)

**Status:** Proposed
**Date:** 2025-09-24 (JST)
**Related ADRs:** ADR-0001 (Architecture / single writer), ADR-0002 (Community-scope Orchestrator), ADR-0003 (Workload-only UX simplification), ADR-0006 (Terminology: ResourceBinding → ResourceClaim)

---

## Context

`Workload.status.endpoint` must reflect the **canonical URL** by which a Runtime exposes a workload (Ingress, LB, external gateway, managed DNS, etc.). While the Orchestrator *could* heuristically derive a URL by observing cluster objects, any non-authoritative value risks being wrong or policy-breaking. We therefore prefer **accuracy and governance over always showing something**.

We must preserve two project invariants:

* **Users author only `Workload`**; platform details remain hidden.
* **Orchestrator is the single writer of `Workload.status`** with a runtime-agnostic, abstract vocabulary.

## Decision

Adopt a **runtime-only, mirror model**:

* A small **internal, runtime-authored status object** publishes exposure candidates.
* The Orchestrator **mirrors** the top candidate into `Workload.status.endpoint` **without any observation-based derivation**.
* If Runtime has not published an exposure, `endpoint` remains **`null`**.

### Internal kind (conceptual)

> Name: **`WorkloadExposure`** (group `score.dev/v1b1`, namespaced, **internal**)

* **Spec** (written by Orchestrator)

  * `workloadRef`: { `name`, `namespace`, `uid` }
  * `runtimeClass`: string (selected runtime)
  * `observedWorkloadGeneration`: int64 (causality tracking)
* **Status** (written **only by the Runtime Controller**)

  * `exposures[]` (ordered; higher takes precedence)

    * `url: string` (format: `uri`; e.g., `https://app.example.com`)
    * `priority: int` (optional)
    * `scope: string` (e.g., `Public|ClusterLocal|VPC|Other`)
    * `schemeHint: string` (e.g., `HTTP|HTTPS|GRPC|TCP|OTHER`)
    * `reachable: bool|null` (optional)
  * `conditions[]` (Runtime’s detailed view)

### Orchestrator behavior (mirror-only)

1. **Watch** `WorkloadExposure.status`.
2. If `exposures[0].url` exists and is a valid URI, set `Workload.status.endpoint = url` (mirror; URI validation only).
3. **Normalize conditions** into the abstract vocabulary on `Workload.status.conditions` (e.g., `RuntimeProvisioning`, `NetworkUnavailable`, `Succeeded`, `RuntimeDegraded`).
4. If no valid exposure exists, leave `endpoint = null`. **Do not derive** from Services/Ingresses/etc.

### What we do **not** change

* **Single-writer principle**: Only Orchestrator writes `Workload.status`.
* **User-facing minimalism**: No runtime-specific nouns leak into `Workload.status`.
* **Readiness rule** is unchanged: `InputsValid=True AND ClaimsReady=True AND RuntimeReady=True`.

## Rationale

* **Authority & correctness**: Only Runtime knows the canonical URL across gateways/DNS/cutovers.
* **Governance**: Avoid accidental publication of non-approved or internal endpoints.
* **Simplicity**: No heuristics, no observation watchers for endpoint logic.

## Specification sketch (docs-first, no YAML)

### Contract (fields)

* `WorkloadExposure.spec.workloadRef` (required)
* `WorkloadExposure.spec.runtimeClass` (required)
* `WorkloadExposure.spec.observedWorkloadGeneration` (required)
* `WorkloadExposure.status.exposures[]` (0..N; `url` required)
* `WorkloadExposure.status.conditions[]` (mappable to abstract `RuntimeReady`)

### Endpoint mirror rule

* If a valid `exposures[0].url` exists → set `Workload.status.endpoint = url`.
* Optional policy: if `scope != "Public"` and platform policy hides non-public exposure → keep `endpoint = null`.
* Runtime conditions are mapped to abstract reasons:

  * `Healthy/Available` → `Succeeded`
  * `Selecting/Provisioning/Applying` → `RuntimeProvisioning`
  * `LBPending/IngressPending/DNSNotReady` → `NetworkUnavailable`
  * `Degraded` → `RuntimeDegraded`
  * (Policy/Quota/Permission errors map to existing abstract reasons)

### No derivation

* The Orchestrator **does not** compute endpoints from observed K8s objects (Ingress/LB/NodePort/ClusterIP) nor any external discovery. If Runtime is silent, `endpoint` remains `null`.

## Impact on docs

* **`docs/spec/lifecycle.md`**: After `WorkloadPlan` emission: (1) Orchestrator creates `WorkloadExposure` (spec only); (2) Runtime updates `WorkloadExposure.status` with exposures; (3) Orchestrator mirrors into `Workload.status.endpoint` and normalizes conditions. Remove mentions of observation-based derivation.
* **`docs/spec/crds.md`**: Add **internal** `WorkloadExposure` to conceptual list; mark as PF-internal, hidden from users. Terminology uses **ResourceClaim**.
* **`docs/spec/rbac.md`**: Orchestrator needs `create/get/list/watch/delete` on `workloadexposures` and read on `/status`. It **does not** require Services/Ingresses read access for endpoint logic anymore.
* **`docs/spec/endpoint-derivation.md`**: Rename/reshape to **endpoint mirror** spec or explicitly state "runtime-only". Remove derivation rules.

## Implementation notes (non-code)

* **Generation tracking**: Ignore stale reports via `observedWorkloadGeneration`.
* **Flap-resistance**: Patch `Workload.status.endpoint` only when the value changes; ignore empty/invalid URLs.
* **Template escape hatch (optional)**: Platforms may still define `endpointTemplate` to *override* (policy-driven); default remains runtime-sourced.

## Alternatives considered

1. **Observation-based derivation (fallback)**
   *Rejected.* Adds heuristics, risks incorrect URLs, complicates policy/governance.

2. **Runtime writes `Workload.status` directly**
   *Rejected.* Violates single-writer principle; invites races and RBAC complexity.

3. **Annotate Service/Ingress for discovery**
   *Rejected.* K8s-specific and still heuristic; not portable to non-K8s runtimes.

## Consequences

* **+** Clear, authoritative contract; simpler Orchestrator; fewer moving parts.
* **+** Better security/governance: only Runtime-approved URLs surface to users.
* **±** Requires Runtime change to publish exposures; until then, users see `endpoint=null` + abstract conditions.
* **–** No "best-effort" URL for non-compliant runtimes/environments.
