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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/meta"
)

// EnqueueRequestForOwningWorkload returns a handler that enqueues the owner Workload
// for ResourceBinding and WorkloadPlan changes
func EnqueueRequestForOwningWorkload() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		// Try ResourceBinding first
		if binding, ok := obj.(*scorev1b1.ResourceBinding); ok {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      binding.Spec.WorkloadRef.Name,
						Namespace: binding.Spec.WorkloadRef.Namespace,
					},
				},
			}
		}

		// Try WorkloadPlan next
		if plan, ok := obj.(*scorev1b1.WorkloadPlan); ok {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      plan.Spec.WorkloadRef.Name,
						Namespace: plan.Spec.WorkloadRef.Namespace,
					},
				},
			}
		}

		// Unknown type - should not happen if used correctly
		return []reconcile.Request{}
	})
}

// GetResourceBindingsForWorkload retrieves all ResourceBindings for a given Workload
func GetResourceBindingsForWorkload(ctx context.Context, c client.Client, workload *scorev1b1.Workload) ([]scorev1b1.ResourceBinding, error) {
	bindingList := &scorev1b1.ResourceBindingList{}
	key := fmt.Sprintf("%s/%s", workload.Namespace, workload.Name)

	// Try using indexer first, fallback to label selector for tests
	err := c.List(ctx, bindingList, client.MatchingFields{meta.IndexResourceBindingByWorkload: key})
	if err != nil && err.Error() == "field label not supported: resourcebinding.workloadRef" {
		// Fallback: use label selector when indexer is not available (e.g., in tests)
		return getResourceBindingsWithoutIndex(ctx, c, workload)
	}
	if err != nil {
		return nil, err
	}

	return bindingList.Items, nil
}

// getResourceBindingsWithoutIndex retrieves ResourceBindings using owner reference or labels
func getResourceBindingsWithoutIndex(ctx context.Context, c client.Client, workload *scorev1b1.Workload) ([]scorev1b1.ResourceBinding, error) {
	bindingList := &scorev1b1.ResourceBindingList{}

	// Use label selector to find bindings for this workload
	err := c.List(ctx, bindingList,
		client.InNamespace(workload.Namespace),
		client.MatchingLabels{"score.dev/workload": workload.Name})
	if err != nil {
		return nil, err
	}

	return bindingList.Items, nil
}

// GetWorkloadPlanForWorkload retrieves the WorkloadPlan for a given Workload
func GetWorkloadPlanForWorkload(ctx context.Context, c client.Client, workload *scorev1b1.Workload) (*scorev1b1.WorkloadPlan, error) {
	planList := &scorev1b1.WorkloadPlanList{}
	key := fmt.Sprintf("%s/%s", workload.Namespace, workload.Name)

	// Try using indexer first, fallback to label selector for tests
	err := c.List(ctx, planList, client.MatchingFields{meta.IndexWorkloadPlanByWorkload: key})
	if err != nil && err.Error() == "field label not supported: workloadplan.workloadRef" {
		// Fallback: use label selector when indexer is not available (e.g., in tests)
		err = c.List(ctx, planList,
			client.InNamespace(workload.Namespace),
			client.MatchingLabels{"score.dev/workload": workload.Name})
	}
	if err != nil {
		return nil, err
	}

	if len(planList.Items) == 0 {
		return nil, nil // Not found
	}

	if len(planList.Items) > 1 {
		return nil, fmt.Errorf("multiple WorkloadPlans found for Workload %s/%s", workload.Namespace, workload.Name)
	}

	return &planList.Items[0], nil
}
