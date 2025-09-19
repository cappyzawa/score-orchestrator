package phases

import (
	"context"

	"github.com/cappyzawa/score-orchestrator/internal/conditions"
)

// ValidationPhase handles input validation and policy checks
type ValidationPhase struct{}

// Name returns the name of the validation phase
func (p *ValidationPhase) Name() string {
	return "Validation"
}

// Execute performs input validation and policy checks
func (p *ValidationPhase) Execute(ctx context.Context, phaseCtx *PhaseContext) PhaseResult {
	log := phaseCtx.Logger.WithValues("phase", p.Name())
	log.V(1).Info("Starting validation phase")

	// Perform validation logic
	inputsValid, reason, message := p.validateInputsAndPolicy(ctx, phaseCtx)

	// Update context with validation results
	phaseCtx.InputsValid = inputsValid
	phaseCtx.ValidationReason = reason
	phaseCtx.ValidationMessage = message

	// Update status condition
	phaseCtx.StatusManager.SetInputsValidCondition(phaseCtx.Workload, inputsValid, reason, message)

	if !inputsValid {
		log.V(1).Info("Inputs validation failed", "reason", reason, "message", message)
		// Skip remaining phases and update status
		return PhaseResult{Skip: true}
	}

	log.V(1).Info("Inputs validated successfully")
	return PhaseResult{}
}

// ShouldSkip determines if validation phase should be skipped
func (p *ValidationPhase) ShouldSkip(ctx context.Context, phaseCtx *PhaseContext) bool {
	// Validation is always performed for active workloads
	return !phaseCtx.Workload.DeletionTimestamp.IsZero()
}

// validateInputsAndPolicy performs the actual validation logic
// This is extracted to allow for easier testing and future expansion
func (p *ValidationPhase) validateInputsAndPolicy(_ context.Context, phaseCtx *PhaseContext) (bool, string, string) {
	// For MVP: basic validation (CRD-level validation handles most cases)
	// Resources are optional - workloads can be stateless without dependencies

	// ADR-0003: Policy validation is now handled via Orchestrator Config + Admission
	// For MVP, basic spec validation is sufficient
	return true, conditions.ReasonSucceeded, "Workload specification is valid"
}
