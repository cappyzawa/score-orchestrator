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
// for ResourceClaim and WorkloadPlan changes
func EnqueueRequestForOwningWorkload() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		// Try ResourceClaim first
		if claim, ok := obj.(*scorev1b1.ResourceClaim); ok {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      claim.Spec.WorkloadRef.Name,
						Namespace: claim.Spec.WorkloadRef.Namespace,
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

// GetResourceClaimsForWorkload retrieves all ResourceClaims for a given Workload
func GetResourceClaimsForWorkload(ctx context.Context, c client.Client, workload *scorev1b1.Workload) ([]scorev1b1.ResourceClaim, error) {
	claimList := &scorev1b1.ResourceClaimList{}
	key := fmt.Sprintf("%s/%s", workload.Namespace, workload.Name)

	// Try using indexer first, fallback to label selector for tests
	err := c.List(ctx, claimList, client.MatchingFields{meta.IndexResourceClaimByWorkload: key})
	if err != nil && err.Error() == "field label not supported: resourceclaim.workloadRef" {
		// Fallback: use label selector when indexer is not available (e.g., in tests)
		return getResourceClaimsWithoutIndex(ctx, c, workload)
	}
	if err != nil {
		return nil, err
	}

	return claimList.Items, nil
}

// getResourceClaimsWithoutIndex retrieves ResourceClaims using owner reference or labels
func getResourceClaimsWithoutIndex(ctx context.Context, c client.Client, workload *scorev1b1.Workload) ([]scorev1b1.ResourceClaim, error) {
	claimList := &scorev1b1.ResourceClaimList{}

	// Use label selector to find claims for this workload
	err := c.List(ctx, claimList,
		client.InNamespace(workload.Namespace),
		client.MatchingLabels{"score.dev/workload": workload.Name})
	if err != nil {
		return nil, err
	}

	return claimList.Items, nil
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
