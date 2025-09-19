// Package config provides configuration management for the Score Orchestrator
package config

import (
	"time"
)

// ReconcilerConfig contains configuration settings for the WorkloadReconciler
type ReconcilerConfig struct {
	// Retry configuration
	Retry RetryConfig `json:"retry" yaml:"retry"`

	// Timeout configuration for different phases
	Timeouts TimeoutConfig `json:"timeouts" yaml:"timeouts"`

	// Feature flags for optional functionality
	Features FeatureConfig `json:"features" yaml:"features"`
}

// RetryConfig defines retry behavior for reconciliation operations
type RetryConfig struct {
	// DefaultRequeueDelay is the default delay for requeuing when waiting for resources
	DefaultRequeueDelay time.Duration `json:"defaultRequeueDelay" yaml:"defaultRequeueDelay"`

	// ConflictRequeueDelay is the delay for requeuing on resource version conflicts
	ConflictRequeueDelay time.Duration `json:"conflictRequeueDelay" yaml:"conflictRequeueDelay"`

	// MaxRetries is the maximum number of retries for failed operations
	MaxRetries int `json:"maxRetries" yaml:"maxRetries"`

	// BackoffMultiplier is the multiplier for exponential backoff
	BackoffMultiplier float64 `json:"backoffMultiplier" yaml:"backoffMultiplier"`
}

// TimeoutConfig defines timeout settings for different operations
type TimeoutConfig struct {
	// ClaimTimeout is the timeout for resource claim operations
	ClaimTimeout time.Duration `json:"claimTimeout" yaml:"claimTimeout"`

	// PlanTimeout is the timeout for workload plan operations
	PlanTimeout time.Duration `json:"planTimeout" yaml:"planTimeout"`

	// StatusTimeout is the timeout for status update operations
	StatusTimeout time.Duration `json:"statusTimeout" yaml:"statusTimeout"`

	// DeletionTimeout is the timeout for deletion operations
	DeletionTimeout time.Duration `json:"deletionTimeout" yaml:"deletionTimeout"`
}

// FeatureConfig defines feature flags for optional functionality
type FeatureConfig struct {
	// EnableDetailedLogging enables verbose logging for debugging
	EnableDetailedLogging bool `json:"enableDetailedLogging" yaml:"enableDetailedLogging"`

	// EnableMetrics enables metrics collection
	EnableMetrics bool `json:"enableMetrics" yaml:"enableMetrics"`

	// EnableTracing enables distributed tracing
	EnableTracing bool `json:"enableTracing" yaml:"enableTracing"`

	// EnableExperimentalFeatures enables experimental features
	EnableExperimentalFeatures bool `json:"enableExperimentalFeatures" yaml:"enableExperimentalFeatures"`
}

// DefaultReconcilerConfig returns a ReconcilerConfig with sensible defaults
func DefaultReconcilerConfig() *ReconcilerConfig {
	return &ReconcilerConfig{
		Retry: RetryConfig{
			DefaultRequeueDelay:  30 * time.Second,
			ConflictRequeueDelay: 1 * time.Second,
			MaxRetries:           3,
			BackoffMultiplier:    2.0,
		},
		Timeouts: TimeoutConfig{
			ClaimTimeout:    5 * time.Minute,
			PlanTimeout:     3 * time.Minute,
			StatusTimeout:   30 * time.Second,
			DeletionTimeout: 10 * time.Minute,
		},
		Features: FeatureConfig{
			EnableDetailedLogging:      false,
			EnableMetrics:              true,
			EnableTracing:              false,
			EnableExperimentalFeatures: false,
		},
	}
}

// Validate validates the reconciler configuration
func (c *ReconcilerConfig) Validate() error {
	if c.Retry.DefaultRequeueDelay <= 0 {
		c.Retry.DefaultRequeueDelay = 30 * time.Second
	}

	if c.Retry.ConflictRequeueDelay <= 0 {
		c.Retry.ConflictRequeueDelay = 1 * time.Second
	}

	if c.Retry.MaxRetries < 0 {
		c.Retry.MaxRetries = 3
	}

	if c.Retry.BackoffMultiplier <= 1.0 {
		c.Retry.BackoffMultiplier = 2.0
	}

	if c.Timeouts.ClaimTimeout <= 0 {
		c.Timeouts.ClaimTimeout = 5 * time.Minute
	}

	if c.Timeouts.PlanTimeout <= 0 {
		c.Timeouts.PlanTimeout = 3 * time.Minute
	}

	if c.Timeouts.StatusTimeout <= 0 {
		c.Timeouts.StatusTimeout = 30 * time.Second
	}

	if c.Timeouts.DeletionTimeout <= 0 {
		c.Timeouts.DeletionTimeout = 10 * time.Minute
	}

	return nil
}
