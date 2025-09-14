
Resolves: #16 

Add independent Kubernetes Runtime Controller as reference
implementation for GitHub issue https://github.com/cappyzawa/score-orchestrator/issues/16.

This enables the runtime-agnostic architecture from ADR-0003
where WorkloadPlan resources are materialized into
platform-specific resources by independent controller processes.

The implementation demonstrates the pattern for extending
score-orchestrator to additional runtime platforms beyond
Kubernetes.

## Architecture

```
Workload → Orchestrator → WorkloadPlan → Runtime Controller → Kubernetes Resources
```

The controller successfully materializes Kubernetes Deployments and Services from WorkloadPlan specs, demonstrating the reference pattern for extending to additional runtime platforms.
