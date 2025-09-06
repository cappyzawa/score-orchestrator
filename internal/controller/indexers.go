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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/meta"
)

// SetupIndexers sets up the indexers for efficient lookups
func SetupIndexers(ctx context.Context, mgr manager.Manager) error {
	// Index ResourceBinding by workloadRef
	if err := mgr.GetFieldIndexer().IndexField(ctx, &scorev1b1.ResourceBinding{}, meta.IndexResourceBindingByWorkload,
		func(obj client.Object) []string {
			binding := obj.(*scorev1b1.ResourceBinding)
			return []string{binding.Spec.WorkloadRef.Namespace + "/" + binding.Spec.WorkloadRef.Name}
		},
	); err != nil {
		return err
	}

	// Index WorkloadPlan by workloadRef
	if err := mgr.GetFieldIndexer().IndexField(ctx, &scorev1b1.WorkloadPlan{}, meta.IndexWorkloadPlanByWorkload,
		func(obj client.Object) []string {
			plan := obj.(*scorev1b1.WorkloadPlan)
			return []string{plan.Spec.WorkloadRef.Namespace + "/" + plan.Spec.WorkloadRef.Name}
		},
	); err != nil {
		return err
	}

	return nil
}
