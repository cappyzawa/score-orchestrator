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

package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
)

// BindingAggregation holds the aggregated binding information
type BindingAggregation struct {
	Ready    bool
	Reason   string
	Message  string
	Bindings []scorev1b1.BindingSummary
}

// AggregateClaimStatuses processes all ResourceClaims and returns aggregated status
func AggregateClaimStatuses(claims []scorev1b1.ResourceClaim) BindingAggregation {
	if len(claims) == 0 {
		return BindingAggregation{
			Ready:    false,
			Reason:   conditions.ReasonBindingPending,
			Message:  conditions.MessageNoClaimsFound,
			Bindings: []scorev1b1.BindingSummary{},
		}
	}

	summaries := make([]scorev1b1.BindingSummary, 0, len(claims))
	var boundCount, failedCount int

	for _, claim := range claims {
		summary := scorev1b1.BindingSummary{
			Key:              claim.Spec.Key,
			Phase:            claim.Status.Phase,
			Reason:           claim.Status.Reason,
			Message:          claim.Status.Message,
			OutputsAvailable: claim.Status.OutputsAvailable,
		}

		// Handle empty phase as Pending
		if summary.Phase == "" {
			summary.Phase = scorev1b1.ResourceClaimPhasePending
		}

		summaries = append(summaries, summary)

		// Count phases for overall status
		switch claim.Status.Phase {
		case scorev1b1.ResourceClaimPhaseBound:
			if claim.Status.OutputsAvailable {
				boundCount++
			}
		case scorev1b1.ResourceClaimPhaseFailed:
			failedCount++
		}
	}

	// Determine overall claim readiness
	totalClaims := len(claims)
	var ready bool
	var reason, message string

	if failedCount > 0 {
		ready = false
		reason = conditions.ReasonBindingFailed
		message = conditions.MessageClaimsFailed
	} else if boundCount == totalClaims {
		ready = true
		reason = conditions.ReasonSucceeded
		message = conditions.MessageAllClaimsReady
	} else {
		ready = false
		reason = conditions.ReasonBindingPending
		message = conditions.MessageClaimsProvisioning
	}

	return BindingAggregation{
		Ready:    ready,
		Reason:   reason,
		Message:  message,
		Bindings: summaries,
	}
}

// UpdateWorkloadStatusFromAggregation updates the Workload status based on binding aggregation
func UpdateWorkloadStatusFromAggregation(workload *scorev1b1.Workload, agg BindingAggregation) {
	// Update bindings summary
	workload.Status.Bindings = agg.Bindings

	// Update ClaimsReady condition
	var status metav1.ConditionStatus
	if agg.Ready {
		status = metav1.ConditionTrue
	} else {
		status = metav1.ConditionFalse
	}

	conditions.SetCondition(&workload.Status.Conditions,
		conditions.ConditionBindingsReady,
		status,
		agg.Reason,
		agg.Message)
}
