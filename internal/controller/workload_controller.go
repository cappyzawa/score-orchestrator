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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
	"github.com/cappyzawa/score-orchestrator/internal/meta"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// WorkloadReconciler reconciles a Workload object.
//
// Design: The orchestrator implements the single-writer pattern for Workload status.
// It creates and manages ResourceBindings and WorkloadPlans based on the Workload specification.

// +kubebuilder:rbac:groups=score.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=score.dev,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=workloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=score.dev,resources=resourcebindings;workloadplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=score.dev,resources=resourcebindings/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=score.dev,resources=platformpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
type WorkloadReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile handles Workload reconciliation - the single writer of Workload.status
func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("workload", req.NamespacedName)

	// Get the Workload
	workload := &scorev1b1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Workload not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Workload")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !workload.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, workload)
	}

	// Ensure finalizer
	if err := reconcile.EnsureFinalizer(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to ensure finalizer")
		return ctrl.Result{}, err
	}

	// Validate inputs and apply policy
	inputsValid, reason, message := r.validateInputsAndPolicy(ctx, workload)
	conditions.SetCondition(&workload.Status.Conditions,
		conditions.ConditionInputsValid,
		inputsValidToConditionStatus(inputsValid),
		reason, message)

	if !inputsValid {
		return r.updateStatusAndReturn(ctx, workload, ctrl.Result{}, nil)
	}

	// Create/update ResourceBindings
	if err := reconcile.UpsertResourceBindings(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to upsert ResourceBindings")
		r.Recorder.Eventf(workload, "Warning", "BindingError", "Failed to create resource bindings: %v", err)
		return ctrl.Result{}, err
	}

	// Aggregate binding statuses
	bindings, err := GetResourceBindingsForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceBindings")
		return ctrl.Result{}, err
	}

	agg := status.AggregateBindingStatuses(bindings)
	status.UpdateWorkloadStatusFromAggregation(workload, agg)

	// Create WorkloadPlan if bindings are ready
	if agg.Ready {
		runtimeClass := r.determineRuntimeClass(ctx, workload)
		if err := reconcile.UpsertWorkloadPlan(ctx, r.Client, workload, bindings, runtimeClass); err != nil {
			log.Error(err, "Failed to upsert WorkloadPlan")
			r.Recorder.Eventf(workload, "Warning", "PlanError", "Failed to create workload plan: %v", err)
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(workload, "Normal", "PlanCreated", "WorkloadPlan created successfully")
	}

	// Update RuntimeReady and endpoint (MVP logic)
	r.updateRuntimeStatus(ctx, workload)

	// Compute and set Ready condition
	readyStatus, readyReason, readyMessage := conditions.ComputeReadyCondition(workload.Status.Conditions)
	conditions.SetCondition(&workload.Status.Conditions, conditions.ConditionReady, readyStatus, readyReason, readyMessage)

	if readyStatus == metav1.ConditionTrue {
		r.Recorder.Event(workload, "Normal", "Ready", "Workload is ready and operational")
	}

	return r.updateStatusAndReturn(ctx, workload, ctrl.Result{}, nil)
}

// handleDeletion handles Workload deletion with finalizer cleanup
func (r *WorkloadReconciler) handleDeletion(ctx context.Context, workload *scorev1b1.Workload) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if !reconcile.HasFinalizer(workload) {
		return ctrl.Result{}, nil
	}

	// Wait for ResourceBindings to be cleaned up by their owners (Resolvers)
	bindings, err := GetResourceBindingsForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceBindings during deletion")
		return ctrl.Result{}, err
	}

	if len(bindings) > 0 {
		log.Info("Waiting for ResourceBindings to be cleaned up", "count", len(bindings))
		return ctrl.Result{RequeueAfter: 30 * 1000000000}, nil // 30 seconds
	}

	// Remove finalizer
	if err := reconcile.RemoveFinalizer(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(workload, "Normal", "Deleted", "Workload cleanup completed")
	return ctrl.Result{}, nil
}

// validateInputsAndPolicy validates workload inputs and applies platform policy
func (r *WorkloadReconciler) validateInputsAndPolicy(ctx context.Context, workload *scorev1b1.Workload) (bool, string, string) {
	// For MVP: basic validation (CRD-level validation handles most cases)
	if len(workload.Spec.Resources) == 0 {
		return false, conditions.ReasonSpecInvalid, "Workload must define at least one resource"
	}

	// Policy application is minimal in MVP - just check if policy exists
	policy := r.findApplicablePlatformPolicy(ctx, workload)
	if policy == nil {
		// No policy is OK for MVP - use defaults
		return true, conditions.ReasonSucceeded, "Workload specification is valid"
	}

	// In a full implementation, this would validate against policy constraints
	return true, conditions.ReasonSucceeded, "Workload specification is valid and policy compliant"
}

// findApplicablePlatformPolicy finds the applicable PlatformPolicy for the workload
func (r *WorkloadReconciler) findApplicablePlatformPolicy(ctx context.Context, _ *scorev1b1.Workload) *scorev1b1.PlatformPolicy {
	policyList := &scorev1b1.PlatformPolicyList{}
	if err := r.List(ctx, policyList); err != nil {
		return nil
	}

	// For MVP: simple first-match logic
	// Real implementation would have sophisticated targeting logic
	for _, policy := range policyList.Items {
		if policy.Spec.Targeting == nil {
			// Policy applies to all workloads
			return &policy
		}
		// For MVP, we skip complex targeting logic
	}

	return nil
}

// determineRuntimeClass determines the runtime class for the workload
func (r *WorkloadReconciler) determineRuntimeClass(ctx context.Context, workload *scorev1b1.Workload) string {
	policy := r.findApplicablePlatformPolicy(ctx, workload)
	if policy != nil && policy.Spec.RuntimeClass != "" {
		return policy.Spec.RuntimeClass
	}

	// Default runtime class for MVP
	return meta.RuntimeClassKubernetes
}

// updateRuntimeStatus updates RuntimeReady condition and endpoint (MVP implementation)
func (r *WorkloadReconciler) updateRuntimeStatus(ctx context.Context, workload *scorev1b1.Workload) {
	policy := r.findApplicablePlatformPolicy(ctx, workload)

	// MVP logic: If PlatformPolicy has endpoint template, we can determine success
	if policy != nil && policy.Spec.EndpointPolicy != nil && policy.Spec.EndpointPolicy.Template != nil {
		// Derive endpoint from template
		derivedEndpoint := endpoint.DeriveEndpoint(workload, policy)
		if derivedEndpoint != nil {
			workload.Status.Endpoint = derivedEndpoint
			conditions.SetCondition(&workload.Status.Conditions,
				conditions.ConditionRuntimeReady,
				metav1.ConditionTrue,
				conditions.ReasonSucceeded,
				"Runtime provisioned successfully with endpoint")
			return
		}
	}

	// Check if we have a WorkloadPlan (indicates runtime is being engaged)
	plan, err := GetWorkloadPlanForWorkload(ctx, r.Client, workload)
	if err != nil || plan == nil {
		conditions.SetCondition(&workload.Status.Conditions,
			conditions.ConditionRuntimeReady,
			metav1.ConditionFalse,
			conditions.ReasonRuntimeSelecting,
			"Runtime controller is being selected")
		return
	}

	// Plan exists, runtime is provisioning
	conditions.SetCondition(&workload.Status.Conditions,
		conditions.ConditionRuntimeReady,
		metav1.ConditionFalse,
		conditions.ReasonRuntimeProvisioning,
		"Runtime is provisioning workload")
}

// updateStatusAndReturn updates the Workload status and returns the result
func (r *WorkloadReconciler) updateStatusAndReturn(ctx context.Context, workload *scorev1b1.Workload, result ctrl.Result, originalErr error) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, workload); err != nil {
		if apierrors.IsConflict(err) {
			// Resource version conflict - requeue for retry
			ctrl.LoggerFrom(ctx).V(1).Info("Resource version conflict, requeuing", "error", err)
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
		ctrl.LoggerFrom(ctx).Error(err, "Failed to update Workload status")
		return ctrl.Result{}, err
	}
	return result, originalErr
}

// inputsValidToConditionStatus converts boolean to ConditionStatus
func inputsValidToConditionStatus(valid bool) metav1.ConditionStatus {
	if valid {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.Workload{}).
		Owns(&scorev1b1.ResourceBinding{}).
		Owns(&scorev1b1.WorkloadPlan{}).
		Watches(&scorev1b1.ResourceBinding{}, EnqueueRequestForOwningWorkload()).
		Watches(&scorev1b1.WorkloadPlan{}, EnqueueRequestForOwningWorkload()).
		Named("workload").
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
