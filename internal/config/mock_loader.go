package config

import (
	"context"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// MockLoader implements the Loader interface for testing
type MockLoader struct {
	config *scorev1b1.OrchestratorConfig
	err    error
}

// NewMockLoader creates a new MockLoader
func NewMockLoader() *MockLoader {
	return &MockLoader{
		config: &scorev1b1.OrchestratorConfig{
			Spec: scorev1b1.OrchestratorConfigSpec{
				Provisioners: []scorev1b1.ProvisionerSpec{
					{
						Type:        "test",
						Provisioner: "mock",
						Classes:     []scorev1b1.ClassSpec{},
					},
				},
			},
		},
	}
}

// LoadConfig returns the mock configuration
func (m *MockLoader) LoadConfig(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

// SetConfig sets the mock configuration
func (m *MockLoader) SetConfig(config *scorev1b1.OrchestratorConfig) {
	m.config = config
}

// SetError sets the error to be returned by Load
func (m *MockLoader) SetError(err error) {
	m.err = err
}

// Watch returns a dummy channel for testing
func (m *MockLoader) Watch(ctx context.Context) (<-chan ConfigEvent, error) {
	ch := make(chan ConfigEvent)
	close(ch) // Close immediately for testing
	return ch, nil
}

// Close does nothing for the mock
func (m *MockLoader) Close() error {
	return nil
}
