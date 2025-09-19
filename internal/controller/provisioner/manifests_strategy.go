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
	"sigs.k8s.io/controller-runtime/pkg/log"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// ManifestsStrategy implements the Kubernetes manifests deployment strategy
type ManifestsStrategy struct{}

// Provision implements the Strategy interface for Kubernetes manifests deployment
func (s *ManifestsStrategy) Provision(ctx context.Context, kubeClient client.Client, claim *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error) {
	logger := log.FromContext(ctx)

	// Validate manifests configuration
	if len(config.Manifests) == 0 {
		return nil, fmt.Errorf("%w: manifests configuration is required for manifests strategy", ErrInvalidConfiguration)
	}

	logger.Info("Processing Manifests provisioning strategy", "manifestCount", len(config.Manifests))

	// For Core Controller Implementation, we implement basic structure only
	// Full manifest deployment will be implemented in Phase 2

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

// startProvisioning initiates the manifest deployment
func (s *ManifestsStrategy) startProvisioning(ctx context.Context, _ client.Client, _ *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error) {
	logger := log.FromContext(ctx)

	// Populate template context with class parameters and generated values
	s.populateTemplateContext(templateCtx, config)

	// Prepare manifests with template substitution
	manifests, err := s.prepareManifests(config.Manifests, templateCtx)
	if err != nil {
		return &ProvisioningResult{
			Phase:   scorev1b1.ResourceClaimPhaseFailed,
			Reason:  "TemplateError",
			Message: fmt.Sprintf("Failed to prepare manifests: %v", err),
		}, nil
	}

	logger.Info("Prepared manifests for deployment", "count", len(manifests))

	// TODO: Implement actual manifest deployment
	// For now, simulate by transitioning to Binding phase
	logger.Info("Starting manifest deployment (simulated)", "manifestCount", len(manifests))

	return &ProvisioningResult{
		Phase:        scorev1b1.ResourceClaimPhaseClaiming,
		Reason:       "ManifestDeploymentStarted",
		Message:      fmt.Sprintf("Manifest deployment started for %d resources", len(manifests)),
		RequeueAfter: 30 * time.Second,
	}, nil
}

// checkProgress checks the progress of manifest deployment
func (s *ManifestsStrategy) checkProgress(ctx context.Context, _ client.Client, claim *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error) {
	logger := log.FromContext(ctx)

	logger.Info("Checking manifest deployment progress (simulated)")

	// TODO: Implement actual manifest deployment status checking
	// For now, simulate successful deployment after some time

	// Check if deployment has been running long enough (simulation)
	if claim.CreationTimestamp.Add(1 * time.Minute).Before(time.Now()) {
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
			Reason:  "ManifestDeploymentComplete",
			Message: "Manifests deployed successfully",
			Outputs: outputs,
		}, nil
	}

	// Still in progress
	return &ProvisioningResult{
		Phase:        scorev1b1.ResourceClaimPhaseClaiming,
		Reason:       "ManifestDeploymentInProgress",
		Message:      "Manifest deployment in progress",
		RequeueAfter: 30 * time.Second,
	}, nil
}

// ensureOutputs ensures outputs are properly set for bound claims
func (s *ManifestsStrategy) ensureOutputs(_ context.Context, _ client.Client, claim *scorev1b1.ResourceClaim, _ *scorev1b1.ProvisionerConfig, _ *TemplateContext) (*ProvisioningResult, error) {
	// Already bound, just return current state
	return &ProvisioningResult{
		Phase:   scorev1b1.ResourceClaimPhaseBound,
		Reason:  "AlreadyBound",
		Message: "Resource is already bound",
		Outputs: claim.Status.Outputs,
	}, nil
}

// handleFailure handles failed claims
func (s *ManifestsStrategy) handleFailure(_ context.Context, _ client.Client, claim *scorev1b1.ResourceClaim, _ *scorev1b1.ProvisionerConfig, _ *TemplateContext) (*ProvisioningResult, error) {
	// For now, keep the failed state
	// In a full implementation, we might retry under certain conditions
	return &ProvisioningResult{
		Phase:   scorev1b1.ResourceClaimPhaseFailed,
		Reason:  claim.Status.Reason,
		Message: claim.Status.Message,
	}, nil
}

// populateTemplateContext enriches the template context with manifests-specific data
func (s *ManifestsStrategy) populateTemplateContext(templateCtx *TemplateContext, _ *scorev1b1.ProvisionerConfig) {
	// Use the common function to populate basic context
	PopulateTemplateContext(templateCtx, make(map[string]interface{}))

	// Add manifests-specific context if needed
	// This could include namespace-specific information, label selectors, etc.
}

// prepareManifests prepares Kubernetes manifests with template substitution
func (s *ManifestsStrategy) prepareManifests(manifestsConfig []runtime.RawExtension, templateCtx *TemplateContext) ([]map[string]interface{}, error) {
	manifests := make([]map[string]interface{}, 0, len(manifestsConfig))

	engine := NewTemplateEngine()

	for i, manifestRaw := range manifestsConfig {
		// Perform template substitution on each manifest
		substituted, err := engine.SubstituteJSON(manifestRaw, templateCtx)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to substitute manifest %d: %v", ErrTemplateSubstitution, i, err)
		}

		// Convert to map
		manifest, ok := substituted.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: manifest %d must be an object", ErrInvalidConfiguration, i)
		}

		// Validate required fields
		if err := s.validateManifest(manifest); err != nil {
			return nil, fmt.Errorf("manifest %d validation failed: %w", i, err)
		}

		manifests = append(manifests, manifest)
	}

	return manifests, nil
}

// validateManifest validates a single manifest
func (s *ManifestsStrategy) validateManifest(manifest map[string]interface{}) error {
	// Check required fields
	if _, exists := manifest["apiVersion"]; !exists {
		return fmt.Errorf("%w: apiVersion is required", ErrInvalidConfiguration)
	}

	if _, exists := manifest["kind"]; !exists {
		return fmt.Errorf("%w: kind is required", ErrInvalidConfiguration)
	}

	if _, exists := manifest["metadata"]; !exists {
		return fmt.Errorf("%w: metadata is required", ErrInvalidConfiguration)
	}

	return nil
}

// generateOutputs generates ResourceClaim outputs based on configuration
func (s *ManifestsStrategy) generateOutputs(config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (scorev1b1.ResourceClaimOutputs, error) {
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
