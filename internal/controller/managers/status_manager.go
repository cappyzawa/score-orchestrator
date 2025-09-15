package managers

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
)

// Event constants for StatusManager
const (
	EventReasonReady         = "Ready"
	EventReasonNotReady      = "NotReady"
	EventReasonStatusUpdated = "StatusUpdated"
)

// StatusManager handles all Workload status management operations
type StatusManager struct {
	client          client.Client
	scheme          *runtime.Scheme
	recorder        record.EventRecorder
	endpointDeriver *endpoint.EndpointDeriver
}

// NewStatusManager creates a new StatusManager instance
func NewStatusManager(
	c client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	endpointDeriver *endpoint.EndpointDeriver,
) *StatusManager {
	return &StatusManager{
		client:          c,
		scheme:          scheme,
		recorder:        recorder,
		endpointDeriver: endpointDeriver,
	}
}

// UpdateStatus updates the Workload status and handles conflicts
func (sm *StatusManager) UpdateStatus(ctx context.Context, workload *scorev1b1.Workload) error {
	log := ctrl.LoggerFrom(ctx)

	if err := sm.client.Status().Update(ctx, workload); err != nil {
		log.Error(err, "Failed to update Workload status")
		return fmt.Errorf("failed to update workload status: %w", err)
	}

	log.V(1).Info("Successfully updated Workload status")
	return nil
}

// ComputeReadyCondition determines the Ready condition based on other conditions
// Ready = InputsValid ∧ ClaimsReady ∧ RuntimeReady
func (sm *StatusManager) ComputeReadyCondition(conditionsSlice []metav1.Condition) (metav1.ConditionStatus, string, string) {
	return conditions.ComputeReadyCondition(conditionsSlice)
}

// DeriveEndpoint derives the canonical endpoint for a Workload from its WorkloadPlan
func (sm *StatusManager) DeriveEndpoint(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) (*string, error) {
	if sm.endpointDeriver == nil {
		return nil, fmt.Errorf("endpoint deriver not configured")
	}

	derivedEndpoint, err := sm.endpointDeriver.DeriveEndpoint(ctx, workload, plan)
	if err != nil {
		return nil, fmt.Errorf("failed to derive endpoint: %w", err)
	}

	if derivedEndpoint == "" {
		return nil, nil
	}

	return &derivedEndpoint, nil
}

// SetInputsValidCondition sets the InputsValid condition on the workload
func (sm *StatusManager) SetInputsValidCondition(
	workload *scorev1b1.Workload,
	valid bool,
	reason, message string,
) {
	status := metav1.ConditionFalse
	if valid {
		status = metav1.ConditionTrue
	}

	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionInputsValid,
		status,
		reason,
		message,
	)
}

// SetClaimsReadyCondition sets the ClaimsReady condition on the workload
func (sm *StatusManager) SetClaimsReadyCondition(
	workload *scorev1b1.Workload,
	ready bool,
	reason, message string,
) {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}

	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionClaimsReady,
		status,
		reason,
		message,
	)
}

// SetRuntimeReadyCondition sets the RuntimeReady condition on the workload
func (sm *StatusManager) SetRuntimeReadyCondition(
	workload *scorev1b1.Workload,
	ready bool,
	reason, message string,
) {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}

	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionRuntimeReady,
		status,
		reason,
		message,
	)
}

// ComputeFinalStatus updates runtime status and computes Ready condition
func (sm *StatusManager) ComputeFinalStatus(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) error {
	log := ctrl.LoggerFrom(ctx)

	// Update RuntimeReady condition and endpoint based on plan
	sm.updateRuntimeStatusFromPlan(ctx, workload, plan)

	// Compute and set Ready condition
	readyStatus, readyReason, readyMessage := sm.ComputeReadyCondition(workload.Status.Conditions)
	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionReady,
		readyStatus,
		readyReason,
		readyMessage,
	)

	// Emit events based on Ready status
	if readyStatus == metav1.ConditionTrue {
		sm.recorder.Event(workload, "Normal", EventReasonReady, "Workload is ready and operational")
		log.V(1).Info("Workload is ready")
	} else {
		log.V(1).Info("Workload is not ready", "reason", readyReason, "message", readyMessage)
	}

	return nil
}

// updateRuntimeStatusFromPlan updates RuntimeReady condition and endpoint based on WorkloadPlan
func (sm *StatusManager) updateRuntimeStatusFromPlan(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) {
	log := ctrl.LoggerFrom(ctx)

	if plan == nil {
		sm.SetRuntimeReadyCondition(
			workload,
			false,
			conditions.ReasonRuntimeSelecting,
			"Runtime controller is being selected",
		)
		return
	}

	// Derive endpoint from WorkloadPlan
	derivedEndpoint, err := sm.DeriveEndpoint(ctx, workload, plan)
	if err != nil {
		log.Error(err, "Failed to derive endpoint")
		sm.SetRuntimeReadyCondition(
			workload,
			false,
			conditions.ReasonProjectionError,
			fmt.Sprintf("Failed to derive endpoint: %v", err),
		)
		return
	}

	// Update endpoint in status if derived
	if derivedEndpoint != nil && *derivedEndpoint != "" {
		workload.Status.Endpoint = derivedEndpoint
		log.V(1).Info("Derived endpoint", "endpoint", *derivedEndpoint)
	}

	// For MVP, assume runtime is provisioning when plan exists
	// In a full implementation, this would check actual runtime status
	sm.SetRuntimeReadyCondition(
		workload,
		false,
		conditions.ReasonRuntimeProvisioning,
		"Runtime resources are being provisioned",
	)
}
