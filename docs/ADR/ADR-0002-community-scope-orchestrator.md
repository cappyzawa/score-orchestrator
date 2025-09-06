# ADR-0002: Expand Project Scope (independent from Score Official) from CRDs-only to Include the Orchestrator

- Status: Accepted
- Date: 2025-09-06
- Discussed in: https://github.com/score-spec/spec/discussions/157
- Supersedes/Relates to: ADR-0001 (architecture overview)

## Context

The original intent in the discussion thread above was for the **Score Official** (Score Spec Maintainers) to publish **CRDs only**, and let platform operators (PF) implement their own controllers. During design, several issues surfaced:

- **Duplication of planning logic**: Without a shared planner, each runtime controller would need to re-implement resolution and projection logic. This increases drift risk and fragmentations across runtimes.
- **User-facing contract stability**: We require a single, abstract, user-facing status on `Workload` (`endpoint`, abstract `conditions`, compact binding summaries). Achieving a consistent **single-writer** model is hard if multiple controllers attempt to write `Workload.status`.
- **Portability and conformance**: Score semantics should be portable across runtimes. A project-owned Orchestrator centralizes policy application and planning, which is easier to test with conformance suites.
- **Validation boundaries**: CRD OpenAPI + CEL can enforce spec invariants, but **planning** (policy application, dependency graph, projection rules) is intentionally outside validation. A reference Orchestrator defines that behavior canonically.

## Decision

This **independent reference project** will maintain and release, in addition to CRDs (independent from **Score Official**):

- **Orchestrator (project-owned)**  
  - Interprets `Workload` against `PlatformPolicy`.  
  - Creates and monitors **`ResourceBinding`** (internal) for dependencies.  
  - Emits **`WorkloadPlan`** (internal) as a runtime-agnostic projection plan.  
  - Acts as the **single writer** of `Workload.status` (exposing only `endpoint`, abstract `conditions`, and binding summaries).

What remains **out of scope** for this project:

- **Admission**: Organization-specific. Validation beyond spec invariants should be done via ValidatingAdmissionPolicy / OPA Gatekeeper / Kyverno.
- **Production Resolvers and Runtime Controllers**: Implemented and operated by PFs and vendors. This project may provide simple **sample/reference** resolvers only.

## Consequences

- **Pros**
  - Canonical planning logic; reduced duplication across runtimes.
  - Stronger portability guarantees and simpler conformance testing.
  - Clear responsibility: Orchestrator is the only writer of `Workload.status`.
- **Cons**
  - Additional project maintenance burden (release, security, versioning).
  - A new internal CRD (`WorkloadPlan`) appears in the API surface (hidden from users via RBAC).

## Alternatives Considered

1. **CRDs-only (original plan by Score Official)**  
   - Rejected: planning logic would be re-implemented by each runtime controller, risking divergence and inconsistent user status.
2. **Runtime-specific orchestrators (vendor-owned)**  
   - Rejected: undermines portability; users might see subtly different behavior per runtime.
3. **Embed plan inside `Workload.status`**  
   - Rejected: leaks internal details to users; complicates status ownership and increases write conflicts. The plan is now an internal CRD (`WorkloadPlan`).
4. **Officially managed Admission**  
   - Rejected: org policies vary widely; better separated. This project sticks to CRD OpenAPI + CEL for spec-level invariants while org policies live in platform admission.

## Implementation Notes

- **API Group/Version**: `score.dev/v1b1`
- **Kinds**
  - Public: `Workload` (user-facing)
  - PF-facing (hidden to users): `PlatformPolicy`
  - Internal: `ResourceBinding`, `WorkloadPlan`
- **RBAC**: Users only see `Workload`. `PlatformPolicy`, `ResourceBinding`, and `WorkloadPlan` are hidden from users.
- **Conformance**: Provide tests to verify the single-writer model and the abstract reason vocabulary for conditions.

## Rollback Plan

If maintaining the Orchestrator proves unsustainable, we can:
- Freeze the Orchestrator as a “reference implementation” and formalize the planning contract in a publicly versioned spec so vendors can re-implement it consistently.
- Keep `WorkloadPlan` as the authoritative contract to preserve portability.
