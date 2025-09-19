package config

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// ReconcilerConfigLoader loads ReconcilerConfig from ConfigMap
type ReconcilerConfigLoader struct {
	configMapLoader ConfigLoader
	cache           *ReconcilerConfig
}

// ReconcilerLoaderOptions contains options for creating a ReconcilerConfigLoader
type ReconcilerLoaderOptions struct {
	// ConfigMapName is the name of the ConfigMap containing reconciler configuration
	ConfigMapName string

	// ConfigMapKey is the key in the ConfigMap containing the configuration
	ConfigMapKey string

	// Namespace is the namespace where the ConfigMap is located
	Namespace string
}

// DefaultReconcilerLoaderOptions returns default options for ReconcilerConfigLoader
func DefaultReconcilerLoaderOptions() ReconcilerLoaderOptions {
	return ReconcilerLoaderOptions{
		ConfigMapName: "score-orchestrator-reconciler-config",
		ConfigMapKey:  "config.yaml",
		Namespace:     "score-system",
	}
}

// NewReconcilerConfigLoader creates a new ReconcilerConfigLoader
func NewReconcilerConfigLoader(client kubernetes.Interface, opts ReconcilerLoaderOptions) *ReconcilerConfigLoader {
	loaderOpts := LoaderOptions{
		ConfigMapName: opts.ConfigMapName,
		ConfigMapKey:  opts.ConfigMapKey,
		Namespace:     opts.Namespace,
	}

	configMapLoader := NewConfigMapLoader(client, loaderOpts)

	return &ReconcilerConfigLoader{
		configMapLoader: configMapLoader,
		cache:           DefaultReconcilerConfig(),
	}
}

// LoadConfig loads ReconcilerConfig from ConfigMap
func (r *ReconcilerConfigLoader) LoadConfig(ctx context.Context) (*ReconcilerConfig, error) {
	orchestratorConfig, err := r.configMapLoader.LoadConfig(ctx)
	if err != nil {
		// If ConfigMap is not found, return default configuration
		if errors.Is(err, ErrConfigNotFound) {
			return DefaultReconcilerConfig(), nil
		}
		return nil, fmt.Errorf("failed to load orchestrator config: %w", err)
	}

	// Extract reconciler configuration from the orchestrator config
	reconcilerConfig, err := r.extractReconcilerConfig(orchestratorConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract reconciler config: %w", err)
	}

	// Validate the configuration
	if err := reconcilerConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid reconciler config: %w", err)
	}

	// Cache the configuration
	r.cache = reconcilerConfig

	return reconcilerConfig, nil
}

// GetCachedConfig returns the cached ReconcilerConfig
func (r *ReconcilerConfigLoader) GetCachedConfig() *ReconcilerConfig {
	if r.cache == nil {
		return DefaultReconcilerConfig()
	}
	return r.cache
}

// Watch watches for changes to the ReconcilerConfig and calls the callback when changes occur
func (r *ReconcilerConfigLoader) Watch(ctx context.Context, callback func(*ReconcilerConfig, error)) error {
	eventCh, err := r.configMapLoader.Watch(ctx)
	if err != nil {
		return fmt.Errorf("failed to start watching config: %w", err)
	}

	go func() {
		for event := range eventCh {
			if event.Type == ConfigEventError {
				callback(nil, event.Error)
				continue
			}

			if event.Type == ConfigEventDeleted {
				// Use default configuration when ConfigMap is deleted
				defaultConfig := DefaultReconcilerConfig()
				r.cache = defaultConfig
				callback(defaultConfig, nil)
				continue
			}

			// Extract reconciler configuration from the updated orchestrator config
			reconcilerConfig, err := r.extractReconcilerConfig(event.Config)
			if err != nil {
				callback(nil, fmt.Errorf("failed to extract reconciler config from updated config: %w", err))
				continue
			}

			// Validate the configuration
			if err := reconcilerConfig.Validate(); err != nil {
				callback(nil, fmt.Errorf("invalid updated reconciler config: %w", err))
				continue
			}

			// Cache the configuration
			r.cache = reconcilerConfig

			callback(reconcilerConfig, nil)
		}
	}()

	return nil
}

// Close closes the underlying ConfigMapLoader
func (r *ReconcilerConfigLoader) Close() error {
	if closer, ok := r.configMapLoader.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// extractReconcilerConfig extracts ReconcilerConfig from OrchestratorConfig
func (r *ReconcilerConfigLoader) extractReconcilerConfig(orchestratorConfig interface{}) (*ReconcilerConfig, error) {
	// Handle both map[string]interface{} and OrchestratorConfig struct
	var reconcilerSection interface{}
	var exists bool

	switch config := orchestratorConfig.(type) {
	case map[string]interface{}:
		// Look for reconciler section in the config map
		reconcilerSection, exists = config["reconciler"]
	case *scorev1b1.OrchestratorConfig:
		// Check if reconciler section exists in the struct
		if config.Reconciler != nil && len(config.Reconciler.Raw) > 0 {
			// Unmarshal RawExtension to map[string]interface{}
			var reconcilerMap map[string]interface{}
			if err := yaml.Unmarshal(config.Reconciler.Raw, &reconcilerMap); err != nil {
				return nil, fmt.Errorf("failed to unmarshal reconciler raw data: %w", err)
			}
			reconcilerSection = reconcilerMap
			exists = true
		}
	default:
		return nil, fmt.Errorf("unexpected config type: %T", orchestratorConfig)
	}

	if !exists {
		// If no reconciler section exists, return default configuration
		return DefaultReconcilerConfig(), nil
	}

	// Start with default configuration
	reconcilerConfig := *DefaultReconcilerConfig()

	// Marshal and unmarshal the configured section to overlay on defaults
	yamlData, err := yaml.Marshal(reconcilerSection)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reconciler config: %w", err)
	}

	// Unmarshal the configured values on top of defaults
	if err := yaml.Unmarshal(yamlData, &reconcilerConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reconciler config: %w", err)
	}

	return &reconcilerConfig, nil
}
