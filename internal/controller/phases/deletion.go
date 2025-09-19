package phases

import (
	"context"

	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
)

// Note: DefaultRequeueDelay is now configured via ReconcilerConfig

// Event constants for deletion phase
const (
	EventTypeNormal    = "Normal"
	EventReasonDeleted = "Deleted"
)

// DeletionPhase handles workload deletion and cleanup
type DeletionPhase struct{}

// Name returns the name of the deletion phase
func (p *DeletionPhase) Name() string {
	return "Deletion"
}

// Execute performs workload deletion and cleanup
func (p *DeletionPhase) Execute(ctx context.Context, phaseCtx *PhaseContext) PhaseResult {
	log := phaseCtx.Logger.WithValues("phase", p.Name())
	log.V(1).Info("Starting deletion phase")

	if !reconcile.HasFinalizer(phaseCtx.Workload) {
		log.V(1).Info("No finalizer present, deletion complete")
		return PhaseResult{}
	}

	// Wait for ResourceClaims to be cleaned up by their owners (Provisioners)
	claims, err := phaseCtx.ClaimManager.GetClaims(ctx, phaseCtx.Workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims during deletion")
		return PhaseResult{Error: err}
	}

	if len(claims) > 0 {
		log.V(1).Info("Waiting for ResourceClaims to be cleaned up", "count", len(claims))
		requeueDelay := phaseCtx.ReconcilerConfig.Retry.DefaultRequeueDelay
		return PhaseResult{Requeue: true, RequeueAfter: requeueDelay}
	}

	// Remove finalizer
	if err := reconcile.RemoveFinalizer(ctx, phaseCtx.Client, phaseCtx.Workload); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return PhaseResult{Error: err}
	}

	phaseCtx.Recorder.Event(phaseCtx.Workload, EventTypeNormal, EventReasonDeleted, "Workload cleanup completed")
	log.V(1).Info("Deletion phase completed successfully")
	return PhaseResult{}
}

// ShouldSkip determines if deletion phase should be skipped
func (p *DeletionPhase) ShouldSkip(ctx context.Context, phaseCtx *PhaseContext) bool {
	// Deletion phase is only executed when workload is being deleted
	return phaseCtx.Workload.DeletionTimestamp.IsZero()
}
