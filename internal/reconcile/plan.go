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

package reconcile

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/projection"
	"github.com/cappyzawa/score-orchestrator/internal/selection"
)

// UpsertWorkloadPlan creates or updates the WorkloadPlan for the given Workload
func UpsertWorkloadPlan(ctx context.Context, c client.Client, workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim, selectedBackend *selection.SelectedBackend) error {
	if workload.Name == "" {
		return fmt.Errorf("workload name cannot be empty")
	}
	planName := workload.Name // Same name as Workload
	if planName == "" {
		return fmt.Errorf("plan name cannot be empty, workload.Name: %q", workload.Name)
	}
	plan := &scorev1b1.WorkloadPlan{}

	// Try to get existing plan
	getErr := c.Get(ctx, types.NamespacedName{
		Name:      planName,
		Namespace: workload.Namespace,
	}, plan)

	if getErr != nil && !errors.IsNotFound(getErr) {
		return fmt.Errorf("failed to get WorkloadPlan %s: %w", planName, getErr)
	}

	// Resolve all placeholders to create final values
	resolvedValues, err := resolveAllPlaceholders(workload, claims)
	if err != nil {
		return fmt.Errorf("failed to resolve placeholders: %w", err)
	}

	// Check for unresolved placeholders before creating the plan
	if hasUnresolved, parseErr := projection.HasUnresolvedPlaceholders(resolvedValues.Raw); hasUnresolved {
		if parseErr != nil {
			return fmt.Errorf("unresolved placeholders (parse error): %w", parseErr)
		}
		return fmt.Errorf("unresolved placeholders found in workload projection")
	}

	// Build the desired spec
	desiredSpec := scorev1b1.WorkloadPlanSpec{
		WorkloadRef: scorev1b1.WorkloadPlanWorkloadRef{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		},
		ObservedWorkloadGeneration: workload.Generation,
		RuntimeClass:               selectedBackend.RuntimeClass,
		Template:                   &selectedBackend.Template,
		ResolvedValues:             resolvedValues,
		Claims:                     buildPlanClaims(claims),
	}

	if errors.IsNotFound(getErr) {
		// Create new plan
		plan := &scorev1b1.WorkloadPlan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      planName,
				Namespace: workload.Namespace,
				Labels: map[string]string{
					"score.dev/workload": workload.Name,
				},
			},
			Spec: desiredSpec,
		}

		// Set owner reference
		if err := controllerutil.SetControllerReference(workload, plan, c.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference: %w", err)
		}

		if err := c.Create(ctx, plan); err != nil {
			if errors.IsAlreadyExists(err) {
				// Plan already exists, try to get it and update if needed
				existingPlan := &scorev1b1.WorkloadPlan{}
				if getErr := c.Get(ctx, types.NamespacedName{
					Name:      planName,
					Namespace: workload.Namespace,
				}, existingPlan); getErr != nil {
					return fmt.Errorf("failed to get existing WorkloadPlan after create conflict: %w", getErr)
				}
				// Update existing plan if spec differs
				if !workloadPlanSpecEqual(existingPlan.Spec, desiredSpec) {
					existingPlan.Spec = desiredSpec
					if updateErr := c.Update(ctx, existingPlan); updateErr != nil {
						return fmt.Errorf("failed to update existing WorkloadPlan: %w", updateErr)
					}
				}
				return nil
			}
			return fmt.Errorf("failed to create WorkloadPlan: %w", err)
		}
	} else {
		// Update existing plan if spec differs
		if !workloadPlanSpecEqual(plan.Spec, desiredSpec) {
			plan.Spec = desiredSpec
			if err := c.Update(ctx, plan); err != nil {
				return fmt.Errorf("failed to update WorkloadPlan: %w", err)
			}
		}
	}

	return nil
}

// buildPlanClaims creates the claim requirements for the runtime
func buildPlanClaims(claims []scorev1b1.ResourceClaim) []scorev1b1.PlanClaim {
	planClaims := make([]scorev1b1.PlanClaim, 0, len(claims))

	for _, claim := range claims {
		planClaim := scorev1b1.PlanClaim{
			Key:  claim.Spec.Key,
			Type: claim.Spec.Type,
		}

		if claim.Spec.Class != nil {
			planClaim.Class = claim.Spec.Class
		}

		planClaims = append(planClaims, planClaim)
	}

	return planClaims
}

// workloadPlanSpecEqual compares two WorkloadPlanSpec structs for equality
func workloadPlanSpecEqual(a, b scorev1b1.WorkloadPlanSpec) bool {
	// Simple comparison for MVP - could be more sophisticated
	if a.WorkloadRef != b.WorkloadRef {
		return false
	}
	if a.ObservedWorkloadGeneration != b.ObservedWorkloadGeneration {
		return false
	}
	if a.RuntimeClass != b.RuntimeClass {
		return false
	}

	// For MVP, we do a simple length check for slices
	// More sophisticated comparison could be added if needed
	if len(a.Claims) != len(b.Claims) {
		return false
	}

	return true
}
