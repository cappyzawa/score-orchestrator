# Kubernetes Runtime Controller

Reference implementation of a **Runtime Controller** for the Score Orchestrator that materializes Kubernetes resources.

## Overview

This controller watches `WorkloadPlan` resources and creates corresponding Kubernetes `Deployment` and `Service` resources, enabling the "Workload-only UX" vision from [ADR-0003](../../docs/ADR/ADR-0003-architecture-simplification.md).

## Architecture

```
WorkloadPlan (runtimeClass: kubernetes) 
           ↓
   Kubernetes Runtime Controller
           ↓
   Deployment + Service + ConfigMap
```

### Responsibilities

- ✅ **Watch WorkloadPlan**: Only processes plans with `runtimeClass: kubernetes`
- ✅ **Create Deployments**: From `WorkloadPlan.spec.values` and referenced `Workload`
- ✅ **Create Services**: When `Workload.spec.service.ports` are defined
- ✅ **Resource ownership**: Sets OwnerReference for garbage collection
- ✅ **Independent process**: Runs separately from the Orchestrator

### What it does NOT do

- ❌ **Modify Workload.status** - Only the Orchestrator updates Workload status
- ❌ **Create WorkloadPlan** - Created by the Orchestrator
- ❌ **Handle other runtimeClass** - Kubernetes-specific implementation

## Quick Start

### Prerequisites

- Kubernetes cluster with Score Orchestrator CRDs installed
- Score Orchestrator running and creating WorkloadPlan resources

### Build and Deploy

```bash
# Build binary
make build

# Build and deploy to cluster
make docker-build
make deploy

# View logs
make logs

# Clean up
make undeploy
```

### From Root Project

```bash
# From the root of score-orchestrator project
make build-runtime
make deploy-runtime
```

## Configuration

### Environment Variables

- `RUNTIME_CLASS`: Should be "kubernetes" (default behavior)
- Standard controller-runtime flags available

### RBAC Requirements

The controller requires these permissions (automatically configured):

- `workloadplans`: get, list, watch
- `workloads`: get, list, watch  
- `deployments`: get, list, watch, create, update, patch, delete
- `services`: get, list, watch, create, update, patch, delete
- `configmaps`: get, list, watch, create, update, patch, delete
- `events`: create, patch

## Example Usage

1. **User creates Workload**:
   ```yaml
   apiVersion: score.dev/v1b1
   kind: Workload
   metadata:
     name: myapp
   spec:
     containers:
       app:
         image: nginx:latest
     service:
       ports:
       - port: 8080
   ```

2. **Orchestrator creates WorkloadPlan** (with `runtimeClass: kubernetes`)

3. **This controller automatically creates**:
   - `Deployment/myapp`
   - `Service/myapp`

## Development

### Project Structure

```
kubernetes/
├── cmd/main.go                           # Controller entrypoint
├── internal/controller/
│   └── kubernetes_controller.go          # Main reconciler logic
├── manifests/                            # Kubernetes manifests
│   ├── rbac.yaml                        # ServiceAccount, ClusterRole
│   ├── deployment.yaml                  # Controller deployment
│   └── kustomization.yaml               # Kustomize config
├── Dockerfile                           # Container build
├── Makefile                            # Build/deploy targets
└── README.md                           # This file
```

### Key Components

- **KubernetesRuntimeReconciler**: Main controller that watches WorkloadPlan
- **buildDeployment()**: Converts WorkloadPlan → Kubernetes Deployment
- **buildService()**: Converts WorkloadPlan → Kubernetes Service
- **Resource ownership**: Ensures proper garbage collection

## Testing

```bash
# Run unit tests
make test

# Manual testing
make run  # Runs controller locally against configured cluster
```

## Customization

This is a **reference implementation**. Organizations can:

- Fork and customize resource generation logic
- Add support for Ingress, NetworkPolicy, etc.
- Implement organization-specific naming conventions
- Add validation/policy enforcement

## Related Documentation

- [ADR-0003: Architecture](../../docs/ADR/ADR-0003-architecture-simplification.md)
- [WorkloadPlan API](../../api/v1b1/workloadplan_types.go)
- [Score Specification](https://score.dev)
- [Runtime Controllers Overview](../README.md)