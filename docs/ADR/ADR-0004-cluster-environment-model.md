# ADR-0004 — One Cluster, One Environment Model

**Status:** Accepted
**Date:** 2025-09-15 (JST)
**Discussion:** https://github.com/cappyzawa/score-orchestrator/issues/26
**Related ADRs:** ADR-0003 (Architecture Simplification)

---

## Context

During E2E testing of the Kubernetes Runtime Controller (#16), we discovered that backend selection consistently fails with "no suitable backend candidates found" errors. Investigation revealed that the current label-based environment selection model creates unnecessary complexity:

1. **Multi-layered label evaluation**: Current implementation combines Workload and Namespace labels, requiring both to have appropriate environment labels
2. **Runtime-specific concerns leaking into abstractions**: Environment selection forces users to understand namespace labeling strategies
3. **Operational complexity**: Same cluster hosting multiple environments requires careful namespace management and label coordination

Additionally, analysis of real-world deployment patterns shows that:
- Organizations typically maintain separate clusters per environment (dev, staging, prod)
- GitOps workflows naturally align with cluster-per-environment model
- Day-2 operations (kubectl apply, ArgoCD sync) target specific clusters

## Decision

We adopt a **One Cluster, One Environment** model with the following changes:

### 1. Cluster-Level Environment Configuration
- Environment is defined at the **cluster level** in the OrchestratorConfig
- No environment-based backend selection within a single cluster
- Each cluster has exactly one environment context

### 2. Simplified Backend Selection
- Remove environment label requirements from Workload resources
- Backend selection based solely on:
  - **Profile** (auto-derived from workload characteristics: web-service, batch-job, etc.)
  - **Feature requirements** (http-ingress, monitoring, scale-to-zero, etc.)
  - **Resource constraints** (CPU, memory, storage limits)

### 3. Workload Simplification
```yaml
# Before (complex)
apiVersion: score.dev/v1b1
kind: Workload
metadata:
  labels:
    environment: development  # Required for backend selection
    team: backend             # Organizational, but affects technical selection
spec: ...

# After (simple)
apiVersion: score.dev/v1b1
kind: Workload
metadata:
  name: my-app  # staging variants use different names (my-app-staging)
  # No environment labels required
spec: ...
```

### 4. Configuration Model
```yaml
# OrchestratorConfig defines cluster-wide environment
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: platform-config
spec:
  # Implicit: this entire cluster represents one environment
  profiles:
  - name: web-service
    backends:
    - backendId: k8s-web
      runtimeClass: kubernetes
      # No environment selectors needed
      constraints:
        features: ["http-ingress"]
        resources:
          cpu: "100m-2000m"
          memory: "128Mi-4Gi"
```

## Rationale

### Operational Benefits
- **Simplified deployment**: `kubectl apply` to dev cluster = development deployment
- **GitOps alignment**: Environment promotion = different cluster targets
- **Reduced configuration errors**: No cross-environment labeling mistakes
- **Cleaner RBAC**: Environment isolation through cluster boundaries

### User Experience Benefits
- **Cognitive load reduction**: Users don't reason about environment labels
- **Score abstraction preserved**: Focus on workload characteristics, not infrastructure concerns
- **Day-2 operations**: Natural fit with existing Kubernetes tooling

### Implementation Benefits
- **Deterministic backend selection**: No label combination edge cases
- **Simplified testing**: Each cluster configuration is self-contained
- **Debugging clarity**: Environment context is explicit at cluster level

## Consequences

### Positive
- **Issue #26 resolved**: Backend selection becomes deterministic
- **Reduced configuration surface**: Fewer required labels and selectors
- **Better separation of concerns**: Organizational (team) labels separate from technical selection
- **Improved testability**: Cluster-scoped configuration easier to validate

### Negative
- **Multi-environment clusters not supported**: Organizations using shared clusters need migration
- **Configuration duplication**: Similar backend configs across environment-specific clusters
- **Runtime controller complexity**: May need cluster-aware logic for cross-environment references

### Neutral
- **Migration required**: Existing multi-environment setups need refactoring
- **Documentation updates**: All examples and guides need revision

## Implementation Impact

### Immediate Changes Required
1. **Backend selection logic**: Remove environment-based filtering in `internal/selection/selector.go`
2. **Configuration samples**: Update `config/samples/orchestrator-config.yaml`
3. **Test cases**: Revise E2E tests to assume single-environment clusters
4. **Specification**: Update `docs/spec/orchestrator-config.md`

### Future Considerations
- **Cross-cluster dependencies**: How to handle references between environments
- **Configuration management**: Tooling for managing similar configs across clusters
- **Migration guide**: Supporting organizations transitioning from multi-environment clusters

## Alternatives Considered

### Alternative 1: Workload-Priority Label Selection
Keep namespace+workload label combination but prioritize workload labels. **Rejected** because it maintains unnecessary complexity and still leaks infrastructure concerns to users.

### Alternative 2: Flexible Environment Selectors
Allow more flexible environment matching (OR conditions, regex, etc.). **Rejected** because it increases configuration complexity without addressing the fundamental issue.

### Alternative 3: Profile-Only Selection
Remove environment concept entirely. **Rejected** because environments remain a valid organizational boundary, just not within clusters.

## References

- [Issue #26: Backend selection fails despite matching configuration](https://github.com/cappyzawa/score-orchestrator/issues/26)
- [Score Spec Discussion #157: Define a Kubernetes CRD for Score](https://github.com/score-spec/spec/discussions/157)
- [Orchestrator Configuration Specification](../spec/orchestrator-config.md)