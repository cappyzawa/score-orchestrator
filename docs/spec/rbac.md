# RBAC (Role-Based Access Control) Specification

This document defines the access control model for Score Orchestrator, establishing clear permission boundaries between users, controllers, and platform components while maintaining security isolation and the single-writer principle.

## RBAC Philosophy

Score Orchestrator implements **layered access control** with strict separation of concerns:

- **Users** interact only with public APIs (`Workload`, `PlatformPolicy`)
- **Controllers** have minimal required permissions for their specific responsibilities  
- **Internal APIs** remain invisible to users while being accessible to platform components
- **Single-writer principle** prevents status corruption through exclusive write permissions

## Access Control Matrix

| Actor / Resource         | Workload (public) | PlatformPolicy (PF) | ResourceBinding (internal) | WorkloadPlan (internal) |
|--------------------------|-------------------|---------------------|----------------------------|-------------------------|
| Users (tenants)          | get/list/watch/create/update | **no access**       | **no access**               | **no access**            |
| Orchestrator             | full              | read                | read/write (status)         | full (single writer)     |
| Runtime Controller(s)    | read              | read (optional)     | read                        | read                     |
| Resolver(s)              | read              | read (optional)     | read/write (status)         | none                     |

## User Permissions

### Application Developers

**Permitted Actions:**
- Create, read, update, delete `Workload` resources in authorized namespaces
- Read `Workload.status` for monitoring and debugging
- Read `PlatformPolicy` resources to understand applied constraints
- Read `Workload` events for troubleshooting

**Prohibited Actions:**
- Direct access to `ResourceBinding` or `WorkloadPlan` resources
- Modification of `Workload.status` fields
- Creation or modification of internal orchestration resources
- Access to platform-specific runtime resources

**Sample ClusterRole:**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: score-user
rules:
# Workload management
- apiGroups: ["score.dev"]
  resources: ["workloads"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: ["score.dev"]  
  resources: ["workloads/status"]
  verbs: ["get", "list"]
# Policy visibility
- apiGroups: ["score.dev"]
  resources: ["platformpolicies"]
  verbs: ["get", "list"]
# Event access for debugging
- apiGroups: [""]
  resources: ["events"]
  verbs: ["get", "list"]
```

### Platform Administrators  

**Additional Permissions:**
- Create, update, delete `PlatformPolicy` resources
- Read internal resources for debugging and monitoring
- Manage controller service accounts and permissions
- Access platform-specific runtime resources for troubleshooting

**Sample ClusterRole:**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole  
metadata:
  name: score-platform-admin
rules:
# All user permissions
- apiGroups: ["score.dev"]
  resources: ["workloads", "platformpolicies"]
  verbs: ["*"]
- apiGroups: ["score.dev"]
  resources: ["workloads/status"]
  verbs: ["get", "list"]
# Internal resource visibility for debugging
- apiGroups: ["score.dev"]
  resources: ["resourcebindings", "workloadplans"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["score.dev"]
  resources: ["resourcebindings/status", "workloadplans/status"]
  verbs: ["get", "list"]
```

## Controller Permissions

### Orchestrator Controller

The Orchestrator Controller requires comprehensive permissions as the central coordination component.

**Core Responsibilities:**
- **Exclusive writer** of `Workload.status`
- Creator and manager of `ResourceBinding` and `WorkloadPlan` resources
- Reader of `PlatformPolicy` for governance application
- Event publisher for audit and debugging

**Required ClusterRole:**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: score-orchestrator
rules:
# Workload management (read spec, write status exclusively)
- apiGroups: ["score.dev"]
  resources: ["workloads"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["score.dev"]
  resources: ["workloads/status"]
  verbs: ["get", "list", "update", "patch"]
# Policy consumption
- apiGroups: ["score.dev"]
  resources: ["platformpolicies"]
  verbs: ["get", "list", "watch"]
# Internal resource management
- apiGroups: ["score.dev"]
  resources: ["resourcebindings", "workloadplans"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["score.dev"]
  resources: ["resourcebindings/status"]
  verbs: ["get", "list", "watch"]
# Event publishing
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

**Security Constraints:**
- **Cannot** create or modify `Workload` resources (prevents unauthorized workload injection)
- **Cannot** write to `ResourceBinding.status` (prevents binding state corruption)
- **Must** verify OwnerReference before creating internal resources
- **Should** implement leader election for high availability

### Runtime Controllers

Runtime Controllers consume execution plans and materialize workloads on target platforms.

**Core Responsibilities:**
- Read `WorkloadPlan` resources for execution instructions
- Read `ResourceBinding.status.outputs` for dependency information
- Write `WorkloadPlan.status` to report materialization progress
- Manage platform-specific resources (Kubernetes Deployments, ECS Tasks, etc.)

**Required ClusterRole:**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: score-runtime-controller
rules:
# Plan consumption
- apiGroups: ["score.dev"]
  resources: ["workloadplans"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["score.dev"]
  resources: ["workloadplans/status"]
  verbs: ["get", "list", "update", "patch"]
# Dependency output consumption
- apiGroups: ["score.dev"]
  resources: ["resourcebindings"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["score.dev"]
  resources: ["resourcebindings/status"]
  verbs: ["get", "list"]
# Platform-specific resource management (example: Kubernetes)
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["services", "configmaps", "secrets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses", "networkpolicies"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
# Event publishing
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

**Security Constraints:**
- **Cannot** access `Workload` resources directly (prevents bypassing orchestration)
- **Cannot** modify `ResourceBinding` resources (prevents binding manipulation)
- **Must** validate OwnerReference on `WorkloadPlan` before processing
- **Should** implement resource quotas and limits for platform resources

### Resolver Controllers

Resolver Controllers manage specific resource types and provide standardized outputs.

**Core Responsibilities:**
- Claim and bind `ResourceBinding` resources of supported types
- **Exclusive writer** of `ResourceBinding.status` for owned bindings
- Provision underlying resources (databases, message queues, storage, etc.)
- Populate standardized outputs for consumption by Runtime Controllers

**Required ClusterRole (Generic Resolver):**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: score-resolver-controller
rules:
# ResourceBinding management
- apiGroups: ["score.dev"]
  resources: ["resourcebindings"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["score.dev"]
  resources: ["resourcebindings/status"]
  verbs: ["get", "list", "update", "patch"]
# Provider-specific resources (example: Secret/ConfigMap resolver)
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
# Event publishing
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

**Specialized Resolver Permissions:**

*Database Resolver Example:*
```yaml
# Additional permissions for database provisioning
- apiGroups: ["postgresql.cnpg.io"]
  resources: ["clusters", "backups"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

*Cloud Resource Resolver Example:*
```yaml  
# Additional permissions for cloud resource management
- apiGroups: [""]
  resources: ["persistentvolumes", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## Security Boundaries and Isolation

### Namespace vs Cluster Scope

**Namespace-Scoped Resources:**
- `Workload`: Deployed in application namespaces
- `ResourceBinding`: Co-located with owning Workload  
- `WorkloadPlan`: Co-located with owning Workload
- Platform-specific resources: Follow namespace of parent Workload

**Cluster-Scoped Resources:**
- `PlatformPolicy`: Applied cluster-wide with namespace targeting
- Controller ServiceAccounts: Cluster-scoped for cross-namespace operations

### Resource Ownership and Lifecycle

**OwnerReference Enforcement:**
- `ResourceBinding` resources **MUST** have OwnerReference to parent `Workload`
- `WorkloadPlan` resources **MUST** have OwnerReference to parent `Workload`
- Platform-specific resources **SHOULD** have OwnerReference chain to `Workload`

**Garbage Collection:**
- Deleting `Workload` triggers cascading deletion of all owned resources
- Controllers **MUST** verify OwnerReference before processing resources
- Orphaned resources are automatically cleaned up by Kubernetes GC

### Cross-Namespace Access Patterns

**Prohibited Cross-Namespace Access:**
- Users cannot access Workloads in unauthorized namespaces
- Controllers cannot modify resources outside OwnerReference chain
- ResourceBindings cannot reference resources in different namespaces

**Permitted Cross-Namespace Access:**
- PlatformPolicy can target multiple namespaces via selectors
- Cluster-scoped resolver resources (e.g., StorageClasses) are accessible
- Controllers can read cluster-scoped resources for configuration

## Audit and Compliance

### Audit Event Categories

**User Actions:**
- `Workload` creation, modification, deletion
- `PlatformPolicy` management (admin users)
- Status queries and monitoring

**Controller Actions:**
- `ResourceBinding` lifecycle management
- `WorkloadPlan` generation and updates
- Status aggregation and endpoint reflection
- Platform resource materialization

### Compliance Requirements

**Single Writer Auditing:**
- All `Workload.status` modifications **MUST** be attributed to Orchestrator Controller
- Any status writes from other actors **MUST** trigger security alerts
- Status update conflicts **MUST** be logged and investigated

**Resource Ownership Auditing:**
- Creation of resources without proper OwnerReference **MUST** be flagged
- Cross-namespace resource access **MUST** be logged and monitored
- Orphaned resource detection **MUST** trigger cleanup workflows

### Security Monitoring

**Controller Health:**
- Monitor controller service account permissions for privilege escalation
- Track controller resource access patterns for anomaly detection
- Alert on controller failures or extended reconciliation loops

**Resource Isolation:**
- Monitor cross-namespace access attempts
- Track resource modification patterns for unauthorized changes
- Validate OwnerReference integrity across resource hierarchies

## Implementation Guidelines

### ServiceAccount Configuration

**Orchestrator Controller:**
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: score-orchestrator
  namespace: score-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: score-orchestrator
subjects:
- kind: ServiceAccount
  name: score-orchestrator
  namespace: score-system
roleRef:
  kind: ClusterRole
  name: score-orchestrator
  apiGroup: rbac.authorization.k8s.io
```

### Permission Testing

Controllers **MUST** implement permission validation:

```go
// Example permission test
func (r *OrchestratorReconciler) validatePermissions(ctx context.Context) error {
    // Test Workload status write permission
    if err := r.testWorkloadStatusUpdate(ctx); err != nil {
        return fmt.Errorf("missing Workload status write permission: %w", err)
    }
    
    // Test ResourceBinding creation permission
    if err := r.testResourceBindingCreation(ctx); err != nil {
        return fmt.Errorf("missing ResourceBinding creation permission: %w", err)
    }
    
    return nil
}
```

*Single-writer principle:* only the Orchestrator updates `Workload.status`. This prevents conflicting writes and keeps the user-facing contract stable.

This RBAC model ensures secure, isolated operation while maintaining the abstraction boundaries essential to Score Orchestrator's architecture.