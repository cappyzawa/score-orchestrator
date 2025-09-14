# Orchestrator Configuration Specification

This document specifies the format and structure of the Orchestrator Configuration, which serves as the single source of truth for platform mapping, runtime selection, and dependency provisioning in the Score Orchestrator.

## Overview

The **Orchestrator Configuration** is a non-CRD artifact that defines:

1. **Abstract profiles** and their runtime backend mappings
2. **Provisioner configurations** for dependency resolution
3. **Default values** and selection policies
4. **Template references** for runtime materialization

This configuration replaces the removed `PlatformPolicy` CRD and provides platform operators with flexible, declarative control over workload deployment strategies.

### Cluster-Level Environment Model (ADR-0004)

As of ADR-0004, this configuration operates under a **One Cluster, One Environment** model:

- **Environment context** is defined at the **cluster level**, not through labels
- **Backend selection** is based solely on workload characteristics (profile, features, resources)
- **Namespace labels** are not used for backend selection
- Each cluster represents exactly one environment (dev, staging, prod)

This simplification eliminates complex environment-based label matching and aligns with common operational patterns where organizations maintain separate clusters per environment.

### Distribution Methods

The configuration can be distributed as either:

- **Kubernetes ConfigMap** - Stored in the control namespace, versioned via Kubernetes
- **OCI Artifact** - Distributed via container registry, supporting immutable versioning and GitOps workflows

---

## Schema Definition

### Top-Level Structure

```yaml
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: platform-config
  version: "1.2.3"
spec:
  profiles: []        # Array of ProfileSpec
  provisioners: []    # Array of ProvisionerSpec
  defaults:           # DefaultsSpec
    profile: string
    selectors: []     # Array of SelectorSpec
```

---

## Profiles Configuration

### ProfileSpec

Defines abstract workload profiles (e.g., `web-service`, `batch-job`) and their backend candidates.

```yaml
profiles:
- name: string                    # Abstract profile name (e.g., "web-service")
  description: string             # Optional human-readable description
  backends: []                    # Array of BackendSpec
```

### BackendSpec

Represents a concrete runtime implementation for a profile.

```yaml
backends:
- backendId: string              # Stable identifier (not user-visible)
  runtimeClass: string           # Runtime class (e.g., "kubernetes", "ecs", "nomad")
  template:                      # TemplateSpec
    kind: string                 # Template type: "manifests" | "helm" | "kustomize"
    ref: string                  # Immutable reference (OCI digest recommended)
    values: object               # Optional default template values (see Values Composition)
  priority: integer              # Selection priority (higher = preferred)
  version: string                # Backend version (semver recommended)
  constraints:                   # ConstraintsSpec
    selectors: []                # Array of SelectorSpec (workload labels only, per ADR-0004)
    features: []                 # Array of required features
    regions: []                  # Array of allowed regions
    resources:                   # ResourceConstraints
      cpu: string                # e.g., "100m-4000m"
      memory: string             # e.g., "128Mi-8Gi"
      storage: string            # e.g., "1Gi-100Gi"
```

**Quantity range grammar (normative):**
- `"<q>"` (exact), `"<min>-<max>"` (inclusive), `"<min>-"` (min only), `"-<max>"` (max only).
Quantities use Kubernetes resource formats.

### Template Types

#### Manifests Template
```yaml
template:
  kind: manifests
  ref: "registry.example.com/templates/k8s-web@sha256:abc123..."
  values:
    replicas: 3
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
```

#### Helm Template
```yaml
template:
  kind: helm
  ref: "oci://registry.example.com/charts/webapp@sha256:deadbeef..."
  values:
    image:
      repository: ""              # Filled by WorkloadPlan projection
      tag: ""                     # Filled by WorkloadPlan projection
    service:
      type: LoadBalancer
```

#### Kustomize Template
```yaml
template:
  kind: kustomize
  ref: "https://github.com/example/k8s-configs//overlays/production?ref=v1.2.3"
  values:
    namespace: ""                 # Filled by WorkloadPlan projection
    namePrefix: ""                # Filled by WorkloadPlan projection
```

---

## Provisioners Configuration

### ProvisionerSpec

Defines how dependency resources (from `Workload.spec.resources`) are provisioned.

```yaml
provisioners:
- type: string                   # Resource type (e.g., "postgres", "redis", "s3")
  provisioner: string            # Controller name/identifier
  classes: []                    # Array of ClassSpec
  defaults:                      # Default parameters
    class: string
    params: object
```

### ClassSpec

Defines available service tiers/sizes for a resource type.

```yaml
classes:
- name: string                   # Class identifier (e.g., "small", "large", "enterprise")
  description: string            # Human-readable description
  parameters:                    # Class-specific parameters
    cpu: string
    memory: string
    storage: string
    replicas: integer
    backup: boolean
  constraints:                   # Access constraints
    selectors: []                # Array of SelectorSpec
    features: []                 # Required features
```

### Example Provisioner Configuration

```yaml
provisioners:
- type: postgres
  provisioner: postgres-operator
  classes:
  - name: small
    description: "Small database instance"
    parameters:
      cpu: "500m"
      memory: "1Gi"
      storage: "10Gi"
      replicas: 1
      backup: false
  - name: large
    description: "Large database instance with HA"
    parameters:
      cpu: "2000m"
      memory: "8Gi"
      storage: "100Gi"
      replicas: 3
      backup: true

- type: redis
  provisioner: redis-operator
  defaults:
    class: standard
    params:
      maxMemory: "1Gi"
      persistence: true
  classes:
  - name: standard
    parameters:
      memory: "1Gi"
      persistence: true
  - name: large
    parameters:
      memory: "8Gi"
      persistence: true
      cluster: true
```

---

## Defaults and Selectors

### DefaultsSpec

```yaml
defaults:
  profile: string                # Global default profile
  selectors: []                  # Array of conditional defaults
```

### SelectorSpec

Kubernetes-style label selectors for conditional configuration.

```yaml
selectors:
- matchLabels:                   # Map of exact label matches (workload labels only)
    workload-type: string        # Example: workload characteristic
    app-tier: string             # Example: application tier
    team: string                 # Example: organizational label (not for backend selection)
  matchExpressions: []           # Array of label selector requirements
  profile: string                # Profile to use when selector matches
  constraints:                   # Additional constraints
    features: []
    regions: []
```

**Note (ADR-0004):** Remove `environment`-based selectors. Use workload characteristics like `workload-type: batch` instead of `environment: development`.

### Example Defaults Configuration

```yaml
defaults:
  profile: web-service           # Global fallback
  selectors:
  # Simplified selectors based on workload characteristics only
  - matchExpressions:
    - key: workload-type
      operator: In
      values: ["batch", "job"]
    profile: batch-job
  - matchLabels:
      app-tier: database
    profile: batch-job           # Database maintenance jobs
```

**Label evaluation scope (normative, ADR-0004):** 
- Selectors are evaluated against **`Workload.metadata.labels` only**
- **Namespace labels are ignored** for all selection logic
- Environment-based selectors (e.g., `environment: production`) are not supported
- Each cluster represents exactly one environment
- Use workload characteristics instead: `workload-type`, `app-tier`, `component`, etc.

**Migration Note:** Replace environment-based selectors with workload characteristic-based ones when upgrading configurations.

**Selector precedence (normative):** `defaults.selectors[]` is evaluated in document order. The **first** matching selector wins; no further selectors are evaluated. `matchLabels` and `matchExpressions` are ANDed.

**Environment model (normative):** Each cluster represents exactly one environment context. Backend selection within a cluster is based on workload characteristics (profile, features, resources) rather than environment labels.

---

## Profile Selection Pipeline

The Orchestrator **MUST** use a deterministic selection pipeline to ensure reproducible deployments:

### 1. Profile Selection (Normative)
The orchestrator MUST select exactly one profile by evaluating, in order:

1. **User hint evaluation**: `score.dev/profile` annotation on Workload (if present)
2. **Auto-derivation**: Profile inferred from Workload characteristics (service ports, resource types)
3. **Selector matching**: Apply `defaults.selectors[]` based on Workload labels only (per ADR-0004)
4. **Global fallback**: Use `defaults.profile` as final fallback

### 2. Backend Filtering (Normative)
For the selected profile, the orchestrator MUST:

1. **Collect candidates** from `profile.backends[]`
2. **Apply workload selectors** - filter by `constraints.selectors[]` against Workload labels (environment selectors removed per ADR-0004)
3. **Validate feature requirements** - verify `score.dev/requirements` annotation against `constraints.features[]`
4. **Check resource constraints** - validate CPU/memory/storage against `constraints.resources`
5. **Admission control** - VAP/OPA/Kyverno policy enforcement (platform-specific)

### 3. Backend Selection (Normative)
From filtered candidates, the orchestrator MUST:

1. **Sort deterministically** by: `priority` (desc) → `version` (SemVer desc; releases rank above pre-releases) → `backendId` (lexicographical)
2. **Select first** matching candidate
3. **Handle selection failure**:
   - If no candidates remain: Set `RuntimeReady=False` with reason `RuntimeSelecting`
   - If admission denied: Set `RuntimeReady=False` with reason `PolicyViolation`

**Hint handling (normative):** If a hinted profile does not exist, set `InputsValid=False (SpecInvalid)`. If it exists but yields no viable backend, set `RuntimeReady=False (RuntimeSelecting)`.

**Tie-breaking rule**: When multiple backends have identical priority and version, selection is deterministic by lexicographical `backendId` comparison.

**Version ordering (normative):** Versions follow SemVer 2.0.0. For the same base, a release (e.g., `1.2.3`) ranks **above** any pre-release (e.g., `1.2.3-rc.1`).

### Values Composition

Template rendering uses a deterministic composition of values sources. For detailed value resolution and placeholder handling, see [Lifecycle Documentation](./lifecycle.md).

**Composition Order** (right-hand wins):
```
final_values = defaults ⊕ normalize(Workload) ⊕ outputs
```

Where:
- **defaults**: Template default values from `backend.template.values`
- **normalize(Workload)**: Normalized Workload spec (containers, service, etc.)
- **outputs**: Resolved ResourceClaim outputs (`${resources.<key>.outputs.<name>}`)

**Example Values Flow**:
1. Backend template provides base values: `{replicas: 1, resources: {cpu: "100m"}}`
2. Workload normalization adds: `{image: "myapp:1.0", ports: [{port: 8080}]}`
3. ResourceClaim outputs override: `{database_url: "postgres://..."}`
4. Final values: Combined object used for template rendering

**Projection failures (normative):** Missing required outputs in `${resources.<key>.outputs.<name>}` MUST set `RuntimeReady=False (ProjectionError)`.

---

## Configuration Examples

### Complete ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: orchestrator-config
  namespace: score-system
data:
  config.yaml: |
    apiVersion: score.dev/v1b1
    kind: OrchestratorConfig
    metadata:
      name: platform-config
      version: "1.0.0"
    spec:
      profiles:
      - name: web-service
        description: "HTTP-based web applications"
        backends:
        - backendId: k8s-web-standard
          runtimeClass: kubernetes
          template:
            kind: manifests
            ref: "registry.company.com/templates/k8s-web@sha256:abc123"
            values:
              replicas: 3
              resources:
                requests:
                  cpu: "100m"
                  memory: "128Mi"
          priority: 100
          version: "1.2.3"
          constraints:
            features: ["http-ingress"]
            resources:
              cpu: "100m-2000m"
              memory: "128Mi-4Gi"

      - name: batch-job
        description: "Batch processing workloads"
        backends:
        - backendId: k8s-job-standard
          runtimeClass: kubernetes
          template:
            kind: manifests
            ref: "registry.company.com/templates/k8s-job@sha256:def456"
          priority: 100
          version: "1.0.0"

      provisioners:
      - type: postgres
        provisioner: postgres-operator
        defaults:
          class: small
        classes:
        - name: small
          parameters:
            cpu: "500m"
            memory: "1Gi"
            storage: "10Gi"
        - name: large
          parameters:
            cpu: "2000m"
            memory: "8Gi"
            storage: "100Gi"

      defaults:
        profile: web-service
        selectors:
        - matchLabels:
            workload-type: batch
          profile: batch-job
```

### OCI Artifact Structure

When distributed as an OCI artifact, the configuration follows OCI Image Manifest v1:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.score.orchestrator.config.v1+json",
    "size": 1234,
    "digest": "sha256:abc123..."
  },
  "layers": [
    {
      "mediaType": "application/vnd.score.orchestrator.config.v1+yaml",
      "size": 5678,
      "digest": "sha256:def456..."
    }
  ],
  "annotations": {
    "org.opencontainers.image.title": "Score Orchestrator Config",
    "org.opencontainers.image.version": "1.0.0",
    "score.dev/config.profiles": "web-service,batch-job,event-consumer"
  }
}
```

---

## Validation and Constraints

### Validation Boundaries

The Orchestrator Configuration uses a different validation strategy than Workload CRDs:

**Workload Validation (CRD-based)**
- **Mechanism**: OpenAPI schema + CEL validation rules
- **Scope**: Score specification invariants, required fields, structural constraints
- **Enforcement**: Kubernetes API server at admission time

**Orchestrator Config Validation (External)**
- **Mechanism**: JSON Schema, cue lang, or external linters
- **Scope**: Configuration structure, reference validity, constraint logic
- **Enforcement**: CI/CD pipelines, GitOps workflows, or admission webhooks

**Platform Policy Validation (Admission)**
- **Mechanism**: VAP (ValidatingAdmissionPolicy), OPA Gatekeeper, or Kyverno
- **Scope**: Organization-specific rules (image registries, resource limits, team policies)
- **Enforcement**: Kubernetes admission control during Workload submission

### Required Fields
- `profiles[].name` - Must be unique within configuration
- `profiles[].backends[].backendId` - Must be unique across all profiles
- `profiles[].backends[].runtimeClass` - Must be valid runtime class
- `profiles[].backends[].template.kind` - Must be supported template type
- `profiles[].backends[].template.ref` - Must be valid, resolvable reference

### Validation Rules
1. **Profile names** must be valid DNS labels (RFC 1123)
2. **Backend IDs** must be unique across the entire configuration
3. **Template references** should use immutable references (digests) for production
4. **Priority values** must be non-negative integers
5. **Version strings** should follow semantic versioning
6. **Resource constraints** must use valid Kubernetes resource quantities

### Template Reference Requirements (Normative)

**Immutable References (Production)**

Template and provisioner references **MUST** be pinned by digest in production environments. Platforms **MUST** reject tag-only references unless explicitly allowed by policy.

- **Required format**: `registry.example.com/templates/web@sha256:abc123...`
- **Prohibited in production**: Tag-based references like `registry.example.com/templates/web:latest`
- **Policy enforcement**: Platform admission controllers SHOULD validate digest-based references

**Development Environments**

Tag-based references are acceptable for development iteration:
- `registry.example.com/templates/web:latest` (development only)
- `registry.example.com/templates/web:v1.2.3` (pre-production)

**Security Requirements**

- **Authentication**: Template registries MUST require authentication
- **Verification**: Orchestrator SHOULD verify template signatures when available
- **Supply chain**: Template references SHOULD include provenance metadata

**Caching and Performance**

- **Local caching**: Orchestrator MUST cache fetched templates locally
- **Cache invalidation**: Cache keyed by digest (immutable) or tag+timestamp (mutable)
- **Failure handling**: Cached templates used during registry unavailability

---

## Operational Considerations

### Configuration Updates
- **ConfigMap**: Updates trigger Orchestrator reconciliation automatically
- **OCI Artifact**: Requires updating the artifact reference in Orchestrator deployment
- **Versioning**: Use semantic versioning for configuration versions
- **Rollback**: Keep previous configuration versions for emergency rollback

### Monitoring and Observability
- **Metrics**: Track profile selection rates, backend utilization, template fetch times
- **Logging**: Log configuration load events, selection decisions, policy violations
- **Alerts**: Alert on configuration parse failures, template fetch failures

### Security Considerations
- **RBAC**: Limit configuration write access to platform operators
- **Secrets**: Never embed credentials in configuration; use external secret references
- **Validation**: Enforce admission policies on configuration changes
- **Audit**: Log all configuration modifications with user attribution

---

## Migration and Compatibility

### From PlatformPolicy CRD
For users migrating from the removed `PlatformPolicy` CRD:

1. **Extract profile definitions** from PlatformPolicy specs
2. **Convert backend mappings** to new backend format
3. **Migrate selector logic** to new defaults structure
4. **Update template references** to OCI-based format

### Version Compatibility
- **v1b1**: Current specification version
- **Backward compatibility**: Future versions will provide migration guides
- **Deprecation policy**: 2 release cycles notice for breaking changes

---

## References

- [ADR-0003: Architecture Simplification](../ADR/ADR-0003-architecture-simplification.md)
- [CRDs Specification](./crds.md)
- [Lifecycle Documentation](./lifecycle.md)
- [Score Specification](https://github.com/score-spec/spec)