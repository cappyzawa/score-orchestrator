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
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// UpsertWorkloadPlan creates or updates the WorkloadPlan for the given Workload
func UpsertWorkloadPlan(ctx context.Context, c client.Client, workload *scorev1b1.Workload, bindings []scorev1b1.ResourceBinding, runtimeClass string) error {
	planName := workload.Name // Same name as Workload
	plan := &scorev1b1.WorkloadPlan{}

	// Try to get existing plan
	err := c.Get(ctx, types.NamespacedName{
		Name:      planName,
		Namespace: workload.Namespace,
	}, plan)

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get WorkloadPlan %s: %w", planName, err)
	}

	// Build the desired spec
	desiredSpec := scorev1b1.WorkloadPlanSpec{
		WorkloadRef: scorev1b1.WorkloadPlanWorkloadRef{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		},
		ObservedWorkloadGeneration: workload.Generation,
		RuntimeClass:               runtimeClass,
		Projection:                 buildProjection(workload, bindings),
		Bindings:                   buildPlanBindings(bindings),
	}

	if errors.IsNotFound(err) {
		// Create new plan
		plan = &scorev1b1.WorkloadPlan{
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

// buildProjection creates the projection rules for injecting binding outputs
func buildProjection(_ *scorev1b1.Workload, bindings []scorev1b1.ResourceBinding) scorev1b1.WorkloadProjection {
	projection := scorev1b1.WorkloadProjection{}

	// Build environment variable mappings
	// This is a simplified projection - real implementation would need more sophisticated logic
	for _, binding := range bindings {
		if binding.Status.OutputsAvailable {
			// Create common environment variable mappings
			// This is MVP implementation - more sophisticated mapping would be configurable
			envVar := fmt.Sprintf("%s_URI", strings.ToUpper(binding.Spec.Key))
			projection.Env = append(projection.Env, scorev1b1.EnvMapping{
				Name: envVar,
				From: scorev1b1.FromBindingOutput{
					BindingKey: binding.Spec.Key,
					OutputKey:  "uri", // Common output key
				},
			})
		}
	}

	return projection
}

// buildPlanBindings creates the binding requirements for the runtime
func buildPlanBindings(bindings []scorev1b1.ResourceBinding) []scorev1b1.PlanBinding {
	planBindings := make([]scorev1b1.PlanBinding, 0, len(bindings))

	for _, binding := range bindings {
		planBinding := scorev1b1.PlanBinding{
			Key:  binding.Spec.Key,
			Type: binding.Spec.Type,
		}

		if binding.Spec.Class != nil {
			planBinding.Class = binding.Spec.Class
		}

		planBindings = append(planBindings, planBinding)
	}

	return planBindings
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
	if len(a.Projection.Env) != len(b.Projection.Env) {
		return false
	}
	if len(a.Bindings) != len(b.Bindings) {
		return false
	}

	return true
}
