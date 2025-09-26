# ADR-0008: Development-Grade Resource Provisioning Strategy

**Status**: Accepted
**Date**: 2025-09-27

## Context

During the implementation of the Getting Started guide (Issue #91), we discovered that the current PostgreSQL provisioner only creates a Secret with mock connection details but doesn't provision an actual PostgreSQL instance. This causes Score sample applications to fail with CrashLoopBackOff because they cannot connect to a database.

Investigation of the official Score implementation (`score-k8s`) revealed that it provides **complete development-grade provisioning** for resources like PostgreSQL, Redis, MySQL, etc., creating actual StatefulSets, Services, and persistent storage alongside credential Secrets.

Key findings from score-k8s analysis:
1. **Template-based provisioning** creates complete database instances (StatefulSet + Service + Secret + PVC)
2. **Development-first approach** with immediate functionality out-of-the-box
3. **Production replacement expectation** - users are expected to provide custom provisioners for production environments
4. **Clear documentation** about development vs. production use cases

The current approach in this project of providing only Secret-based mock provisioning creates a poor Getting Started experience and diverges from user expectations set by the official Score tooling.

## Decision

We will adopt a **development-grade provisioning strategy** aligned with score-k8s, where provisioners create functional resource instances suitable for development and demonstration purposes.

### Core Principles

1. **Development-First Experience**: Provisioners should create working resource instances that enable immediate functionality testing
2. **Production Replacement Path**: Clear documentation and architecture that allows production provisioners to replace development ones
3. **Score Ecosystem Alignment**: Match the approach and user expectations established by score-k8s
4. **Graduated Sophistication**: Start with development-grade implementations, evolve toward production-ready patterns

### Implementation Strategy

For resource types like `postgres`, `redis`, `mysql`, etc., provisioners will create:

1. **StatefulSet**: Running the actual database/service container
2. **Service**: ClusterIP service for internal connectivity
3. **Secret**: Generated credentials and connection details
4. **PersistentVolumeClaim**: Basic storage for data persistence (development-grade)

Example PostgreSQL provisioning will include:
- StatefulSet with `postgres:alpine` container
- ClusterIP Service exposing port 5432
- Secret with generated username/password
- PVC with modest storage allocation (1Gi)
- Owner references for proper cleanup

### Development vs. Production Positioning

- **Development Use**: Complete, working instances with reasonable defaults
- **Production Use**: Template for replacement with operator-based or cloud-managed resources
- **Documentation**: Clear guidance on production deployment patterns
- **Configuration**: Provision for environment-specific provisioner selection

## Consequences

### Positive

1. **Improved Getting Started Experience**: Users can immediately test complete workload functionality
2. **Score Ecosystem Consistency**: Aligns with official Score tooling expectations
3. **Reference Implementation Value**: Demonstrates complete Score capability
4. **Development Velocity**: Faster iteration cycles with working dependencies

### Negative

1. **Increased Complexity**: Provisioners become more sophisticated than simple Secret creation
2. **Resource Overhead**: Development clusters will run additional database instances
3. **Security Considerations**: Default configurations may not meet production security standards
4. **Maintenance Burden**: More complex provisioners require more maintenance

### Mitigation

1. **Clear Documentation**: Emphasize development-grade nature and production replacement paths
2. **Resource Limits**: Apply reasonable resource constraints to prevent cluster resource exhaustion
3. **Security Defaults**: Use basic security practices suitable for development environments
4. **Modular Design**: Ensure provisioners can be easily replaced or extended

## Alternatives Considered

1. **Mock-Only Provisioning (Current Approach)**
   - Rejected: Poor user experience, diverges from Score ecosystem

2. **External Dependency Requirements**
   - Rejected: Increases Getting Started complexity, requires additional setup

3. **Optional Development Mode**
   - Considered but rejected: Adds configuration complexity without clear benefit

4. **Helm-Based Provisioning**
   - Deferred: Requires Helm dependency, can be added later as alternative strategy

## References

- Issue #91: Implement actual PostgreSQL provisioning for Getting Started guide
- score-k8s default provisioners: https://github.com/score-spec/score-k8s/blob/main/internal/provisioners/default/zz-default.provisioners.yaml
- ADR-0005: Unified Provisioner Controller Design
- Getting Started guide requirements and user expectations

## Implementation Plan

1. **Phase 1**: Update PostgreSQL provisioner to create StatefulSet + Service + Secret
2. **Phase 2**: Update documentation to reflect development-grade positioning
3. **Phase 3**: Create production replacement guidance and examples
4. **Phase 4**: Apply same pattern to other resource types (Redis, MySQL, etc.)

This approach balances immediate usability with long-term architectural flexibility, positioning the project as both a useful reference implementation and a foundation for production deployment patterns.
