package phases

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
)

// Note: DefaultRequeueDelay is now configured via ReconcilerConfig

// Event constants for deletion phase
const (
	EventTypeNormal    = "Normal"
	EventReasonDeleted = "Deleted"
)

// DeprovisionPolicy constants for deletion handling
const (
	DeprovisionPolicyDelete = "Delete"
	DeprovisionPolicyRetain = "Retain"
	DeprovisionPolicyOrphan = "Orphan"
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

	// Get all ResourceClaims for this Workload
	claims, err := phaseCtx.ClaimManager.GetClaims(ctx, phaseCtx.Workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims during deletion")
		return PhaseResult{Error: err}
	}

	// Process each claim according to its DeprovisionPolicy
	for _, claim := range claims {
		claimLog := log.WithValues("claim", claim.Name, "claimNamespace", claim.Namespace)

		// Apply DeprovisionPolicy-specific behavior
		if err := p.processDeprovisionPolicy(ctx, phaseCtx, &claim, claimLog); err != nil {
			claimLog.Error(err, "Failed to process DeprovisionPolicy")
			return PhaseResult{Error: err}
		}
	}

	// Wait for ResourceClaims to be cleaned up by their owners (Provisioners)
	// Only wait for claims that should be deleted according to their policy
	remainingClaims, err := phaseCtx.ClaimManager.GetClaims(ctx, phaseCtx.Workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims during deletion check")
		return PhaseResult{Error: err}
	}

	claimsToWaitFor := 0
	for _, claim := range remainingClaims {
		// Count claims that need to be deleted according to their policy
		policy := p.getDeprovisionPolicy(&claim)
		if policy == DeprovisionPolicyDelete {
			claimsToWaitFor++
		}
	}

	if claimsToWaitFor > 0 {
		log.V(1).Info("Waiting for ResourceClaims to be cleaned up", "count", claimsToWaitFor)
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

// processDeprovisionPolicy handles ResourceClaim lifecycle according to its DeprovisionPolicy
func (p *DeletionPhase) processDeprovisionPolicy(ctx context.Context, phaseCtx *PhaseContext, claim *scorev1b1.ResourceClaim, log logr.Logger) error {
	policy := p.getDeprovisionPolicy(claim)

	switch policy {
	case DeprovisionPolicyDelete:
		// Default behavior: let owner reference handle deletion
		log.V(1).Info("Processing claim with Delete policy - letting owner reference handle deletion")
		return nil

	case DeprovisionPolicyRetain:
		// Remove owner reference to retain the claim but detach from workload
		log.V(1).Info("Processing claim with Retain policy - removing owner reference")
		return p.removeOwnerReference(ctx, phaseCtx, claim)

	case DeprovisionPolicyOrphan:
		// Leave claim as-is, remove owner reference
		log.V(1).Info("Processing claim with Orphan policy - removing owner reference")
		return p.removeOwnerReference(ctx, phaseCtx, claim)

	default:
		// Default to Delete policy if not specified
		log.V(1).Info("Processing claim with default Delete policy")
		return nil
	}
}

// getDeprovisionPolicy returns the effective DeprovisionPolicy for a claim
func (p *DeletionPhase) getDeprovisionPolicy(claim *scorev1b1.ResourceClaim) string {
	if claim.Spec.DeprovisionPolicy != nil {
		return string(*claim.Spec.DeprovisionPolicy)
	}
	// Default to Delete if not specified
	return DeprovisionPolicyDelete
}

// removeOwnerReference removes the Workload owner reference from a ResourceClaim
func (p *DeletionPhase) removeOwnerReference(ctx context.Context, phaseCtx *PhaseContext, claim *scorev1b1.ResourceClaim) error {
	// Create a copy to modify
	claimCopy := claim.DeepCopy()

	// Filter out the Workload owner reference
	var newOwnerRefs []metav1.OwnerReference
	for _, ownerRef := range claimCopy.OwnerReferences {
		if ownerRef.Kind != "Workload" || ownerRef.Name != phaseCtx.Workload.Name {
			newOwnerRefs = append(newOwnerRefs, ownerRef)
		}
	}

	claimCopy.OwnerReferences = newOwnerRefs

	// Update the claim
	return phaseCtx.Client.Update(ctx, claimCopy)
}

// ShouldSkip determines if deletion phase should be skipped
func (p *DeletionPhase) ShouldSkip(ctx context.Context, phaseCtx *PhaseContext) bool {
	// Deletion phase is only executed when workload is being deleted
	return phaseCtx.Workload.DeletionTimestamp.IsZero()
}
