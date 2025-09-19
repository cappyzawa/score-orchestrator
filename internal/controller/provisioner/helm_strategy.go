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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// HelmStrategy implements the Helm chart deployment strategy
type HelmStrategy struct{}

// Provision implements the Strategy interface for Helm chart deployment
func (s *HelmStrategy) Provision(ctx context.Context, kubeClient client.Client, claim *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error) {
	logger := log.FromContext(ctx)

	// Validate Helm configuration
	if config.Helm == nil {
		return nil, fmt.Errorf("%w: helm configuration is required for helm strategy", ErrInvalidConfiguration)
	}

	helmConfig := config.Helm
	if helmConfig.Chart == "" {
		return nil, fmt.Errorf("%w: helm chart name is required", ErrInvalidConfiguration)
	}

	logger.Info("Processing Helm provisioning strategy",
		"chart", helmConfig.Chart,
		"repository", helmConfig.Repository,
		"version", helmConfig.Version,
	)

	// For Core Controller Implementation, we implement basic structure only
	// Full Helm integration will be implemented in Phase 2

	// Check current claim phase
	switch claim.Status.Phase {
	case "":
		// Initial state - start provisioning
		return s.startProvisioning(ctx, kubeClient, claim, config, templateCtx)
	case scorev1b1.ResourceClaimPhasePending:
		// Already started - check progress
		return s.checkProgress(ctx, kubeClient, claim, config, templateCtx)
	case scorev1b1.ResourceClaimPhaseClaiming:
		// In progress - continue checking
		return s.checkProgress(ctx, kubeClient, claim, config, templateCtx)
	case scorev1b1.ResourceClaimPhaseBound:
		// Already bound - ensure outputs are set
		return s.ensureOutputs(ctx, kubeClient, claim, config, templateCtx)
	case scorev1b1.ResourceClaimPhaseFailed:
		// Failed - could retry depending on reason
		return s.handleFailure(ctx, kubeClient, claim, config, templateCtx)
	default:
		return &ProvisioningResult{
			Phase:   scorev1b1.ResourceClaimPhaseFailed,
			Reason:  "UnknownPhase",
			Message: fmt.Sprintf("Unknown claim phase: %s", claim.Status.Phase),
		}, nil
	}
}

// startProvisioning initiates the Helm chart deployment
func (s *HelmStrategy) startProvisioning(ctx context.Context, _ client.Client, _ *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error) {
	logger := log.FromContext(ctx)

	// Populate template context with class parameters and generated values
	s.populateTemplateContext(templateCtx, config)

	// Prepare Helm values with template substitution
	values, err := s.prepareHelmValues(config.Helm, templateCtx)
	if err != nil {
		return &ProvisioningResult{
			Phase:   scorev1b1.ResourceClaimPhaseFailed,
			Reason:  "TemplateError",
			Message: fmt.Sprintf("Failed to prepare Helm values: %v", err),
		}, nil
	}

	logger.Info("Prepared Helm values for deployment", "values", values)

	// TODO: Implement actual Helm chart deployment
	// For now, simulate by transitioning to Binding phase
	logger.Info("Starting Helm chart deployment (simulated)", "chart", config.Helm.Chart)

	return &ProvisioningResult{
		Phase:        scorev1b1.ResourceClaimPhaseClaiming,
		Reason:       "HelmDeploymentStarted",
		Message:      fmt.Sprintf("Helm chart deployment started for %s", config.Helm.Chart),
		RequeueAfter: 30 * time.Second,
	}, nil
}

// checkProgress checks the progress of Helm chart deployment
func (s *HelmStrategy) checkProgress(ctx context.Context, _ client.Client, claim *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error) {
	logger := log.FromContext(ctx)

	logger.Info("Checking Helm deployment progress (simulated)")

	// TODO: Implement actual Helm deployment status checking
	// For now, simulate successful deployment after some time

	// Check if deployment has been running long enough (simulation)
	if claim.CreationTimestamp.Add(2 * time.Minute).Before(time.Now()) {
		// Simulate successful deployment
		outputs, err := s.generateOutputs(config, templateCtx)
		if err != nil {
			return &ProvisioningResult{
				Phase:   scorev1b1.ResourceClaimPhaseFailed,
				Reason:  "OutputGenerationFailed",
				Message: fmt.Sprintf("Failed to generate outputs: %v", err),
			}, nil
		}

		return &ProvisioningResult{
			Phase:   scorev1b1.ResourceClaimPhaseBound,
			Reason:  "HelmDeploymentComplete",
			Message: "Helm chart deployed successfully",
			Outputs: outputs,
		}, nil
	}

	// Still in progress
	return &ProvisioningResult{
		Phase:        scorev1b1.ResourceClaimPhaseClaiming,
		Reason:       "HelmDeploymentInProgress",
		Message:      "Helm chart deployment in progress",
		RequeueAfter: 30 * time.Second,
	}, nil
}

// ensureOutputs ensures outputs are properly set for bound claims
func (s *HelmStrategy) ensureOutputs(_ context.Context, _ client.Client, claim *scorev1b1.ResourceClaim, _ *scorev1b1.ProvisionerConfig, _ *TemplateContext) (*ProvisioningResult, error) {
	// Already bound, just return current state
	return &ProvisioningResult{
		Phase:   scorev1b1.ResourceClaimPhaseBound,
		Reason:  "AlreadyBound",
		Message: "Resource is already bound",
		Outputs: claim.Status.Outputs,
	}, nil
}

// handleFailure handles failed claims
func (s *HelmStrategy) handleFailure(_ context.Context, _ client.Client, claim *scorev1b1.ResourceClaim, _ *scorev1b1.ProvisionerConfig, _ *TemplateContext) (*ProvisioningResult, error) {
	// For now, keep the failed state
	// In a full implementation, we might retry under certain conditions
	return &ProvisioningResult{
		Phase:   scorev1b1.ResourceClaimPhaseFailed,
		Reason:  claim.Status.Reason,
		Message: claim.Status.Message,
	}, nil
}

// populateTemplateContext enriches the template context with Helm-specific data
func (s *HelmStrategy) populateTemplateContext(templateCtx *TemplateContext, _ *scorev1b1.ProvisionerConfig) {
	// Use the common function to populate basic context
	PopulateTemplateContext(templateCtx, make(map[string]interface{}))

	// Add Helm-specific context if needed
	// This could include chart-specific defaults, repository information, etc.
}

// prepareHelmValues prepares Helm chart values with template substitution
func (s *HelmStrategy) prepareHelmValues(helmConfig *scorev1b1.HelmStrategy, templateCtx *TemplateContext) (map[string]interface{}, error) {
	if helmConfig.Values == nil {
		return make(map[string]interface{}), nil
	}

	// Create template engine
	engine := NewTemplateEngine()

	// Perform template substitution on the values
	substituted, err := engine.SubstituteJSON(helmConfig.Values, templateCtx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateSubstitution, err)
	}

	// Convert to map
	values, ok := substituted.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: values must be an object", ErrInvalidConfiguration)
	}

	return values, nil
}

// generateOutputs generates ResourceClaim outputs based on configuration
func (s *HelmStrategy) generateOutputs(config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (scorev1b1.ResourceClaimOutputs, error) {
	var outputs scorev1b1.ResourceClaimOutputs

	if config.Outputs == nil {
		return outputs, nil
	}

	engine := NewTemplateEngine()

	// Generate URI output if configured
	if uriTemplate, exists := config.Outputs["uri"]; exists {
		uri, err := engine.Substitute(uriTemplate, templateCtx)
		if err != nil {
			return outputs, fmt.Errorf("failed to substitute URI template: %w", err)
		}
		outputs.URI = &uri
	}

	// Generate secretRef output if configured
	if secretTemplate, exists := config.Outputs["secretRef"]; exists {
		secretRef, err := engine.Substitute(secretTemplate, templateCtx)
		if err != nil {
			return outputs, fmt.Errorf("failed to substitute secretRef template: %w", err)
		}
		outputs.SecretRef = &scorev1b1.LocalObjectReference{Name: secretRef}
	}

	// TODO: Add support for other output types (configMapRef, cert, etc.)

	return outputs, nil
}
