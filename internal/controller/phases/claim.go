package phases

import (
	"context"

	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// Event constants for claim phase
const (
	EventTypeWarning      = "Warning"
	EventReasonClaimError = "ClaimError"
)

// ClaimPhase handles ResourceClaim creation, update, and status aggregation
type ClaimPhase struct{}

// Name returns the name of the claim phase
func (p *ClaimPhase) Name() string {
	return "Claim"
}

// Execute performs ResourceClaim operations
func (p *ClaimPhase) Execute(ctx context.Context, phaseCtx *PhaseContext) PhaseResult {
	log := phaseCtx.Logger.WithValues("phase", p.Name())
	log.V(1).Info("Starting claim phase")

	// Create/update ResourceClaims using ClaimManager
	if err := phaseCtx.ClaimManager.EnsureClaims(ctx, phaseCtx.Workload); err != nil {
		log.Error(err, "Failed to ensure ResourceClaims")
		phaseCtx.Recorder.Eventf(phaseCtx.Workload, EventTypeWarning, EventReasonClaimError, "Failed to create resource claims: %v", err)
		return PhaseResult{Error: err}
	}

	log.V(1).Info("ResourceClaims ensured successfully")

	// Get and aggregate claim statuses using ClaimManager
	claims, err := phaseCtx.ClaimManager.GetClaims(ctx, phaseCtx.Workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims")
		return PhaseResult{Error: err}
	}

	log.V(1).Info("Retrieved ResourceClaims", "count", len(claims))

	// Update context with claim data
	phaseCtx.Claims = claims
	phaseCtx.ClaimAgg = phaseCtx.ClaimManager.AggregateStatus(claims)

	// Update workload status from aggregation
	status.UpdateWorkloadStatusFromAggregation(phaseCtx.Workload, phaseCtx.ClaimAgg)

	log.V(1).Info("Claim phase completed successfully")
	return PhaseResult{}
}

// ShouldSkip determines if claim phase should be skipped
func (p *ClaimPhase) ShouldSkip(ctx context.Context, phaseCtx *PhaseContext) bool {
	// Skip claim phase during deletion
	return !phaseCtx.Workload.DeletionTimestamp.IsZero()
}
