# Contributing to Score Orchestrator

> **Non-affiliation:** This is an independent reference project and is not affiliated with the Score Official (Score Spec Maintainers).

Score Orchestrator follows a **documentation-first development approach**. All significant changes begin with documentation and architectural decisions before implementation.

## Development Philosophy

### Documentation-Driven Development

We believe that well-designed systems start with clear specifications:

1. **Document First**: Design decisions are captured in documentation before code
2. **Specification Clarity**: APIs and behavior are precisely specified before implementation  
3. **Community Review**: Major changes go through community discussion via ADRs
4. **Implementation Follows**: Code implementation validates and realizes documented designs

### Incremental Delivery

Development proceeds through well-defined phases:

- **Phase 1**: Documentation and specifications
- **Phase 2**: CRD schema definitions and validation rules
- **Phase 3**: Controller implementations and integration testing
- **Phase 4**: Platform integrations and conformance testing

## Contributing Workflow

### 1. Issue Creation

All contributions start with GitHub issues:

**For New Features:**
```
Title: [FEATURE] Brief description
Labels: enhancement, needs-design

Description:
- Problem statement
- Proposed solution approach  
- Impact on existing specifications
- Implementation complexity estimate
```

**For Documentation Improvements:**
```
Title: [DOCS] Brief description  
Labels: documentation

Description:
- Current documentation issues
- Proposed improvements
- Affected documents
- Clarity/completeness improvements
```

**For Specification Changes:**
```
Title: [SPEC] Brief description
Labels: specification, needs-adr

Description:
- Current specification limitations
- Proposed changes and rationale
- Backward compatibility considerations
- Alternative approaches considered
```

### 2. Discussion and Design

**For Major Changes (requiring ADR):**
1. Create issue with initial proposal
2. Project maintainer discussion in issue comments
3. ADR creation following ADR template
4. ADR review and approval process (maintainer-driven in this project; not an official Score process)
5. Documentation updates based on approved ADR
6. Implementation (future phases)

**For Minor Changes:**
1. Create issue with proposed changes
2. Brief project feedback period (3-5 days)
3. Direct documentation/specification updates
4. Pull request review and merge

### 3. Architecture Decision Records (ADRs)

We track key design choices under `docs/ADR/`.

- **Statuses:** Proposed → Accepted → Deprecated → Superseded
- **Numbering:** Incremental (`ADR-0001`, `ADR-0002`, ...). Do not renumber after merge.
- **Scope:** ADRs capture externally visible contracts (GVK, visibility/RBAC, status vocabulary, controller responsibilities), not internal coding details.

#### Current Maintainer-driven ADRs

At this stage, ADRs are authored and accepted by the maintainer to keep velocity. Each ADR should still include:
- Context and problem statement
- Decision and rationale
- Alternatives considered (with reasons for rejection)
- Consequences and migration/rollback notes

#### When more contributors join

We will move to a lightweight consensus process:
- Open a PR with an ADR in `Proposed` status
- Provide at least one reviewer approval outside the author
- Keep discussion in the PR; link any external threads (e.g., score-spec discussions)
- Merge when consensus is reached and flip status to `Accepted`

**When ADR Required:**
- Changes to core CRD structure
- New controller responsibilities or boundaries  
- Changes to status aggregation logic
- New validation strategies
- RBAC or security model changes

**ADR Template:**
```markdown
# ADR-NNNN: Title

## Status
[PROPOSED | ACCEPTED | DEPRECATED | SUPERSEDED]

## Context
[Problem statement and background]

## Decision Outcomes
[What was decided]

## Considered Alternatives  
[Other approaches evaluated]

## Rationale
[Why this decision was made]

## Impact and Consequences
[Expected effects and trade-offs]
```

### 4. Pull Request Process

**Documentation Changes:**
1. Fork repository and create feature branch
2. Make changes following style guide
3. Update related documentation for consistency
4. Submit pull request with clear description
5. Address review feedback
6. Merge after approval

**Specification Changes:**
1. Ensure ADR approved (if required)
2. Update all affected specification documents
3. Verify cross-document consistency
4. Include impact analysis in PR description
5. Community review period (5-7 days)
6. Address feedback and merge

## Documentation Style Guide

### Writing Principles

**Clarity and Precision:**
- Use clear, unambiguous language
- Define technical terms consistently
- Provide examples for complex concepts
- Structure information hierarchically

**Consistency:**
- Follow established terminology throughout
- Use consistent formatting and style
- Maintain uniform section organization
- Cross-reference related concepts

**Completeness:**
- Cover all relevant use cases and scenarios
- Include both success and error paths
- Document assumptions and constraints
- Provide implementation guidance where appropriate

### Formatting Standards

**Document Structure:**
```markdown
# Document Title

Brief document description and scope.

## Overview
[High-level summary]

## Section Headings
[Detailed content with subsections]

### Subsections
[Specific topics]

#### Implementation Details
[Technical specifics]
```

**Code and Examples:**
- Use fenced code blocks with appropriate language tags
- Include conceptual examples rather than complete implementations
- Focus on structure and relationships rather than syntax
- Provide context for all code snippets

**Cross-References:**
- Link to related ADRs and specifications
- Reference external standards and specifications
- Include links to Score specification documentation
- Maintain link validity through regular review

### Content Guidelines

**API Documentation:**
- Start with conceptual overview before field details
- Group related fields and explain relationships
- Include validation rules and constraints
- Specify required vs optional fields clearly

**Process Documentation:**
- Use numbered lists for sequential processes
- Include decision points and alternative paths
- Specify responsibilities and boundaries clearly
- Document error conditions and recovery

**Architecture Documentation:**
- Lead with principles and philosophy
- Explain component relationships and interactions
- Include diagrams or text-based visualizations where helpful
- Balance high-level concepts with implementation guidance

## Community Guidelines

### Communication Standards

**Issue Discussion:**
- Stay focused on the specific issue at hand
- Provide constructive feedback and suggestions
- Ask clarifying questions when requirements are unclear
- Respect different perspectives and use cases

**Pull Request Reviews:**
- Review for clarity, accuracy, and completeness
- Check cross-document consistency
- Verify that changes align with architectural principles
- Provide specific, actionable feedback

**ADR Process:**
- Allow adequate time for community input
- Consider diverse platform requirements
- Balance community needs with implementation feasibility
- Document dissenting opinions and their resolution

### Review Criteria

**Documentation Quality:**
- [ ] Clear, unambiguous language
- [ ] Consistent terminology and formatting
- [ ] Complete coverage of the topic
- [ ] Appropriate examples and context

**Technical Accuracy:**
- [ ] Alignment with Score specification principles
- [ ] Consistency with existing architectural decisions
- [ ] Feasible implementation approach
- [ ] Proper consideration of edge cases

**Community Value:**
- [ ] Addresses real community needs
- [ ] Balances simplicity with functionality
- [ ] Maintains platform independence
- [ ] Considers diverse deployment scenarios

## Getting Started

### For New Contributors

1. **Read Core Documents:**
   - README.md for project overview
   - docs/ADR/ADR-0001-architecture.md for architectural foundation
   - docs/spec/ directory for detailed specifications

2. **Understand the Philosophy:**
   - Users only interact with Workload resources
   - Platform complexity is completely abstracted
   - Status provides minimal, neutral information
   - Implementation is platform-agnostic

3. **Choose Your Contribution:**
   - Documentation improvements (good first contribution)
   - Specification clarifications or extensions
   - New ADRs for architectural enhancements
   - Review and feedback on existing proposals

4. **Engage with Community:**
   - Join discussions on existing issues
   - Provide feedback on proposed ADRs
   - Share your use cases and requirements
   - Help improve documentation clarity

### Development Environment

**For Documentation Work:**
- Fork the repository on GitHub
- Clone your fork locally
- Use markdown-compatible editor
- Preview changes before submitting
- Test all links and cross-references

**For Future Implementation Phases:**
- Go 1.19+ for controller development
- Kubernetes 1.24+ for CRD and controller testing
- Docker for containerized development
- Kind or minikube for local testing

## Project Status and Roadmap

### Current Focus
- Core architecture specification
- CRD field definitions and relationships  
- Validation strategy and boundaries
- Lifecycle and state management
- RBAC and security model
- Community review and refinement

### Next Phases
- **CRD Implementation**: OpenAPI schemas with CEL validation
- **Controller Development**: Orchestrator, Runtime, and Provisioner controllers
- **Integration Testing**: End-to-end workflow validation
- **Platform Support**: Kubernetes, ECS, and Nomad runtime implementations
- **Conformance Testing**: Platform certification and compatibility

## Questions and Support

- **GitHub Issues**: For bugs, feature requests, and specification discussions
- **GitHub Discussions**: For questions, ideas, and community conversation
- **ADR Process**: For major architectural decisions and changes

We welcome contributions from developers, platform engineers, and anyone interested in making Score workloads more accessible and portable across diverse runtime environments.