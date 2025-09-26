# Getting Started with Score Orchestrator

This guide walks you through using the Score Orchestrator to deploy and manage workloads, mirroring the learning flow of the [official Score tutorial](https://docs.score.dev/docs/get-started/score-compose-hello-world/) but demonstrating the orchestrator's unique capabilities around **status observation** and **dependency binding**.

You'll learn how the orchestrator manages the **Workload → ResourceClaim → WorkloadPlan** relationships by observing status changes rather than diving into implementation details.

## Prerequisites & Installation

### Required Tools

- `kubectl` (any recent version)
- `kind` (or any Kubernetes cluster)
- `make` (for building and deploying)
- `git` (to clone this repository)

### Install Score Orchestrator

1. **Clone the repository**:
   ```bash
   git clone https://github.com/cappyzawa/score-orchestrator
   cd score-orchestrator
   ```

2. **Install CRDs into your cluster**:
   ```bash
   make install
   ```

3. **Build and load controller images**:
   ```bash
   # Build the orchestrator controller image with correct name
   make docker-build IMG=example.com/kbinit:latest

   # Load image into Kind cluster (adjust cluster name if different)
   kind load docker-image example.com/kbinit:latest --name <your-cluster-name>

   # Build the Kubernetes runtime controller image
   make docker-build-runtime

   # Load runtime image into Kind cluster
   kind load docker-image kubernetes-runtime:latest --name <your-cluster-name>
   ```

4. **Deploy the controllers**:
   ```bash
   # Deploy the orchestrator controller with correct image
   make deploy IMG=example.com/kbinit:latest

   # Deploy the Kubernetes runtime controller
   make deploy-runtime

   # Apply orchestrator configuration
   kubectl apply -f test/e2e/fixtures/orchestrator-config.yaml
   ```

5. **Verify installation**:
   ```bash
   # Check CRDs are installed
   kubectl get crds | grep score.dev
   ```

   You should see output like:
   ```
   resourceclaims.score.dev
   workloadexposures.score.dev
   workloadplans.score.dev
   workloads.score.dev
   ```

   ```bash
   # Check controllers are running
   kubectl get pods -n score-system
   ```

   You should see both controllers in Running state:
   ```
   NAME                                       READY   STATUS    RESTARTS   AGE
   score-controller-manager-xxx               1/1     Running   0          1m
   kubernetes-runtime-controller-manager-xxx 1/1     Running   0          1m
   ```

> **Note**: If you see Kubernetes DRA's `ResourceClaim` when using `kubectl get resourceclaim`, always qualify with the API group: `kubectl get resourceclaim.score.dev`

## Hello Web (Minimal Workload)

Let's start with a simple web application using Score's official sample image.

1. **Apply the hello web workload**:
   ```bash
   kubectl apply -f docs/examples/hello/workload.yaml
   ```

2. **Watch the workload status in real-time**:
   ```bash
   kubectl get workload hello-web -w
   ```

   You'll see the status evolve through different phases:
   - Initially: `Ready=False` with various conditions being evaluated
   - Eventually: `Ready=True` when all conditions are satisfied

3. **Examine detailed status**:
   ```bash
   kubectl describe workload hello-web
   ```

4. **Verify Kubernetes resources are created**:
   ```bash
   # Check if a WorkloadPlan was created
   kubectl get workloadplan hello-web -o yaml

   # Check if actual Kubernetes resources were created by the runtime controller
   kubectl get deployment hello-web
   kubectl get service hello-web
   kubectl get pods -l app=hello-web
   ```

   When everything is working, you should see:
   - A `Deployment` with the specified container image
   - A `Service` exposing the specified ports
   - Pod(s) running the application

### Understanding the Status

The workload status provides these key pieces of information:

- **`endpoint`**: The primary URI for accessing your workload (appears when runtime is ready)
- **`conditions`**: Current state indicators with these standard types:
  - `Ready`: Overall readiness (True when InputsValid ∧ RuntimeReady)
  - `InputsValid`: Spec validation passed
  - `RuntimeReady`: Runtime environment is ready

Each condition includes a `reason` field with standardized values like `Succeeded`, `RuntimeProvisioning`, etc.

### What Happens Behind the Scenes

When you apply a Workload, the orchestrator:
1. **Validates** the spec and sets `InputsValid=True`
2. **Creates a WorkloadPlan** with the deployment instructions
3. **Sets RuntimeReady=True** when the runtime controller successfully processes the plan
4. **The runtime controller** creates actual Kubernetes resources (Deployment, Service, etc.)
5. **Sets endpoint** once the service is available

## Add a Dependency (Application + Postgres)

Now let's make it more interesting by adding a database dependency.

1. **Apply the enhanced workload**:
   ```bash
   kubectl apply -f docs/examples/node-postgres/workload.yaml
   ```

2. **Watch the dependency binding process**:
   ```bash
   # Watch the main workload
   kubectl get workload node-postgres-app -w

   # In another terminal, watch resource claims
   kubectl get resourceclaim.score.dev -w
   ```

3. **Observe the new condition and resource creation**:
   ```bash
   # Watch the workload status
   kubectl get workload node-postgres-app -o yaml

   # Check that a ResourceClaim was created
   kubectl get resourceclaim.score.dev

   # Inspect the claim details
   kubectl describe resourceclaim.score.dev <claim-name>

   # Verify the WorkloadPlan includes environment variable mappings
   kubectl get workloadplan node-postgres-app -o yaml

   # Check that Kubernetes resources are created with environment variables
   kubectl get deployment node-postgres-app -o yaml | grep -A 10 env:
   ```

   The workload now shows a `ClaimsReady` condition that tracks dependency status.

### Understanding Resource Binding

When you declare a resource dependency:

```yaml
resources:
  db:
    type: postgres
```

The orchestrator:
1. **Creates a ResourceClaim** to request the database
2. **Waits for a Provisioner** to fulfill the claim (provide connection details)
3. **Updates ClaimsReady condition** based on binding progress
4. **Projects database connection details** into environment variables in the WorkloadPlan
5. **Runtime controller** creates Deployment with the resolved environment variables
6. **Sets Ready=True** only when **InputsValid ∧ ClaimsReady ∧ RuntimeReady**

> **Note**: In a real environment, you'd have provisioners that can actually provide PostgreSQL instances. For testing, you might see the ResourceClaim remain in `Pending` state until a suitable provisioner is deployed.

## Reading Status Like a Pro

### Readiness Rule

A workload is `Ready=True` when **all three** conditions are `True`:
- **InputsValid**: Spec validation passed
- **ClaimsReady**: All resource dependencies are bound and available
- **RuntimeReady**: Runtime environment is ready and healthy

### Standard Condition Reasons

The `reason` field uses a fixed vocabulary for consistent tooling:

| Reason | Meaning |
|--------|---------|
| `Succeeded` | Operation completed successfully |
| `SpecInvalid` | Workload specification is invalid |
| `PolicyViolation` | Blocked by admission policy |
| `BindingPending` | Waiting for resource binding |
| `BindingFailed` | Resource binding failed |
| `RuntimeProvisioning` | Runtime is being set up |
| `RuntimeDegraded` | Runtime is unhealthy |
| `QuotaExceeded` | Resource quota exceeded |
| `PermissionDenied` | Insufficient permissions |

### Endpoint Format

The `endpoint` field contains at most one URI providing the primary access point to your workload. When multiple access points exist, the orchestrator selects the most appropriate one.

### Claims Summary

The `claims` array provides a quick overview of dependency status:

```yaml
claims:
- key: db
  phase: Bound
  reason: Succeeded
  message: PostgreSQL instance ready
  outputsAvailable: true
```

## Update & Rollout

Let's see how the orchestrator handles updates.

1. **Update the workload** (e.g., change an environment variable):
   ```bash
   kubectl patch workload node-postgres-app --type='merge' -p='{"spec":{"containers":{"app":{"variables":{"NEW_VAR":"updated_value"}}}}}'
   ```

2. **Watch the conditions converge**:
   ```bash
   kubectl get workload node-postgres-app -w
   ```

   You'll see conditions temporarily become `False` as the orchestrator processes the change, then return to `True` once the update is complete.

## Cleanup

When you're done experimenting:

1. **Delete the workloads**:
   ```bash
   kubectl delete workload hello-web node-postgres-app
   ```

2. **Verify cleanup**:
   ```bash
   # Check that resource claims are cleaned up automatically
   kubectl get resourceclaim.score.dev

   # Should show no claims or only claims from other workloads
   ```

The orchestrator automatically handles cleanup of associated `ResourceClaim` and `WorkloadPlan` resources through Kubernetes owner references.

## What's Next?

- **Explore Runtime Selection**: Learn how the orchestrator chooses between different runtime backends
- **Advanced Dependencies**: Experiment with different resource types and complex binding scenarios
- **Status Monitoring**: Build tooling around the standardized condition vocabulary
- **Troubleshooting**: Use the [troubleshooting guide](../spec/lifecycle.md) when things don't go as expected

## Comparison with Official Score

This guide mirrors the Score Get Started flow but highlights orchestrator-specific concepts:

- **Score CLI**: Translates Score files → Docker Compose/K8s manifests
- **Score Orchestrator**: Manages **Workload** → **ResourceClaim** → **WorkloadPlan** lifecycle with rich status reporting

Both approaches give you runtime-agnostic application definitions, but the orchestrator adds declarative dependency management and standardized observability.