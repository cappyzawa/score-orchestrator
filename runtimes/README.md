# Runtime Controllers

This directory contains **independent Runtime Controller implementations** that are completely separate from the main Score Orchestrator.

## Architecture

Per [ADR-0003](../docs/ADR/ADR-0003-architecture-simplification.md), Runtime Controllers are independent processes that:

1. **Watch WorkloadPlan** resources created by the Orchestrator
2. **Materialize runtime-specific resources** (Deployments, Services, etc.)
3. **Run as separate processes** from the Orchestrator

```
[User] → [Workload] → [Orchestrator] → [WorkloadPlan] → [Runtime Controller] → [Runtime Resources]
                          ↑                                       ↓
                    [OrchestratorConfig]                 [Kubernetes/ECS/Nomad]
```

## Available Runtime Controllers

### Kubernetes Runtime Controller

- **Path**: `kubernetes/`
- **Watches**: WorkloadPlan with `runtimeClass: kubernetes`
- **Creates**: Kubernetes Deployments, Services, ConfigMaps
- **Status**: ✅ **Reference Implementation Complete**

```bash
# Build and deploy
cd kubernetes/
make build
make docker-build
make deploy

# Or from root
make build-runtime
make deploy-runtime
```

## Adding New Runtime Controllers

To add support for new runtime platforms (ECS, Nomad, etc.):

1. **Create directory structure**:
   ```
   runtimes/ecs/
   ├── cmd/main.go
   ├── internal/controller/
   ├── manifests/
   ├── Dockerfile
   ├── Makefile
   └── README.md
   ```

2. **Implement WorkloadPlan watcher** with appropriate `runtimeClass` filter
3. **Add to root Makefile** with delegation targets
4. **Follow same patterns** as the Kubernetes implementation

## Design Principles

- **Complete encapsulation** - Each runtime controller is self-contained
- **Independent scaling** - Different runtime controllers scale independently  
- **Runtime-agnostic core** - Orchestrator remains platform-neutral
- **Standard interfaces** - All runtime controllers use WorkloadPlan/ResourceClaim APIs

## Related Documentation

- [ADR-0003: Control-plane simplification](../docs/ADR/ADR-0003-architecture-simplification.md)
- [WorkloadPlan API](../api/v1b1/workloadplan_types.go)
- [Main Project README](../README.md)