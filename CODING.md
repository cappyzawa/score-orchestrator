# Coding Guidelines

## File Organization Principles

### Domain-Driven Design for Controllers

Controller packages should be organized by **domain responsibility** rather than by technical implementation details. This approach improves maintainability, readability, and allows teams to work on different aspects of the system independently.

#### File Structure

```
internal/controller/
├── workload_controller.go      # Main reconciliation orchestration
├── workload_lifecycle.go       # Workload state management and deletion
├── resource_coordination.go    # Resource binding and plan management
├── runtime_management.go       # Runtime selection and endpoint derivation
├── reconcile_flow.go           # Error handling and flow control
├── watches.go                  # Event watching and indexing
└── suite_test.go               # Test suite setup
```

#### Domain Responsibilities

**workload_lifecycle.go**
- Workload state transitions
- Deletion and cleanup logic
- Lifecycle-related constants and events
- Finalizer management

**resource_coordination.go**
- ResourceClaim coordination
- WorkloadPlan management
- Resource binding events and constants
- Cross-resource dependencies

**runtime_management.go**
- Backend selection logic
- Endpoint derivation
- Runtime integration
- Platform-specific configurations

**reconcile_flow.go**
- Error handling and categorization
- Reconciliation flow control
- Validation logic
- Status update coordination

#### Benefits

1. **Cohesion**: Related code changes together
2. **Separation of Concerns**: Each file has a single, clear responsibility
3. **Team Scalability**: Different teams can work on different domains
4. **Maintainability**: Easier to understand and modify specific functionality

#### Guidelines

- **Co-locate constants with their usage**: Don't separate constants into generic files
- **Group by change reason**: If code changes for the same business reason, keep it together
- **Avoid technical grouping**: Don't group by "all errors" or "all constants"
- **Maintain domain boundaries**: Avoid cross-domain dependencies where possible

#### Anti-Patterns

❌ **Technical Grouping**
```
constants.go    # All constants regardless of domain
errors.go      # All errors regardless of context
utils.go       # Miscellaneous utilities
```

✅ **Domain Grouping**
```
workload_lifecycle.go     # Lifecycle constants + logic
resource_coordination.go  # Resource constants + logic
runtime_management.go     # Runtime constants + logic
```

## Naming Conventions

### File Names
- Use descriptive domain names: `workload_lifecycle.go` not `lifecycle.go`
- Avoid generic names: `utils.go`, `helpers.go`, `common.go`
- Use snake_case for file names following Go conventions

### Constants
- Use domain-specific prefixes
- Group related constants together
- Include documentation explaining the business context

### Functions and Methods
- Method receivers should be on the appropriate domain file
- Helper functions should be close to their usage
- Avoid large utility packages

## Testing

### File Organization
- Keep test files alongside their corresponding source files
- Use `_test.go` suffix for all test files
- Organize integration tests by the domain they primarily test

### Test Structure
- Focus on domain behavior rather than implementation details
- Use descriptive test names that explain the business scenario
- Group related test cases using nested `Describe` blocks

## Migration Strategy

When refactoring existing code:

1. **Identify domains** in the current codebase
2. **Create new domain files** with appropriate structure
3. **Move related code** together (constants + logic + tests)
4. **Update imports** and ensure tests pass
5. **Remove old generic files** (constants.go, errors.go, etc.)

This approach ensures that code organization reflects the business domains and improves long-term maintainability.
