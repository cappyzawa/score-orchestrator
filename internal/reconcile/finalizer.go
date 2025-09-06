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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/meta"
)

// EnsureFinalizer adds the finalizer to the workload if it doesn't exist
func EnsureFinalizer(ctx context.Context, c client.Client, workload *scorev1b1.Workload) error {
	if controllerutil.ContainsFinalizer(workload, meta.WorkloadFinalizer) {
		return nil
	}

	controllerutil.AddFinalizer(workload, meta.WorkloadFinalizer)
	return c.Update(ctx, workload)
}

// RemoveFinalizer removes the finalizer from the workload
func RemoveFinalizer(ctx context.Context, c client.Client, workload *scorev1b1.Workload) error {
	if !controllerutil.ContainsFinalizer(workload, meta.WorkloadFinalizer) {
		return nil
	}

	controllerutil.RemoveFinalizer(workload, meta.WorkloadFinalizer)
	return c.Update(ctx, workload)
}

// HasFinalizer returns true if the workload has the finalizer
func HasFinalizer(workload *scorev1b1.Workload) bool {
	return controllerutil.ContainsFinalizer(workload, meta.WorkloadFinalizer)
}
