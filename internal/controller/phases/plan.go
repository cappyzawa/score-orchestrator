package phases

import (
	"context"
	"strings"
)

// PlanPhase handles WorkloadPlan creation and updates
type PlanPhase struct{}

// Name returns the name of the plan phase
func (p *PlanPhase) Name() string {
	return "Plan"
}

// Execute performs WorkloadPlan operations
func (p *PlanPhase) Execute(ctx context.Context, phaseCtx *PhaseContext) PhaseResult {
	log := phaseCtx.Logger.WithValues("phase", p.Name())
	log.V(1).Info("Starting plan phase")

	// Ensure WorkloadPlan using PlanManager
	if err := phaseCtx.PlanManager.EnsurePlan(ctx, phaseCtx.Workload, phaseCtx.Claims, phaseCtx.ClaimAgg); err != nil {
		log.Error(err, "Failed to ensure WorkloadPlan")

		// Check if this is a status-only error that should trigger immediate return
		if strings.Contains(err.Error(), "missing required outputs for projection") {
			log.V(1).Info("Missing required outputs for projection, skipping to status update")
			return PhaseResult{Skip: true}
		}

		return PhaseResult{Error: err}
	}

	// Get the current plan for context
	plan, err := phaseCtx.PlanManager.GetPlan(ctx, phaseCtx.Workload)
	if err != nil {
		log.V(1).Info("Failed to get WorkloadPlan, continuing with nil plan", "error", err)
		// This is not a fatal error - nil plan is handled elsewhere
		plan = nil
	}

	// Update context with plan data
	phaseCtx.Plan = plan

	log.V(1).Info("Plan phase completed successfully")
	return PhaseResult{}
}

// ShouldSkip determines if plan phase should be skipped
func (p *PlanPhase) ShouldSkip(ctx context.Context, phaseCtx *PhaseContext) bool {
	// Skip plan phase during deletion
	return !phaseCtx.Workload.DeletionTimestamp.IsZero()
}
