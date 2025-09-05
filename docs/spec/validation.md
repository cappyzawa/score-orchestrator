# Validation Strategy and Implementation

This document outlines the validation approach for Score Orchestrator, defining clear boundaries between specification-level validation (community-provided) and organization-specific policy enforcement (platform-delegated).

## Validation Philosophy

Score Orchestrator implements a **two-tier validation strategy**:

1. **Specification Validation**: Ensures structural correctness and Score specification compliance
2. **Policy Validation**: Enforces organization-specific governance and security requirements

This separation allows the community to maintain consistent specification compliance while enabling platforms to implement customized governance without fragmenting the core API.

## Tier 1: Specification Validation (CRD OpenAPI + CEL)

Enforced at the CRD level using **OpenAPI schema validation** and **Common Expression Language (CEL)** rules. These validations are **community-provided** and focus on specification-level invariants that must hold for any valid Score workload.

### Structural Requirements

#### Required Fields Validation

**Container Requirements:**
- `spec.containers[].image` must be present and non-empty
- Container names must follow DNS subdomain naming conventions
- `containers` **must be present** and contain at least one container

**Service Requirements:**
- `spec.service.ports[].port` must be present when service is defined
- Port numbers must be in valid range (1-65535)
- Port names must be unique within a service

**Resource Requirements:**
- `spec.resources[].type` must be present and non-empty
- Resource keys must follow DNS subdomain naming conventions
- Resource type must match known Score resource type patterns

#### OneOf Constraints

**File Sources (within containers):**
```cel
spec.containers.all(c,
  !has(c.files) || c.files.all(f,
    ([has(f.source), has(f.content), has(f.binaryContent)]
      .where(x, x).size() == 1)
  )
)
```

#### Cross-Field Validation

**Resource Reference Consistency:**
```cel
// Environment variables referencing resources must reference existing resources
spec.containers.all(container,
  container.env.all(envVar,
    has(envVar.from) && has(envVar.from.resourceRef) ?
      envVar.from.resourceRef in spec.resources : true
  )
)
```

**Volume Mount Validation:**
```cel
// Volume mounts must reference defined resources or local sources
spec.containers.all(container,
  container.volumes.all(volume,
    has(volume.source.resource) ? 
      volume.source.resource in spec.resources : true
  )
)
```

### Format Validations

**URI Formats:**
- `status.endpoint` must conform to URI format when present
- Resource connection strings must be valid URIs where applicable

**Naming Conventions:**
- All resource keys, container names, and service port names must follow Kubernetes naming conventions
- Labels and annotations must conform to Kubernetes metadata standards

**Version Constraints:**
- API version fields must match supported versions
- Workload metadata must include required labels for orchestration

### Spec-level invariants (enforced via CRD OpenAPI + CEL)

- `containers` is a non-empty map; each container **requires** `image`.
- `containers.*.image` must be either `"."` or a valid OCI image reference string. Placeholders are **not** supported in `image`.
- If `service.ports` is present, each port **requires** `port` (integer).
- If `resources` is present, each item **requires** `type`.
- For `files[*]`, **exactly one** of `content | binaryContent | source` must be set.

**CEL examples (illustrative)**
- Exactly-one for files:
```cel
([has(self.source), has(self.content), has(self.binaryContent)]
.where(x, x).size() == 1)
```
- Non-empty containers map:
```cel
size(self.containers) > 0
```
- Image field (allow dot or OCI reference):
```cel
(self.image == ".") ||
regex_match(
  self.image,
  "^[a-z0-9]+([._-][a-z0-9]+)*/?[a-z0-9._/-]+(:[A-Za-z0-9._-]+)?(@sha256:[A-Fa-f0-9]{64})?$"
)
```

(Organization-specific policy — registry allow-lists, naming, resource limits — is out of scope here; use VAP/OPA/Kyverno.)

### Score Specification Compliance

**Score Field Mapping:**
- Validate that Score-specific fields (containers, service, resources) map correctly to CRD structure
- Ensure Score resource types are recognized and properly structured
- Validate Score metadata and annotation requirements

## Tier 2: Organization Policy Validation (Platform-Delegated)

Organization-specific policies are **NOT enforced at the CRD level**. Instead, platforms implement these using:

- **ValidatingAdmissionPolicy (VAP)** for Kubernetes-native policy enforcement
- **Open Policy Agent (OPA)** with Gatekeeper for complex policy scenarios
- **Kyverno** for YAML-based policy definitions
- Custom admission controllers for specialized validation logic

### Security and Governance Policies

**Image Registry Restrictions:**
```yaml
# Example VAP/OPA policy (NOT in CRD)
# Restrict container images to approved registries
spec:
  containers:
    - image: "myregistry.company.com/app:v1.0"  # Allowed
    - image: "docker.io/nginx:latest"          # Blocked by policy
```

**Resource Quotas and Limits:**
```yaml
# Example organization policy
# Enforce resource consumption limits
spec:
  containers:
    - resources:
        requests:
          memory: "128Mi"    # Within limits
          cpu: "100m"        # Within limits
        limits:
          memory: "1Gi"      # May exceed org limits
```

**Naming and Labeling Standards:**
```yaml
# Example naming policy
metadata:
  name: "myapp-prod-v1"           # Follows naming convention  
  labels:
    app.kubernetes.io/name: "myapp"      # Required label present
    team: "platform"                     # Required team label
    environment: "production"            # Required environment label
```

**Runtime Class Restrictions:**

> Runtime class selection is governed by `PlatformPolicy` and must not appear in `Workload.spec`.

**Network and Security Policies:**
```yaml
# Example security policy  
spec:
  containers:
    - securityContext:
        runAsNonRoot: true        # Required by policy
        runAsUser: 1000           # Non-privileged user
        privileged: false         # Unprivileged container
  service:
    ports:
      - port: 8080                # Allowed port range
      - port: 22                  # SSH port blocked by policy
```

### Environmental and Operational Policies

**Environment-Specific Constraints:**
```yaml
# Development environment - more permissive
spec:
  containers:
    - image: "dev-registry/app:latest"    # Latest tags allowed in dev

# Production environment - strict controls  
spec:
  containers:
    - image: "prod-registry/app:v1.2.3"   # Semantic versioning required
    - image: "prod-registry/app:latest"   # Latest tags blocked in prod
```

**Cost and Resource Management:**
```yaml
# Example cost control policies
spec:
  resources:
    database:
      type: "postgresql"
      class: "small"              # Cost-effective for dev
      class: "enterprise"         # May require approval for cost
```

## Policy Implementation Examples

### ValidatingAdmissionPolicy (VAP) Example

```yaml
apiVersion: admissionregistration.k8s.io/v1alpha1
kind: ValidatingAdmissionPolicy
metadata:
  name: score-image-registry-policy
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - operations: ["CREATE", "UPDATE"]
      apiGroups: ["score.dev"]
      apiVersions: ["v1b1"]
      resources: ["workloads"]
  validations:
  - expression: |
      object.spec.containers.all(container,
        container.image.startsWith('myregistry.company.com/')
      )
    message: "Container images must be from approved registry"
```

### OPA/Gatekeeper Policy Example

```yaml
apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: scoreworkloadpolicy
spec:
  crd:
    spec:
      names:
        kind: ScoreWorkloadPolicy
      validation:
        properties:
          allowedTeams:
            type: array
            items:
              type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package scoreworkloadpolicy
        
        violation[{"msg": msg}] {
          input.review.object.kind == "Workload"
          not input.review.object.metadata.labels["team"] in input.parameters.allowedTeams
          msg := "Workload must have valid team label"
        }
```

## Validation Boundaries and Responsibilities

### Community Responsibilities (CRD Level)

**ENFORCE:**
- Score specification compliance
- Required field presence
- Structural constraints (OneOf, mutual exclusivity)
- Format validation (URIs, naming conventions)
- Cross-field consistency
- API version compatibility

**DO NOT ENFORCE:**
- Organization-specific naming patterns
- Registry or image restrictions  
- Resource quotas or limits
- Team or environment-specific policies
- Cost or approval workflows
- Security scanning requirements

### Platform Responsibilities (Policy Level)

**ENFORCE:**
- Image registry allowlists/blocklists
- Resource consumption limits
- Security and compliance requirements
- Organization naming conventions
- Team and environment access controls
- Cost management and approval workflows
- Vulnerability and scanning policies

**DO NOT ENFORCE:**
- Score specification structure (handled by CRD)
- Basic field validation (handled by CRD)
- API compatibility (handled by CRD)

## Testing and Validation

### CRD Validation Testing

Community-provided tests should cover:
- Valid and invalid Score specifications
- Required field validation
- OneOf constraint enforcement
- Cross-field reference validation
- Format and naming compliance

### Policy Validation Testing

Platform teams should test:
- Organization-specific policy enforcement
- Environment-based rule variations
- Integration with admission controllers
- Policy exemption and override mechanisms
- Performance impact of policy evaluation

## Migration and Compatibility

### CRD Schema Evolution

- CRD validation rules must be backward-compatible
- New validation rules should have feature gates or gradual rollout
- Breaking validation changes require API version bumps

### Policy Evolution

- Platform policies can evolve independently of CRD validation
- Policy changes should include migration guides for affected workloads
- Policy rollback mechanisms should be available for emergency changes

This separation ensures that Score Orchestrator remains both specification-compliant and organizationally flexible while maintaining clear boundaries of responsibility between community and platform concerns.