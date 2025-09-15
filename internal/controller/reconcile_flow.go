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

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
)

// Event types for Kubernetes events
const (
	// EventTypeWarning indicates a warning event
	EventTypeWarning = "Warning"
	// EventTypeNormal indicates a normal event
	EventTypeNormal = "Normal"
)

// ReconcileErrorCategory represents different categories of reconciliation errors
type ReconcileErrorCategory string

const (
	// ErrorCategoryValidation indicates input validation errors
	ErrorCategoryValidation ReconcileErrorCategory = "Validation"
	// ErrorCategoryClaim indicates resource claiming errors
	ErrorCategoryClaim ReconcileErrorCategory = "Claim"
	// ErrorCategoryProjection indicates workload projection errors
	ErrorCategoryProjection ReconcileErrorCategory = "Projection"
	// ErrorCategoryRuntime indicates runtime selection/provisioning errors
	ErrorCategoryRuntime ReconcileErrorCategory = "Runtime"
	// ErrorCategoryConfig indicates configuration loading errors
	ErrorCategoryConfig ReconcileErrorCategory = "Config"
	// ErrorCategoryStatus indicates status update errors
	ErrorCategoryStatus ReconcileErrorCategory = "Status"
)

// ReconcileError represents a structured error with category and event information
type ReconcileError struct {
	Category    ReconcileErrorCategory
	EventReason string
	EventType   string
	Message     string
	Cause       error
}

// Error implements the error interface
func (e *ReconcileError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Category, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Category, e.Message)
}

// Unwrap returns the underlying error
func (e *ReconcileError) Unwrap() error {
	return e.Cause
}

// NewReconcileError creates a new ReconcileError
func NewReconcileError(category ReconcileErrorCategory, eventReason, eventType, message string, cause error) *ReconcileError {
	return &ReconcileError{
		Category:    category,
		EventReason: eventReason,
		EventType:   eventType,
		Message:     message,
		Cause:       cause,
	}
}

// HandleReconcileError handles a ReconcileError by emitting events and returning appropriate ctrl.Result
func HandleReconcileError(ctx context.Context, recorder record.EventRecorder, workload *scorev1b1.Workload, err *ReconcileError) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Emit Kubernetes event
	recorder.Eventf(workload, err.EventType, err.EventReason, "%s", err.Message)

	// Log the error
	log.Error(err, "Reconciliation error", "category", string(err.Category), "reason", err.EventReason)

	// For certain error categories, we might want to requeue
	switch err.Category {
	case ErrorCategoryStatus:
		// Status update conflicts should requeue
		if apierrors.IsConflict(err.Cause) {
			log.V(1).Info("Resource version conflict, requeuing", "error", err.Cause)
			return ctrl.Result{RequeueAfter: ConflictRequeueDelay}, nil
		}
	case ErrorCategoryConfig, ErrorCategoryRuntime:
		// Configuration and runtime errors might be transient
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	// Default: return the error to trigger exponential backoff
	return ctrl.Result{}, err
}

// validateInputsAndPolicy validates workload inputs and applies platform policy
func (r *WorkloadReconciler) validateInputsAndPolicy(_ context.Context, workload *scorev1b1.Workload) (bool, string, string) {
	// For MVP: basic validation (CRD-level validation handles most cases)
	// Resources are optional - workloads can be stateless without dependencies

	// ADR-0003: Policy validation is now handled via Orchestrator Config + Admission
	// For MVP, basic spec validation is sufficient
	return true, conditions.ReasonSucceeded, "Workload specification is valid"
}

// updateStatusAndReturn updates the Workload status and returns the result
func (r *WorkloadReconciler) updateStatusAndReturn(ctx context.Context, workload *scorev1b1.Workload) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, workload); err != nil {
		if apierrors.IsConflict(err) {
			// Resource version conflict - requeue for retry
			ctrl.LoggerFrom(ctx).V(1).Info("Resource version conflict, requeuing", "error", err)
			return ctrl.Result{RequeueAfter: ConflictRequeueDelay}, nil
		}
		ctrl.LoggerFrom(ctx).Error(err, "Failed to update Workload status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// inputsValidToConditionStatus converts boolean to ConditionStatus
func inputsValidToConditionStatus(valid bool) metav1.ConditionStatus {
	if valid {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

// computeFinalStatus updates runtime status and computes Ready condition
func (r *WorkloadReconciler) computeFinalStatus(ctx context.Context, workload *scorev1b1.Workload) {
	// Update RuntimeReady and endpoint (MVP logic)
	r.updateRuntimeStatus(ctx, workload)

	// Compute and set Ready condition
	readyStatus, readyReason, readyMessage := conditions.ComputeReadyCondition(workload.Status.Conditions)
	conditions.SetCondition(&workload.Status.Conditions, conditions.ConditionReady, readyStatus, readyReason, readyMessage)

	if readyStatus == metav1.ConditionTrue {
		r.Recorder.Event(workload, EventTypeNormal, EventReasonReady, "Workload is ready and operational")
	}
}
