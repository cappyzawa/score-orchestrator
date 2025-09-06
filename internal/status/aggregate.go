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

// AggregateBindingStatuses processes all ResourceBindings and returns aggregated status
func AggregateBindingStatuses(bindings []scorev1b1.ResourceBinding) BindingAggregation {
	if len(bindings) == 0 {
		return BindingAggregation{
			Ready:    false,
			Reason:   conditions.ReasonBindingPending,
			Message:  conditions.MessageNoBindingsFound,
			Bindings: []scorev1b1.BindingSummary{},
		}
	}

	summaries := make([]scorev1b1.BindingSummary, 0, len(bindings))
	var boundCount, failedCount int

	for _, binding := range bindings {
		summary := scorev1b1.BindingSummary{
			Key:              binding.Spec.Key,
			Phase:            binding.Status.Phase,
			Reason:           binding.Status.Reason,
			Message:          binding.Status.Message,
			OutputsAvailable: binding.Status.OutputsAvailable,
		}

		// Handle empty phase as Pending
		if summary.Phase == "" {
			summary.Phase = scorev1b1.ResourceBindingPhasePending
		}

		summaries = append(summaries, summary)

		// Count phases for overall status
		switch binding.Status.Phase {
		case scorev1b1.ResourceBindingPhaseBound:
			if binding.Status.OutputsAvailable {
				boundCount++
			}
		case scorev1b1.ResourceBindingPhaseFailed:
			failedCount++
		}
	}

	// Determine overall binding readiness
	totalBindings := len(bindings)
	var ready bool
	var reason, message string

	if failedCount > 0 {
		ready = false
		reason = conditions.ReasonBindingFailed
		message = conditions.MessageBindingsFailed
	} else if boundCount == totalBindings {
		ready = true
		reason = conditions.ReasonSucceeded
		message = conditions.MessageAllBindingsReady
	} else {
		ready = false
		reason = conditions.ReasonBindingPending
		message = conditions.MessageBindingsProvisioning
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

	// Update BindingsReady condition
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
