package controller

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// ResourceClaim lifecycle constants
const (
	// Reasons for ResourceClaim status (abstract vocabulary from control-plane.md)
	ReasonClaimPending       = "ClaimPending"
	ReasonClaimFailed        = "ClaimFailed"
	ReasonBindingPending     = "BindingPending"
	ReasonBindingFailed      = "BindingFailed"
	ReasonSucceeded          = "Succeeded"
	ReasonQuotaExceeded      = "QuotaExceeded"
	ReasonPermissionDenied   = "PermissionDenied"
	ReasonNetworkUnavailable = "NetworkUnavailable"
	ReasonProjectionError    = "ProjectionError"

	// Finalizer for ResourceClaim cleanup
	ResourceClaimFinalizer = "provisioner.score.dev/finalizer"
)

// ResourceClaimLifecycleManager manages ResourceClaim lifecycle operations
type ResourceClaimLifecycleManager struct{}

// NewResourceClaimLifecycleManager creates a new lifecycle manager
func NewResourceClaimLifecycleManager() *ResourceClaimLifecycleManager {
	return &ResourceClaimLifecycleManager{}
}

// SetPhase updates the ResourceClaim phase with proper timestamps and generation tracking
func (lm *ResourceClaimLifecycleManager) SetPhase(
	claim *scorev1b1.ResourceClaim,
	phase scorev1b1.ResourceClaimPhase,
	reason, message string,
) {
	now := metav1.NewTime(time.Now())

	// Update phase transition time if phase changed
	if claim.Status.Phase != phase {
		claim.Status.LastTransitionTime = &now
	}

	claim.Status.Phase = phase
	claim.Status.Reason = reason
	claim.Status.Message = message
	claim.Status.ObservedGeneration = claim.Generation
}

// SetPending sets the ResourceClaim to Pending phase
func (lm *ResourceClaimLifecycleManager) SetPending(claim *scorev1b1.ResourceClaim, reason, message string) {
	lm.SetPhase(claim, scorev1b1.ResourceClaimPhasePending, reason, message)
	claim.Status.OutputsAvailable = false
	claim.Status.Outputs = nil
}

// SetClaiming sets the ResourceClaim to Claiming phase
func (lm *ResourceClaimLifecycleManager) SetClaiming(claim *scorev1b1.ResourceClaim, reason, message string) {
	lm.SetPhase(claim, scorev1b1.ResourceClaimPhaseClaiming, reason, message)
	claim.Status.OutputsAvailable = false
	claim.Status.Outputs = nil
}

// SetBound sets the ResourceClaim to Bound phase with outputs
func (lm *ResourceClaimLifecycleManager) SetBound(
	claim *scorev1b1.ResourceClaim,
	outputs *scorev1b1.ResourceClaimOutputs,
) {
	lm.SetPhase(claim, scorev1b1.ResourceClaimPhaseBound, ReasonSucceeded, "Resource successfully provisioned")
	claim.Status.Outputs = outputs
	claim.Status.OutputsAvailable = true
}

// SetFailed sets the ResourceClaim to Failed phase
func (lm *ResourceClaimLifecycleManager) SetFailed(claim *scorev1b1.ResourceClaim, reason, message string) {
	lm.SetPhase(claim, scorev1b1.ResourceClaimPhaseFailed, reason, message)
	claim.Status.OutputsAvailable = false
	claim.Status.Outputs = nil
}

// NeedsFinalizer checks if the claim needs a finalizer
func (lm *ResourceClaimLifecycleManager) NeedsFinalizer(claim *scorev1b1.ResourceClaim) bool {
	return !claim.DeletionTimestamp.IsZero() && !lm.HasFinalizer(claim)
}

// HasFinalizer checks if the claim has the provisioner finalizer
func (lm *ResourceClaimLifecycleManager) HasFinalizer(claim *scorev1b1.ResourceClaim) bool {
	for _, finalizer := range claim.Finalizers {
		if finalizer == ResourceClaimFinalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds the provisioner finalizer to the claim
func (lm *ResourceClaimLifecycleManager) AddFinalizer(claim *scorev1b1.ResourceClaim) {
	if !lm.HasFinalizer(claim) {
		claim.Finalizers = append(claim.Finalizers, ResourceClaimFinalizer)
	}
}

// RemoveFinalizer removes the provisioner finalizer from the claim
func (lm *ResourceClaimLifecycleManager) RemoveFinalizer(claim *scorev1b1.ResourceClaim) {
	finalizers := make([]string, 0, len(claim.Finalizers))
	for _, finalizer := range claim.Finalizers {
		if finalizer != ResourceClaimFinalizer {
			finalizers = append(finalizers, finalizer)
		}
	}
	claim.Finalizers = finalizers
}

// IsBeingDeleted checks if the claim is being deleted
func (lm *ResourceClaimLifecycleManager) IsBeingDeleted(claim *scorev1b1.ResourceClaim) bool {
	return !claim.DeletionTimestamp.IsZero()
}

// ShouldReconcile determines if the claim should be reconciled
func (lm *ResourceClaimLifecycleManager) ShouldReconcile(claim *scorev1b1.ResourceClaim) bool {
	// Don't reconcile if being deleted and no finalizer
	if lm.IsBeingDeleted(claim) && !lm.HasFinalizer(claim) {
		return false
	}

	// Don't reconcile if bound and generation hasn't changed
	if claim.Status.Phase == scorev1b1.ResourceClaimPhaseBound {
		if claim.Status.ObservedGeneration == claim.Generation {
			return false
		}
	}

	return true
}

// GetReconcileResult returns appropriate reconcile result based on phase
func (lm *ResourceClaimLifecycleManager) GetReconcileResult(
	ctx context.Context,
	claim *scorev1b1.ResourceClaim,
	err error,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if err != nil {
		log.Error(err, "Reconciliation failed", "phase", claim.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Minute * 2}, err
	}

	switch claim.Status.Phase {
	case scorev1b1.ResourceClaimPhasePending:
		// Requeue quickly for pending claims
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	case scorev1b1.ResourceClaimPhaseClaiming:
		// Requeue moderately for claiming
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	case scorev1b1.ResourceClaimPhaseBound:
		// Requeue slowly for bound claims (for health checks)
		return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
	case scorev1b1.ResourceClaimPhaseFailed:
		// Requeue with backoff for failed claims
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	default:
		return ctrl.Result{}, nil
	}
}
