# Endpoint Mirror Specification

This document defines the **runtime-only mirror** model for endpoint resolution in the Score Orchestrator, as per ADR-0007.

## Overview

The Orchestrator **mirrors** Runtime-published endpoints into `Workload.status.endpoint` without any observation-based derivation. This ensures accuracy, governance, and simplicity while maintaining the single-writer principle.

## Contract

### WorkloadExposure Resource

The `WorkloadExposure` resource serves as the coordination mechanism between Runtime Controllers and the Orchestrator:

**Spec (written by Orchestrator):**
- `workloadRef`: Reference to the target Workload
- `runtimeClass`: Selected runtime (e.g., `kubernetes`, `ecs`, `nomad`)
- `observedWorkloadGeneration`: For causality tracking

**Status (written only by Runtime Controllers):**
- `exposures[]`: Array of published endpoints (ordered by priority)
- `conditions[]`: Runtime-specific conditions

### Exposure Entry Schema

Each item in `status.exposures[]`:
```yaml
- url: "https://app.example.com"    # required: valid URI
  priority: 100                      # optional: higher wins (default: 0)
  scope: "Public"                   # optional: Public|ClusterLocal|VPC|Other
  schemeHint: "HTTPS"               # optional: HTTP|HTTPS|GRPC|TCP|OTHER
  reachable: true                   # optional: endpoint health status
```

## Mirror Process

### Step 1: WorkloadExposure Registration
The WorkloadExposureRegistrar Controller:
1. Watches `Workload` resources for creation/updates
2. Creates corresponding `WorkloadExposure` with populated spec
3. Sets OwnerReference to target Workload
4. Leaves status empty (Runtime Controller will populate)

### Step 2: Runtime Publication
Runtime Controllers:
1. Monitor platform-specific exposure resources (Ingress, LoadBalancer, etc.)
2. Determine canonical endpoint URLs
3. Update `WorkloadExposure.status.exposures[]` with discovered endpoints
4. Set appropriate conditions reflecting exposure status

### Step 3: Endpoint Mirroring
The ExposureMirror Controller:
1. Watches `WorkloadExposure.status` changes
2. Validates `exposures[0].url` as a well-formed URI
3. Mirrors the URL to `Workload.status.endpoint` (no modification)
4. Maps Runtime conditions to abstract reasons on `Workload.status.conditions`
5. Performs identity verification via `workloadRef.uid` to prevent stale updates

## Mirror Rules

### URI Selection
- **Priority**: Use `exposures[0]` (first in array after Runtime ordering)
- **Validation**: Ensure URL is a valid URI format
- **Generation Guard**: Ignore updates with stale `observedWorkloadGeneration`
- **Null Fallback**: If no valid exposure exists, `endpoint` remains `null`

### What is NOT Done
- **No Service observation**: Orchestrator does not read Services or Ingresses
- **No URL derivation**: No computation based on platform resources
- **No fallback logic**: No "best effort" endpoint generation
- **No template expansion**: URLs are mirrored as-is

### Condition Normalization

Runtime conditions are mapped to abstract reasons:
- `Healthy/Available` → `Succeeded`
- `Selecting/Provisioning/Applying` → `RuntimeProvisioning`
- `LBPending/IngressPending/DNSNotReady` → `NetworkUnavailable`
- `Degraded` → `RuntimeDegraded`
- Policy/Quota/Permission errors → existing abstract reasons

## Flap Resistance

To avoid unnecessary status updates:
- **Change detection**: Update `Workload.status.endpoint` only when value changes
- **Invalid URL handling**: Ignore malformed URLs; keep previous valid value
- **Generation tracking**: Skip updates from stale WorkloadExposure reports
- **Debouncing**: Avoid rapid succession updates within controller reconciliation

## Authority Model

### Controller Responsibilities
- **WorkloadExposureRegistrar**: Exclusive writer of `WorkloadExposure.spec`
- **ExposureMirror**: Co-writer of `Workload.status` (endpoint and conditions only)
- **Main Orchestrator**: Co-writer of `Workload.status` (claims, other conditions)
- **Runtime Controllers**: Exclusive writer of `WorkloadExposure.status`
- **Clear separation**: Each controller has distinct, non-overlapping write authority

### RBAC Requirements

**WorkloadExposureRegistrar permissions:**
- `workloads`: get, list, watch
- `workloadplans`: get, list, watch
- `workloadexposures`: get, list, watch, create, update, patch, delete
- `workloadexposures/status`: get, list, watch (read-only)

**ExposureMirror permissions:**
- `workloadexposures`: get, list, watch
- `workloadexposures/status`: get, list (read-only)
- `workloads`: get, list, watch
- `workloads/status`: get, list, update, patch (endpoint/conditions writer)

**Runtime Controller permissions:**
- `workloadexposures`: get, list, watch
- `workloadexposures/status`: get, list, update, patch (status writer)

**User permissions:**
- **No access** to `workloadexposures` (internal resource)

## Policy Integration

### Scope-based Filtering
Platforms may implement policies to filter non-public endpoints:
```yaml
# Example policy: hide non-public exposures
if exposure.scope != "Public":
  endpoint = null
```

### Template Override (Optional)
Advanced platforms may provide template-based endpoint overrides:
```yaml
# Platform config example
endpointTemplate: "https://{{.workload.name}}.{{.workload.namespace}}.example.com"
```

When configured as a **policy override**, the template explicitly replaces the Runtime-published URL. The default behavior remains **runtime-sourced**.

## Error Handling

### Runtime Controller Failures
- **Missing WorkloadExposure**: Runtime creates empty status, Orchestrator sees `endpoint: null`
- **Exposure detection failure**: Runtime sets appropriate condition, endpoint remains `null`
- **Invalid URL publication**: Orchestrator ignores invalid URLs, logs warning

### Orchestrator Failures
- **WorkloadExposure creation failure**: Retry with exponential backoff
- **Status mirror failure**: Log error, maintain previous endpoint value
- **Watch connection loss**: Re-establish watch, reconcile on reconnection

## Observability

### User-Facing Status
```bash
# Check endpoint availability
kubectl get workload myapp -o jsonpath='{.status.endpoint}'

# Monitor exposure conditions
kubectl describe workload myapp
```

### Platform-Facing Debug
```bash
# Check Runtime-published exposures
kubectl get workloadexposure myapp -o yaml

# Monitor exposure conditions
kubectl describe workloadexposure myapp
```

### Metrics and Events
- **Endpoint mirror events**: Record successful/failed mirror operations
- **Exposure publication latency**: Time from platform resource to endpoint availability
- **Invalid URL rejections**: Count and log malformed URLs from Runtime

## Compatibility

### Upgrade Path
Existing endpoint derivation logic is removed. During upgrade:
1. WorkloadExposure resources are created for existing Workloads
2. Runtime Controllers begin publishing exposures
3. Users may see `endpoint: null` until Runtime Controllers are updated
4. Old derivation code paths are disabled

### Rollback Considerations
- WorkloadExposure resources persist during rollback
- Previous endpoint values are preserved in Workload status
- Runtime Controllers gracefully handle missing WorkloadExposure resources

This specification ensures that endpoint resolution is authoritative, governance-friendly, and maintains clear separation of concerns between Orchestrator and Runtime Controllers.