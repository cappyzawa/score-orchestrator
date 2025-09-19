# Reconciler Configuration

This document describes the configuration options for the Score Orchestrator reconciler behavior.

## Overview

The reconciler configuration allows platform operators to customize the behavior of the WorkloadReconciler without code changes. Configuration is loaded from a ConfigMap and supports hot-reloading.

## Configuration Structure

### ReconcilerConfig

The top-level configuration structure:

```yaml
reconciler:
  retry:
    defaultRequeueDelay: 30s
    conflictRequeueDelay: 1s
    maxRetries: 3
    backoffMultiplier: 2.0
  timeouts:
    claimTimeout: 5m
    planTimeout: 3m
    statusTimeout: 30s
    deletionTimeout: 10m
  features:
    enableDetailedLogging: false
    enableMetrics: true
    enableTracing: false
    enableExperimentalFeatures: false
```

### Retry Configuration

Controls retry behavior for reconciliation operations:

- **`defaultRequeueDelay`**: Default delay for requeuing when waiting for resources during deletion
  - Default: `30s`
  - Example: `"45s"`, `"2m"`

- **`conflictRequeueDelay`**: Delay for requeuing on resource version conflicts
  - Default: `1s`
  - Example: `"2s"`, `"500ms"`

- **`maxRetries`**: Maximum number of retries for failed operations
  - Default: `3`
  - Example: `5`

- **`backoffMultiplier`**: Multiplier for exponential backoff
  - Default: `2.0`
  - Example: `1.5`, `3.0`

### Timeout Configuration

Defines timeout settings for different operations:

- **`claimTimeout`**: Timeout for resource claim operations
  - Default: `5m`
  - Example: `"10m"`, `"30s"`

- **`planTimeout`**: Timeout for workload plan operations
  - Default: `3m`
  - Example: `"5m"`, `"1m"`

- **`statusTimeout`**: Timeout for status update operations
  - Default: `30s`
  - Example: `"60s"`, `"15s"`

- **`deletionTimeout`**: Timeout for deletion operations
  - Default: `10m`
  - Example: `"15m"`, `"5m"`

### Feature Configuration

Controls optional functionality:

- **`enableDetailedLogging`**: Enable verbose logging for debugging
  - Default: `false`
  - Example: `true`

- **`enableMetrics`**: Enable metrics collection
  - Default: `true`
  - Example: `false`

- **`enableTracing`**: Enable distributed tracing
  - Default: `false`
  - Example: `true`

- **`enableExperimentalFeatures`**: Enable experimental features
  - Default: `false`
  - Example: `true`

## Configuration Deployment

### ConfigMap Setup

Create a ConfigMap containing the reconciler configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: score-orchestrator-reconciler-config
  namespace: score-system
data:
  config.yaml: |
    apiVersion: score.dev/v1b1
    kind: OrchestratorConfig
    metadata:
      name: platform-config
    spec:
      profiles:
        - name: default
          backends:
            - backendId: k8s-default
              runtimeClass: kubernetes
              template:
                kind: manifests
                ref: "registry.example.com/templates/k8s@sha256:abc123"
              priority: 100
              version: "1.0.0"
      defaults:
        profile: default
    reconciler:
      retry:
        defaultRequeueDelay: 45s
        conflictRequeueDelay: 2s
        maxRetries: 5
      timeouts:
        claimTimeout: 10m
        planTimeout: 5m
      features:
        enableDetailedLogging: true
```

### Hot Reloading

The reconciler automatically detects ConfigMap changes and reloads the configuration without requiring a restart.

Configuration changes are applied to new reconciliation cycles. Ongoing reconciliation operations continue with the previous configuration.

## Default Values

If no configuration is provided or if the ConfigMap is not found, the reconciler uses sensible defaults:

```yaml
retry:
  defaultRequeueDelay: 30s
  conflictRequeueDelay: 1s
  maxRetries: 3
  backoffMultiplier: 2.0
timeouts:
  claimTimeout: 5m
  planTimeout: 3m
  statusTimeout: 30s
  deletionTimeout: 10m
features:
  enableDetailedLogging: false
  enableMetrics: true
  enableTracing: false
  enableExperimentalFeatures: false
```

## Validation

The configuration is validated when loaded:

- Duration values must be valid Go duration strings (e.g., `"30s"`, `"5m"`, `"1h"`)
- Numeric values must be non-negative
- Invalid values are replaced with defaults and logged as warnings

## Monitoring

Monitor reconciler configuration through:

- **Logs**: Configuration load events and validation warnings
- **Metrics**: Configuration reload count and validation errors
- **Events**: Kubernetes events for configuration changes

## Troubleshooting

### Configuration Not Loading

1. Verify ConfigMap exists in the correct namespace
2. Check ConfigMap key name matches `config.yaml`
3. Validate YAML syntax and structure
4. Review reconciler logs for error messages

### Invalid Configuration Values

1. Check reconciler logs for validation warnings
2. Verify duration strings use valid Go duration format
3. Ensure numeric values are within acceptable ranges
4. Test configuration changes in development environment first

### Performance Impact

1. Monitor reconciliation latency after configuration changes
2. Adjust timeout values based on cluster performance
3. Consider reducing retry counts for faster failure detection
4. Use detailed logging sparingly in production

## Migration

When migrating from hardcoded values:

1. Note current hardcoded values in your environment
2. Create ConfigMap with equivalent settings
3. Deploy updated reconciler with configuration support
4. Gradually adjust values based on operational needs

## Security Considerations

- Limit write access to the reconciler configuration ConfigMap
- Use RBAC to control who can modify reconciler settings
- Audit configuration changes for compliance
- Avoid exposing sensitive information in configuration values