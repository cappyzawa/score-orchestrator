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
	"strings"
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
	"github.com/cappyzawa/score-orchestrator/internal/config"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
	"github.com/cappyzawa/score-orchestrator/internal/selection"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// WorkloadReconciler reconciles a Workload object
type WorkloadReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	ConfigLoader    config.ConfigLoader
	EndpointDeriver *endpoint.EndpointDeriver
}

// +kubebuilder:rbac:groups=score.dev,resources=workloads,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=workloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims;workloadplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile handles Workload reconciliation - the single writer of Workload.status
func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("workload", req.NamespacedName)
	log.Info("Reconcile called", "namespace", req.Namespace, "name", req.Name)

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

	log.Info("Processing Workload", "generation", workload.Generation, "resourceVersion", workload.ResourceVersion)

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
		log.Info("Inputs validation failed", "reason", reason, "message", message)
		return r.updateStatusAndReturn(ctx, workload)
	}

	log.Info("Inputs validated successfully, proceeding to resource claims")

	// Create/update ResourceClaims
	if err := reconcile.UpsertResourceClaims(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to upsert ResourceClaims")
		r.Recorder.Eventf(workload, "Warning", "BindingError", "Failed to create resource bindings: %v", err)
		return ctrl.Result{}, err
	}

	log.Info("ResourceClaims upserted successfully")

	// Aggregate binding statuses
	claims, err := GetResourceClaimsForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims")
		return ctrl.Result{}, err
	}

	log.Info("Retrieved ResourceClaims", "count", len(claims))

	agg := status.AggregateClaimStatuses(claims)
	status.UpdateWorkloadStatusFromAggregation(workload, agg)

	// Create WorkloadPlan if bindings are ready
	if agg.Ready {
		log.Info("Claims are ready, creating WorkloadPlan")
		selectedBackend, err := r.selectBackend(ctx, workload)
		if err != nil {
			log.Error(err, "Failed to select backend")
			conditions.SetCondition(&workload.Status.Conditions,
				conditions.ConditionRuntimeReady,
				metav1.ConditionFalse,
				conditions.ReasonRuntimeSelecting,
				fmt.Sprintf("Backend selection failed: %v", err))
			return r.updateStatusAndReturn(ctx, workload)
		}

		if err := reconcile.UpsertWorkloadPlan(ctx, r.Client, workload, claims, selectedBackend); err != nil {
			log.Error(err, "Failed to upsert WorkloadPlan")

			// Check if this is a projection error (missing outputs)
			if strings.Contains(err.Error(), "missing required outputs for projection") {
				conditions.SetCondition(&workload.Status.Conditions,
					conditions.ConditionRuntimeReady,
					metav1.ConditionFalse,
					conditions.ReasonProjectionError,
					fmt.Sprintf("Cannot create plan: %v", err))
				r.Recorder.Eventf(workload, "Warning", "ProjectionError", "Missing required resource outputs: %v", err)
				return r.updateStatusAndReturn(ctx, workload)
			}

			r.Recorder.Eventf(workload, "Warning", "PlanError", "Failed to create workload plan: %v", err)
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(workload, "Normal", "PlanCreated", "WorkloadPlan created successfully")
	} else {
		log.Info("Claims are not ready yet", "ready", agg.Ready, "reason", agg.Reason, "message", agg.Message)
	}

	// Update RuntimeReady and endpoint (MVP logic)
	r.updateRuntimeStatus(ctx, workload)

	// Compute and set Ready condition
	readyStatus, readyReason, readyMessage := conditions.ComputeReadyCondition(workload.Status.Conditions)
	conditions.SetCondition(&workload.Status.Conditions, conditions.ConditionReady, readyStatus, readyReason, readyMessage)

	if readyStatus == metav1.ConditionTrue {
		r.Recorder.Event(workload, "Normal", "Ready", "Workload is ready and operational")
	}

	return r.updateStatusAndReturn(ctx, workload)
}

// handleDeletion handles Workload deletion with finalizer cleanup
func (r *WorkloadReconciler) handleDeletion(ctx context.Context, workload *scorev1b1.Workload) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if !reconcile.HasFinalizer(workload) {
		return ctrl.Result{}, nil
	}

	// Wait for ResourceClaims to be cleaned up by their owners (Provisioners)
	claims, err := GetResourceClaimsForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims during deletion")
		return ctrl.Result{}, err
	}

	if len(claims) > 0 {
		log.Info("Waiting for ResourceClaims to be cleaned up", "count", len(claims))
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
func (r *WorkloadReconciler) validateInputsAndPolicy(_ context.Context, workload *scorev1b1.Workload) (bool, string, string) {
	// For MVP: basic validation (CRD-level validation handles most cases)
	if len(workload.Spec.Resources) == 0 {
		return false, conditions.ReasonSpecInvalid, "Workload must define at least one resource"
	}

	// ADR-0003: Policy validation is now handled via Orchestrator Config + Admission
	// For MVP, basic spec validation is sufficient
	return true, conditions.ReasonSucceeded, "Workload specification is valid"
}

// selectBackend selects the backend for the workload using deterministic profile selection pipeline
// ADR-0003: Runtime selection is now based on deterministic profile selection pipeline
func (r *WorkloadReconciler) selectBackend(ctx context.Context, workload *scorev1b1.Workload) (*selection.SelectedBackend, error) {
	log := ctrl.LoggerFrom(ctx)

	// Load Orchestrator Configuration
	orchestratorConfig, err := r.ConfigLoader.LoadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load orchestrator config: %w", err)
	}

	// Create ProfileSelector
	selector := selection.NewProfileSelector(orchestratorConfig, r.Client)

	// Select backend using deterministic pipeline
	selectedBackend, err := selector.SelectBackend(ctx, workload)
	if err != nil {
		return nil, fmt.Errorf("failed to select backend: %w", err)
	}

	log.Info("Selected backend for workload",
		"backend", selectedBackend.BackendID,
		"runtime", selectedBackend.RuntimeClass,
		"template", fmt.Sprintf("%s:%s", selectedBackend.Template.Kind, selectedBackend.Template.Ref))

	return selectedBackend, nil
}

// updateRuntimeStatus updates RuntimeReady condition and endpoint using WorkloadPlan templates
// ADR-0003: Endpoint derivation is now based on WorkloadPlan templates
func (r *WorkloadReconciler) updateRuntimeStatus(ctx context.Context, workload *scorev1b1.Workload) {
	log := ctrl.LoggerFrom(ctx)

	// Check if we have a WorkloadPlan (indicates runtime is being engaged)
	plan, err := GetWorkloadPlanForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get WorkloadPlan")
		conditions.SetCondition(&workload.Status.Conditions,
			conditions.ConditionRuntimeReady,
			metav1.ConditionFalse,
			conditions.ReasonRuntimeSelecting,
			"Failed to retrieve workload plan")
		return
	}

	if plan == nil {
		conditions.SetCondition(&workload.Status.Conditions,
			conditions.ConditionRuntimeReady,
			metav1.ConditionFalse,
			conditions.ReasonRuntimeSelecting,
			"Runtime controller is being selected")
		return
	}

	// Derive endpoint from WorkloadPlan using EndpointDeriver
	derivedEndpoint, err := r.EndpointDeriver.DeriveEndpoint(ctx, workload, plan)
	if err != nil {
		log.Error(err, "Failed to derive endpoint")
		conditions.SetCondition(&workload.Status.Conditions,
			conditions.ConditionRuntimeReady,
			metav1.ConditionFalse,
			conditions.ReasonProjectionError,
			fmt.Sprintf("Failed to derive endpoint: %v", err))
		return
	}

	// Update endpoint in status if derived
	if derivedEndpoint != "" {
		workload.Status.Endpoint = &derivedEndpoint
		log.Info("Derived endpoint", "endpoint", derivedEndpoint)
	}

	// For MVP, assume runtime is provisioning when plan exists
	// In a full implementation, this would check actual runtime status
	conditions.SetCondition(&workload.Status.Conditions,
		conditions.ConditionRuntimeReady,
		metav1.ConditionFalse,
		conditions.ReasonRuntimeProvisioning,
		"Runtime is provisioning workload")

	// TODO: Add logic to detect when runtime is actually ready
	// This would involve checking runtime-specific resources and their status
}

// updateStatusAndReturn updates the Workload status and returns the result
func (r *WorkloadReconciler) updateStatusAndReturn(ctx context.Context, workload *scorev1b1.Workload) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, workload); err != nil {
		if apierrors.IsConflict(err) {
			// Resource version conflict - requeue for retry
			ctrl.LoggerFrom(ctx).V(1).Info("Resource version conflict, requeuing", "error", err)
			return ctrl.Result{RequeueAfter: time.Second}, nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.Workload{}).
		Owns(&scorev1b1.ResourceClaim{}).
		Owns(&scorev1b1.WorkloadPlan{}).
		Watches(&scorev1b1.ResourceClaim{}, EnqueueRequestForOwningWorkload()).
		Watches(&scorev1b1.WorkloadPlan{}, EnqueueRequestForOwningWorkload()).
		Named("workload").
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
