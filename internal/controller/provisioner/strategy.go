/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provisioner

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// Strategy defines the interface for provisioning strategies
type Strategy interface {
	// Provision executes the provisioning logic for a ResourceClaim
	Provision(ctx context.Context, kubeClient client.Client, claim *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error)
}

// ProvisioningResult represents the result of a provisioning operation
type ProvisioningResult struct {
	// Phase is the resulting ResourceClaim phase
	Phase scorev1b1.ResourceClaimPhase

	// Reason is a brief reason for the phase
	Reason string

	// Message is a human-readable message describing the status
	Message string

	// Outputs contains the generated outputs for the ResourceClaim
	Outputs scorev1b1.ResourceClaimOutputs

	// RequeueAfter indicates when to requeue if phase is not final
	RequeueAfter time.Duration
}

// TemplateContext provides context for template variable substitution
type TemplateContext struct {
	// ClaimName is the ResourceClaim name
	ClaimName string

	// ClaimKey is the resource key from Workload spec
	ClaimKey string

	// Namespace is the target namespace
	Namespace string

	// Type is the resource type
	Type string

	// Class is the optional class specification
	Class *string

	// Params are custom parameters from ResourceClaim
	Params *runtime.RawExtension

	// ClassParams are resolved class-specific parameters
	ClassParams map[string]interface{}

	// Secrets contains generated secret information
	Secrets map[string]string

	// Services contains generated service information
	Services map[string]string

	// Response contains API response data (for external-api strategy)
	Response map[string]interface{}
}

// NewStrategy creates a new provisioning strategy based on the strategy name
func NewStrategy(strategyName string) (Strategy, error) {
	switch strategyName {
	case "helm":
		return &HelmStrategy{}, nil
	case "manifests":
		return &ManifestsStrategy{}, nil
	case "external-api":
		return &ExternalApiStrategy{}, nil
	default:
		return nil, fmt.Errorf("unknown provisioning strategy: %s", strategyName)
	}
}

// Common strategy errors
var (
	ErrStrategyNotImplemented = fmt.Errorf("strategy not implemented")
	ErrInvalidConfiguration   = fmt.Errorf("invalid strategy configuration")
	ErrTemplateSubstitution   = fmt.Errorf("template substitution failed")
)
