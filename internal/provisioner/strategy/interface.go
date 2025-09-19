package strategy

import (
	"context"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// Strategy defines the interface for provisioning resource types
type Strategy interface {
	// GetType returns the resource type this strategy handles
	GetType() string

	// Provision creates or updates the resource and returns outputs
	Provision(ctx context.Context, claim *scorev1b1.ResourceClaim) (*scorev1b1.ResourceClaimOutputs, error)

	// Deprovision cleans up the resource
	Deprovision(ctx context.Context, claim *scorev1b1.ResourceClaim) error

	// GetStatus returns the current status of the resource
	GetStatus(ctx context.Context, claim *scorev1b1.ResourceClaim) (phase scorev1b1.ResourceClaimPhase, reason, message string, err error)
}

// ProvisioningConfig represents configuration for a provisioning strategy
type ProvisioningConfig struct {
	Type     string            `json:"type"`
	Strategy string            `json:"strategy"`
	Config   map[string]any    `json:"config,omitempty"`
	Outputs  map[string]string `json:"outputs,omitempty"`
}
