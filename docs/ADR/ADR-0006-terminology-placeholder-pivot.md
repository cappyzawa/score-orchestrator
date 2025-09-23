# ADR-0006 — Terminology & Placeholder Handling Pivot

## Status
Accepted • Supersedes parts of ADR-0003 (without modifying ADR-0003 itself).

## Context
After implementing the initial design, we converged on two changes:
1) Rename **ResourceBinding** to **ResourceClaim** to better reflect the ownership and lifecycle.
2) Detect unresolved `${...}` placeholders **inside the Orchestrator before plan emission**. We no longer rely on CEL/admission for deep/opaque JSON scanning.

## Decision
- Documentation uses **ResourceClaim** across the board.
- The Orchestrator short-circuits plan emission when placeholders remain and surfaces
  `RuntimeReady=False` with `Reason=ProjectionError` on `Workload.status`.
- `WorkloadPlan` remains **internal and minimal** (projection contract only).

## Consequences
- Clearer user-facing failures; avoids leaking unresolved values to runtimes.
- Keeps public CRDs simple; validation boundaries stay: OpenAPI+CEL (spec invariants), VAP/OPA/Kyverno (org policy).
- Existing ADRs remain immutable as historical record.