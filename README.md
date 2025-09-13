# Score Orchestrator (independent reference project)

> **Non-affiliation:** This repository is an independent reference project and is **not** affiliated with the **Score Official** (Score Spec Maintainers). It aims to be Score-compatible while keeping the user experience runtime-agnostic.

## Why this exists
Score defines a portable way to describe applications. This project provides a small, opinionated control plane so that **users author only a `Workload`**, while platform/runtime details and dependencies are handled behind the scenes.

## What this project is
- A reference **orchestrator** built around Kubernetes CRDs (group/version: `score.dev/v1b1`)
- Public API surface: **`Workload`** only
- Internal contracts for platform use: `ResourceClaim`, `WorkloadPlan`
- Status is abstract and user-centric (a single `endpoint`, abstract `conditions`, binding summaries)
- Validation boundary: **CRD OpenAPI + CEL** for spec invariants; org policy via **VAP/OPA/Kyverno**

## Design tenets (at a glance)
- **Runtime-agnostic UX** — no runtime-specific nouns in user-visible docs
- **Single-writer status** — only the orchestrator updates `Workload.status`
- **Separation of concerns** — plan vs. bindings; users vs. platform
- **RBAC by default** — users see only `Workload`

## Documentation
- Spec & APIs: [`docs/spec/crds.md`](docs/spec/crds.md)
- Controllers & responsibilities: [`docs/spec/control-plane.md`](docs/spec/control-plane.md)
- Lifecycle & state: [`docs/spec/lifecycle.md`](docs/spec/lifecycle.md)
- Orchestrator Config (profiles/selection/defaults): [`docs/spec/orchestrator-config.md`](docs/spec/orchestrator-config.md)
- Validation strategy (OpenAPI + CEL): [`docs/spec/validation.md`](docs/spec/validation.md)
- RBAC recommendations: [`docs/spec/rbac.md`](docs/spec/rbac.md)
- ADR index: [`docs/ADR/`](docs/ADR/)

## Discussion & background
- Score spec discussion: https://github.com/score-spec/spec/discussions/157

## Contributing
See [`CONTRIBUTING.md`](CONTRIBUTING.md). PRs that improve docs, tests, and conformance are welcome.