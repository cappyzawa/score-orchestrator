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

	ctrl "sigs.k8s.io/controller-runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// Resource coordination events
const (
	// EventReasonClaimError indicates an error in resource claiming
	EventReasonClaimError = "ClaimError"
)

// handleResourceClaims creates/updates ResourceClaims and aggregates their statuses
func (r *WorkloadReconciler) handleResourceClaims(ctx context.Context, workload *scorev1b1.Workload) ([]scorev1b1.ResourceClaim, status.ClaimAggregation, error) {
	log := ctrl.LoggerFrom(ctx)

	// Create/update ResourceClaims using ClaimManager
	if err := r.ClaimManager.EnsureClaims(ctx, workload); err != nil {
		log.Error(err, "Failed to ensure ResourceClaims")
		r.Recorder.Eventf(workload, EventTypeWarning, EventReasonClaimError, "Failed to create resource claims: %v", err)
		return nil, status.ClaimAggregation{}, err
	}

	log.V(1).Info("ResourceClaims ensured successfully")

	// Get and aggregate claim statuses using ClaimManager
	claims, err := r.ClaimManager.GetClaims(ctx, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims")
		return nil, status.ClaimAggregation{}, err
	}

	log.V(1).Info("Retrieved ResourceClaims", "count", len(claims))

	agg := r.ClaimManager.AggregateStatus(claims)
	status.UpdateWorkloadStatusFromAggregation(workload, agg)

	return claims, agg, nil
}

// handleWorkloadPlan creates WorkloadPlan if bindings are ready
func (r *WorkloadReconciler) handleWorkloadPlan(ctx context.Context, workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim, agg status.ClaimAggregation) error {
	return r.PlanManager.EnsurePlan(ctx, workload, claims, agg)
}
