package phases

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ConflictRequeueDelay is the delay for requeuing on resource version conflicts
const ConflictRequeueDelay = 1 * time.Second

// StatusPhase handles final status computation and updates
type StatusPhase struct{}

// Name returns the name of the status phase
func (p *StatusPhase) Name() string {
	return "Status"
}

// Execute performs final status computation and update
func (p *StatusPhase) Execute(ctx context.Context, phaseCtx *PhaseContext) PhaseResult {
	log := phaseCtx.Logger.WithValues("phase", p.Name())
	log.V(1).Info("Starting status phase")

	// Compute final status using StatusManager
	if err := phaseCtx.StatusManager.ComputeFinalStatus(ctx, phaseCtx.Workload, phaseCtx.Plan); err != nil {
		log.Error(err, "Failed to compute final status")
		return PhaseResult{Error: err}
	}

	// Update status
	if err := phaseCtx.StatusManager.UpdateStatus(ctx, phaseCtx.Workload); err != nil {
		if apierrors.IsConflict(err) {
			// Resource version conflict - requeue for retry
			log.V(1).Info("Resource version conflict, requeuing", "error", err)
			return PhaseResult{Requeue: true, RequeueAfter: ConflictRequeueDelay}
		}
		log.Error(err, "Failed to update Workload status")
		return PhaseResult{Error: err}
	}

	log.V(1).Info("Status phase completed successfully")
	return PhaseResult{}
}

// ShouldSkip determines if status phase should be skipped
func (p *StatusPhase) ShouldSkip(ctx context.Context, phaseCtx *PhaseContext) bool {
	// Status phase is always executed for active workloads
	return !phaseCtx.Workload.DeletionTimestamp.IsZero()
}
