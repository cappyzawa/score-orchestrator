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

// Reconcile handles Workload reconciliation - the single writer of Workload.status
func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("workload", req.NamespacedName)
	log.V(1).Info("Reconcile called", "namespace", req.Namespace, "name", req.Name)

	// Get the Workload
	workload := &scorev1b1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Workload not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Workload")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Processing Workload", "generation", workload.Generation, "resourceVersion", workload.ResourceVersion)

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
		log.V(1).Info("Inputs validation failed", "reason", reason, "message", message)
		return r.updateStatusAndReturn(ctx, workload)
	}

	log.V(1).Info("Inputs validated successfully, proceeding to resource claims")

	// Create/update ResourceClaims
	if err := reconcile.UpsertResourceClaims(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to upsert ResourceClaims")
		r.Recorder.Eventf(workload, EventTypeWarning, EventReasonBindingError, "Failed to create resource bindings: %v", err)
		return ctrl.Result{}, err
	}

	log.V(1).Info("ResourceClaims upserted successfully")

	// Aggregate binding statuses
	claims, err := GetResourceClaimsForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Retrieved ResourceClaims", "count", len(claims))

	agg := status.AggregateClaimStatuses(claims)
	status.UpdateWorkloadStatusFromAggregation(workload, agg)

	// Create WorkloadPlan if bindings are ready
	if agg.Ready {
		log.V(1).Info("Claims are ready, creating WorkloadPlan")
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
				r.Recorder.Eventf(workload, EventTypeWarning, EventReasonProjectionError, "Missing required resource outputs: %v", err)
				return r.updateStatusAndReturn(ctx, workload)
			}

			r.Recorder.Eventf(workload, EventTypeWarning, EventReasonPlanError, "Failed to create workload plan: %v", err)
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(workload, EventTypeNormal, EventReasonPlanCreated, "WorkloadPlan created successfully")
	} else {
		log.V(1).Info("Claims are not ready yet", "ready", agg.Ready, "reason", agg.Reason, "message", agg.Message)
	}

	// Update RuntimeReady and endpoint (MVP logic)
	r.updateRuntimeStatus(ctx, workload)

	// Compute and set Ready condition
	readyStatus, readyReason, readyMessage := conditions.ComputeReadyCondition(workload.Status.Conditions)
	conditions.SetCondition(&workload.Status.Conditions, conditions.ConditionReady, readyStatus, readyReason, readyMessage)

	if readyStatus == metav1.ConditionTrue {
		r.Recorder.Event(workload, EventTypeNormal, EventReasonReady, "Workload is ready and operational")
	}

	return r.updateStatusAndReturn(ctx, workload)
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
