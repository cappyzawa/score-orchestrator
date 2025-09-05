# Score Orchestrator

Score Orchestrator provides a Kubernetes-based control-plane (via CRDs) and an orchestration system that keeps the user experience runtime-agnostic for managing Score workload specifications across diverse runtime platforms (Kubernetes, ECS, Nomad, etc.).

## Philosophy

**Users only write `Workload`**. The underlying execution platform, resource resolution, and infrastructure details are completely abstracted away and managed by the Platform (PF) side.

This approach enables:
- **Platform Independence**: Write once, run anywhere (Kubernetes, ECS, Nomad, etc.)
- **Developer Focus**: Focus on application requirements, not infrastructure details
- **Platform Control**: Platforms maintain full control over resource provisioning and policies

## Visibility & RBAC (Public vs Internal)

**API Group/Version:** `score.dev/v1b1`

**Public resources**
- `Workload` — the only user-facing CR.

**PF-facing (hidden from users via RBAC)**
- `PlatformPolicy` — cluster-scoped, managed by platform operators.

**Internal resources (not user-visible)**
- `ResourceBinding` — contract with resolvers for dependency provisioning.
- `WorkloadPlan` — contract from Orchestrator to Runtime.

Only the Orchestrator writes `Workload.status`. Runtimes and Resolvers update their own resources and the Orchestrator aggregates.

## User-facing Status Contract

Users should rely on `Workload.status` only:

- `endpoint: string|null` — the canonical URL if available (at most one; `null` if unknown).
- `conditions[]` — Kubernetes-style entries with **abstract reasons only** (no runtime-specific nouns).
  - Types: `Ready`, `BindingsReady`, `RuntimeReady`, `InputsValid`
  - Reasons (fixed vocabulary): `Succeeded`, `SpecInvalid`, `PolicyViolation`, `BindingPending`, `BindingFailed`, `ProjectionError`, `RuntimeSelecting`, `RuntimeProvisioning`, `RuntimeDegraded`, `QuotaExceeded`, `PermissionDenied`, `NetworkUnavailable`
- `bindings[]` — compact summaries per dependency: `key`, `phase (Pending|Binding|Bound|Failed)`, `reason`, `message`, `outputsAvailable: bool`.

**Readiness rule:** `InputsValid=True AND BindingsReady=True AND RuntimeReady=True`.

## Architecture Overview

Score Orchestrator implements a layered architecture with clear separation of concerns:

### Core Components

1. **Orchestrator Controller** (Community-provided)
   - Interprets `Workload` and `PlatformPolicy` resources
   - Generates and monitors required `ResourceBinding` resources
   - Produces `WorkloadPlan` for runtime execution
   - **Single writer** of `Workload.status` providing unified state aggregation

2. **Runtime Controller** (Platform-provided)
   - Consumes `WorkloadPlan` and `ResourceBinding.status.outputs`
   - Materializes workloads on concrete execution platforms
   - Manages platform-specific internal objects (invisible to users)

3. **Resolver Controllers** (Platform/Vendor-provided)
   - Bind `ResourceBinding` resources to `Bound` state
   - Provide standardized `outputs` for consumption

### Custom Resource Definitions

#### Public APIs (User-facing)
- **`Workload`** (`score.dev/v1b1`): The only resource users interact with

#### PF-facing (hidden from users via RBAC)
- **`PlatformPolicy`** (`score.dev/v1b1`): Platform-applied governance and defaults

#### Internal APIs (Platform-facing)
- **`ResourceBinding`** (`score.dev/v1b1`): Dependency resolution contracts
- **`WorkloadPlan`** (`score.dev/v1b1`): Orchestrator-to-Runtime execution plans

## High-Level Flow

```
1. User applies Workload
2. Orchestrator applies matching PlatformPolicy
3. Orchestrator generates ResourceBinding resources
4. Resolver Controllers bind dependencies and provide outputs
5. Orchestrator generates WorkloadPlan with projection rules
6. Runtime Controller materializes workload on target platform
7. Orchestrator aggregates status and exposes endpoint in Workload.status
```

## User Interface

Users interact exclusively with `Workload` resources. The status provides minimal, abstracted information:

- **`status.endpoint`**: Single exposed service endpoint (URI format)
- **`status.conditions`**: Standard Kubernetes conditions with abstract reasoning
- **`status.bindings`**: Summary of resource dependency states

All status messages use platform-neutral terminology - no Kubernetes-specific or platform-specific details are exposed.

## Validation Strategy

- **CRD OpenAPI + CEL**: Enforces specification-level invariants (community-provided)
- **Organization Policies**: Delegated to platform-specific tools (VAP/OPA/Kyverno)

## Non-goals (for clarity)

- Exposing runtime-specific details (e.g., Kubernetes Deployment/Pod names, ECS Task definitions) to users.
- Shipping organization-specific admission policies. (Spec-level invariants are enforced via CRD OpenAPI + CEL; org-specific policy is left to VAP/OPA/Kyverno.)
- Embedding the "plan" into `Workload.status`. The plan lives in the internal `WorkloadPlan` resource.

## Getting Started

See [CONTRIBUTING.md](./CONTRIBUTING.md) for development guidelines and [docs/](./docs/) for detailed specifications.

See also: [`docs/spec/crds.md`](docs/spec/crds.md), [`docs/spec/validation.md`](docs/spec/validation.md), [`docs/spec/lifecycle.md`](docs/spec/lifecycle.md), [`docs/spec/rbac.md`](docs/spec/rbac.md).