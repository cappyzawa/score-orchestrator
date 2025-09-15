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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
)

// Runtime management constants
const (
	// ConflictRequeueDelay is the delay for requeuing on resource version conflicts
	ConflictRequeueDelay = 1 * time.Second
)

// Runtime management events
const (
	// EventReasonProjectionError indicates an error in workload projection
	EventReasonProjectionError = "ProjectionError"
	// EventReasonReady indicates the workload is ready and operational
	EventReasonReady = "Ready"
)

// updateRuntimeStatus updates RuntimeReady condition and endpoint using WorkloadPlan templates
// ADR-0003: Endpoint derivation is now based on WorkloadPlan templates
func (r *WorkloadReconciler) updateRuntimeStatus(ctx context.Context, workload *scorev1b1.Workload) {
	log := ctrl.LoggerFrom(ctx)

	// Check if we have a WorkloadPlan (indicates runtime is being engaged)
	plan, err := r.PlanManager.GetPlan(ctx, workload)
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
		log.V(1).Info("Derived endpoint", "endpoint", derivedEndpoint)
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
