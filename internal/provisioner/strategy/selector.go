package strategy

import (
	"fmt"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// Selector manages strategy selection and configuration
type Selector struct {
	strategies map[string]Strategy
	configs    map[string]*ProvisioningConfig
}

// NewSelector creates a new strategy selector
func NewSelector() *Selector {
	return &Selector{
		strategies: make(map[string]Strategy),
		configs:    make(map[string]*ProvisioningConfig),
	}
}

// RegisterStrategy registers a strategy for a resource type
func (s *Selector) RegisterStrategy(strategy Strategy) {
	s.strategies[strategy.GetType()] = strategy
}

// LoadConfig loads provisioning configuration
func (s *Selector) LoadConfig(configs []ProvisioningConfig) {
	s.configs = make(map[string]*ProvisioningConfig)
	for i := range configs {
		config := &configs[i]
		s.configs[config.Type] = config
	}
}

// GetStrategy returns the strategy for a given resource type
func (s *Selector) GetStrategy(resourceType string) (Strategy, error) {
	strategy, exists := s.strategies[resourceType]
	if !exists {
		return nil, fmt.Errorf("no strategy registered for resource type: %s", resourceType)
	}
	return strategy, nil
}

// GetConfig returns the configuration for a given resource type
func (s *Selector) GetConfig(resourceType string) (*ProvisioningConfig, error) {
	config, exists := s.configs[resourceType]
	if !exists {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}
	return config, nil
}

// GetSupportedTypes returns all supported resource types
func (s *Selector) GetSupportedTypes() []string {
	types := make([]string, 0, len(s.strategies))
	for resourceType := range s.strategies {
		types = append(types, resourceType)
	}
	return types
}

// IsSupported checks if a resource type is supported
func (s *Selector) IsSupported(claim *scorev1b1.ResourceClaim) bool {
	_, exists := s.strategies[claim.Spec.Type]
	return exists
}
