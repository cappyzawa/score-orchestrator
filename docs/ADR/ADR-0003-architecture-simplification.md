# ADR-0003 — Control-plane simplification around a "Workload-only" UX

**Status:** Accepted
**Date:** 2025-09-13 (JST)
**Discussion:** https://github.com/score-spec/spec/discussions/157
**Feedback trigger:** https://github.com/score-spec/spec/discussions/157#discussioncomment-14375929
**Related ADRs:** ADR-0001 (Architecture), ADR-0002 (Community-scope Orchestrator)

---

## Context
Early drafts introduced policy/projection/provisioning overlaps (e.g., PlatformPolicy vs WorkloadPlan). We aim to keep "Workload-only" UX while reducing CRDs and aligning with score-k8s (provisioners, generate→patch→apply).

## Decision
1) Keep `Workload` only for users
2) Remove `PlatformPolicy` CRD (policy = Orchestrator Config + Admission)
3) Keep internal: `ResourceClaim`, `WorkloadPlan`
4) Rename `ResourceBinding` → `ResourceClaim` for clarity (claiming dependencies vs binding execution)
5) External term = **provisioner** (resolver is internal wording)
6) **Workload profiles** = concept only (ConfigMap/OCI template bundles)
7) Default auto-selection; optional abstract hints via annotation
8) Orchestrator: select & build IR; Runtime: render/apply; single-writer status

## Rationale
- Reduce confusion (policy vs plan vs template)
- Align with score-k8s mental model
- Backend choice is encapsulated; users stay runtime-agnostic
- Fewer public CRDs; distribution via OCI/ConfigMap
- `ResourceClaim` better reflects intent: claiming/requesting dependencies rather than binding execution state

## Details

### CRD set (after this ADR)

* **Public (user-facing):**
  * `Workload` (unchanged responsibilities)

* **Internal:**
  * `ResourceClaim`
  * `WorkloadPlan`

*No new CRDs are added in this ADR; `PlatformPolicy` is removed.*

### Orchestrator configuration (non-CRD)

A single **orchestrator config** (ConfigMap or OCI document) becomes the source of truth for platform mapping:

* `profiles[]` — abstract profile entries (e.g., `web-service`, `batch-job`, `event-consumer`, `function`), each mapping to one or more **backend candidates**:
  * `backendId` (stable identifier, not user-visible)
  * `runtimeClass` (e.g., `kubernetes`, `ecs`, `nomad`, …)
  * `template` { `kind`: `manifests` | `helm` | `…`, `ref`: immutable reference (OCI digest recommended) }
  * `priority` / `version` / `constraints` (selectors by namespace/labels/region; feature requirements like `scale-to-zero`)

> **Templates are declarative** (no embedded functions); rendering is fully determined by `values`.
* `provisioners[]` — dependency realization definitions aligned with Score resources.
* `defaults` — profile defaults by selector (namespace/labels), plus a global default.

### Selection pipeline (deterministic)

1. **Collect candidates** from the chosen abstract profile (either auto‑derived or hinted by user).
2. **Filter** by environment selectors and feature requirements.
3. **Admission** (VAP/OPA/Kyverno) enforces allowed profiles/requirements.
4. **Pick one** using a stable order: `priority` → `version` → `name`.
5. If none, set abstract conditions: `RuntimeReady=False` with reason `RuntimeSelecting` or `PolicyViolation`.

### Provision → Projection → Render

* **Provision (provisioners)**: For each declared `resources` in Workload, create/resolve `ResourceClaim` and wait for `outputs`.
* **Projection/Plan**: Orchestrator builds an IR (`WorkloadPlan`) containing:
  * selected `runtimeClass` and `backendId`
  * `template` reference and `kind`
  * `values` = **`defaults ⊕ normalize(Workload) ⊕ outputs`**
    _(right-hand wins; i.e., `outputs` override `Workload`, which override `defaults`)_
* **Render & Apply (Runtime Controller)**: fetch `template`, render with `values`, materialize. Runtime writes detailed diagnostics to its own internal objects; Orchestrator translates to abstract `Workload.status.conditions`.

### User experience

* Users author only **`Workload`**.
* Optional **abstract hints** via annotations (e.g., `score.dev/profile: web-service`, `score.dev/requirements: ["scale-to-zero"]`).
* No runtime-specific nouns in user input or user-facing status.

### Status model (unchanged)

* `Workload.status.endpoint` (single canonical URI or null)
* `conditions[]` with abstract types/reasons per ADR‑0001
* `bindings[]` summaries
* **Readiness rule** unchanged: `InputsValid=True AND ClaimsReady=True AND RuntimeReady=True`.

### Architecture overview

```
+-------------------+           +-------------------------+          +-----------------------+
|       User        |           |      Orchestrator       |          |   Runtime Controller  |
|  (authors only    |  Workload | - reads Orchestrator    |  Plan/IR | - watches Plan/IR     |
|   Workload)       +---------->+   Config & Admission    +--------->+ - fetches template    |
+-------------------+           | - selects Profile/Backend|          | - render & apply      |
                                | - creates ResourceClaim  |          | - diagnostics (internal)
                                | - builds WorkloadPlan    |          +-----------+-----------+
                                | - updates Workload.status|                      |
                                +-----------+--------------+                      |
                                            |                                 Applied objects
                                            |  ResourceClaim                (runtime‑specific)
                             +--------------v--------------+                         |
                             |        Provisioner          |<------------------------+
                             | (create or bind dependency) |
                             |    -> status.outputs        |
                             +-----------------------------+
```

**Flow**: User submits `Workload` → Orchestrator selects profile/backend and provisions dependencies → Orchestrator builds projection IR (`WorkloadPlan`) → Runtime Controller renders templates and materializes → Provisioners supply dependency outputs → Orchestrator maintains abstract user-facing status.

---

## Consequences

**Positive**

* Fewer CRDs, clearer boundaries (input vs. request vs. projection).
* Platform flexibility: multiple backends per profile; deterministic selection; gradual migration via pinned template refs.
* Terminology aligned with `score-k8s` (provisioners), reducing learning cost.

**Trade-offs / Risks**

* Operators manage a non-CRD config artifact (ConfigMap/OCI) and Admission rules—requires good tooling and GitOps hygiene.
* No immediate API rename may preserve some legacy terms (Binding/Plan) in code; docs use Claim/Projection terminology.

---

## Alternatives considered

1. **Add `WorkloadProfile` and `ResourceClass` as CRDs**
   * Pro: strong API contracts, RBAC‑friendly.
   * Con: increases CRD surface and the policy/plan/template confusion; heavier upgrades.
   * **Rejected** for simplicity and faster convergence.

2. **Keep `PlatformPolicy` CRD**
   * Pro: central policy object.
   * Con: overlapped with projection/templates; caused confusion with `WorkloadPlan`.
   * **Rejected** to clarify separation of concerns.

3. **Let Orchestrator embed templates**
   * Con: couples code and distribution; limits operator control and canarying.
   * **Rejected** in favor of externalized templates (OCI/Git/ConfigMap).

---

<!-- Proposal phase: migration/follow-ups are out of scope in this ADR. -->

---

## References

* score-spec / score-k8s discussion threads (re: provisioners, patch templates, generate flow).
* ADR‑0001 and ADR‑0002 in this repository for status vocabulary and controller split.

