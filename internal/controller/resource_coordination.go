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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// Resource coordination events
const (
	// EventReasonClaimError indicates an error in resource claiming
	EventReasonClaimError = "ClaimError"
	// EventReasonPlanCreated indicates successful workload plan creation
	EventReasonPlanCreated = "PlanCreated"
	// EventReasonPlanError indicates an error in workload plan creation
	EventReasonPlanError = "PlanError"
)

// handleResourceClaims creates/updates ResourceClaims and aggregates their statuses
func (r *WorkloadReconciler) handleResourceClaims(ctx context.Context, workload *scorev1b1.Workload) ([]scorev1b1.ResourceClaim, status.ClaimAggregation, error) {
	log := ctrl.LoggerFrom(ctx)

	// Create/update ResourceClaims
	if err := reconcile.UpsertResourceClaims(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to upsert ResourceClaims")
		r.Recorder.Eventf(workload, EventTypeWarning, EventReasonClaimError, "Failed to create resource claims: %v", err)
		return nil, status.ClaimAggregation{}, err
	}

	log.V(1).Info("ResourceClaims upserted successfully")

	// Aggregate claim statuses
	claims, err := GetResourceClaimsForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims")
		return nil, status.ClaimAggregation{}, err
	}

	log.V(1).Info("Retrieved ResourceClaims", "count", len(claims))

	agg := status.AggregateClaimStatuses(claims)
	status.UpdateWorkloadStatusFromAggregation(workload, agg)

	return claims, agg, nil
}

// handleWorkloadPlan creates WorkloadPlan if bindings are ready
func (r *WorkloadReconciler) handleWorkloadPlan(ctx context.Context, workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim, agg status.ClaimAggregation) error {
	log := ctrl.LoggerFrom(ctx)

	// Create WorkloadPlan if claims are ready
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
			return err
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
				return err
			}

			r.Recorder.Eventf(workload, EventTypeWarning, EventReasonPlanError, "Failed to create workload plan: %v", err)
			return err
		}
		r.Recorder.Eventf(workload, EventTypeNormal, EventReasonPlanCreated, "WorkloadPlan created successfully")
	} else {
		log.V(1).Info("Claims are not ready yet", "ready", agg.Ready, "reason", agg.Reason, "message", agg.Message)
	}

	return nil
}
