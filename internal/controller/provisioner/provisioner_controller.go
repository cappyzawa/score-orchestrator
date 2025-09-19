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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/config"
)

// ProvisionerReconciler reconciles ResourceClaim objects using configuration-driven provisioning
type ProvisionerReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	ConfigLoader config.ConfigLoader
}

// Reconcile handles ResourceClaim reconciliation using unified provisioning strategies
func (r *ProvisionerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ResourceClaim
	var claim scorev1b1.ResourceClaim
	if err := r.Get(ctx, req.NamespacedName, &claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling ResourceClaim", "type", claim.Spec.Type, "phase", claim.Status.Phase)

	// Load orchestrator configuration
	orchestratorConfig, err := r.ConfigLoader.Load(ctx)
	if err != nil {
		logger.Error(err, "Failed to load orchestrator configuration")
		r.Recorder.Eventf(&claim, "Warning", "ConfigLoadFailed", "Failed to load configuration: %v", err)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Find provisioner configuration for this resource type
	provisionerConfig, err := r.findProvisionerConfig(orchestratorConfig, claim.Spec.Type)
	if err != nil {
		logger.Error(err, "Failed to find provisioner configuration", "type", claim.Spec.Type)
		r.Recorder.Eventf(&claim, "Warning", "ProvisionerNotFound", "No provisioner found for type %s: %v", claim.Spec.Type, err)
		return r.updateClaimStatus(ctx, &claim, scorev1b1.ResourceClaimPhaseFailed, "ProvisionerNotFound", err.Error())
	}

	// Route to appropriate provisioning strategy
	return r.processWithStrategy(ctx, &claim, provisionerConfig)
}

// findProvisionerConfig finds the provisioner configuration for the given resource type
func (r *ProvisionerReconciler) findProvisionerConfig(orchestratorConfig *scorev1b1.OrchestratorConfig, resourceType string) (*scorev1b1.ProvisionerSpec, error) {
	for _, provisioner := range orchestratorConfig.Spec.Provisioners {
		if provisioner.Type == resourceType {
			return &provisioner, nil
		}
	}
	return nil, fmt.Errorf("no provisioner configuration found for resource type: %s", resourceType)
}

// processWithStrategy processes the claim using the appropriate provisioning strategy
func (r *ProvisionerReconciler) processWithStrategy(ctx context.Context, claim *scorev1b1.ResourceClaim, provisionerSpec *scorev1b1.ProvisionerSpec) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Handle legacy provisioner field (controller-based provisioning)
	if provisionerSpec.Config == nil {
		if provisionerSpec.Provisioner != "" {
			logger.Info("Using legacy controller-based provisioning", "provisioner", provisionerSpec.Provisioner)
			// Legacy provisioning is handled by external controllers
			// We just mark as pending and let the external controller handle it
			return r.updateClaimStatus(ctx, claim, scorev1b1.ResourceClaimPhasePending, "WaitingForExternalController", "Waiting for external provisioner controller")
		}
		return r.updateClaimStatus(ctx, claim, scorev1b1.ResourceClaimPhaseFailed, "InvalidConfig", "Neither config nor provisioner field specified")
	}

	// Handle configuration-driven provisioning
	strategy, err := NewStrategy(provisionerSpec.Config.Strategy)
	if err != nil {
		logger.Error(err, "Failed to create provisioning strategy", "strategy", provisionerSpec.Config.Strategy)
		return r.updateClaimStatus(ctx, claim, scorev1b1.ResourceClaimPhaseFailed, "InvalidStrategy", err.Error())
	}

	// Create template context for variable substitution
	templateCtx := r.createTemplateContext(claim, provisionerSpec)

	// Execute the provisioning strategy
	result, err := strategy.Provision(ctx, r.Client, claim, provisionerSpec.Config, templateCtx)
	if err != nil {
		logger.Error(err, "Provisioning strategy failed", "strategy", provisionerSpec.Config.Strategy)
		r.Recorder.Eventf(claim, "Warning", "ProvisioningFailed", "Provisioning failed: %v", err)
		return r.updateClaimStatus(ctx, claim, scorev1b1.ResourceClaimPhaseFailed, "ProvisioningFailed", err.Error())
	}

	// Update claim status based on result
	return r.handleProvisioningResult(ctx, claim, result)
}

// createTemplateContext creates the template variable context for substitution
func (r *ProvisionerReconciler) createTemplateContext(claim *scorev1b1.ResourceClaim, _ *scorev1b1.ProvisionerSpec) *TemplateContext {
	var params *runtime.RawExtension
	if claim.Spec.Params != nil {
		params = &runtime.RawExtension{Raw: claim.Spec.Params.Raw}
	}

	return &TemplateContext{
		ClaimName: claim.Name,
		ClaimKey:  claim.Spec.Key,
		Namespace: claim.Namespace,
		Type:      claim.Spec.Type,
		Class:     claim.Spec.Class,
		Params:    params,
		// Additional context will be populated by strategies as needed
	}
}

// handleProvisioningResult handles the result from provisioning strategy
func (r *ProvisionerReconciler) handleProvisioningResult(ctx context.Context, claim *scorev1b1.ResourceClaim, result *ProvisioningResult) (ctrl.Result, error) {
	switch result.Phase {
	case scorev1b1.ResourceClaimPhaseClaiming:
		r.Recorder.Event(claim, "Normal", "Claiming", "Resource claiming in progress")
		return r.updateClaimStatus(ctx, claim, result.Phase, result.Reason, result.Message)
	case scorev1b1.ResourceClaimPhaseBound:
		r.Recorder.Event(claim, "Normal", "Bound", "Resource successfully provisioned")
		return r.updateClaimStatusWithOutputs(ctx, claim, result)
	case scorev1b1.ResourceClaimPhaseFailed:
		r.Recorder.Eventf(claim, "Warning", "Failed", "Resource provisioning failed: %s", result.Message)
		return r.updateClaimStatus(ctx, claim, result.Phase, result.Reason, result.Message)
	default:
		// Continue reconciling for pending or unknown states
		return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
	}
}

// updateClaimStatus updates the ResourceClaim status
func (r *ProvisionerReconciler) updateClaimStatus(ctx context.Context, claim *scorev1b1.ResourceClaim, phase scorev1b1.ResourceClaimPhase, reason, message string) (ctrl.Result, error) {
	claim.Status.Phase = phase
	claim.Status.Reason = reason
	claim.Status.Message = message
	claim.Status.OutputsAvailable = false

	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateClaimStatusWithOutputs updates the ResourceClaim status with outputs
func (r *ProvisionerReconciler) updateClaimStatusWithOutputs(ctx context.Context, claim *scorev1b1.ResourceClaim, result *ProvisioningResult) (ctrl.Result, error) {
	claim.Status.Phase = result.Phase
	claim.Status.Reason = result.Reason
	claim.Status.Message = result.Message
	claim.Status.OutputsAvailable = true
	claim.Status.Outputs = result.Outputs

	if err := r.Status().Update(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ProvisionerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.ResourceClaim{}).
		Named("provisioner").
		Complete(r)
}
