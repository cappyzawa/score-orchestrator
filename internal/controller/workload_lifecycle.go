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
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
)

// Workload lifecycle management constants
const (
	// DefaultRequeueDelay is the default delay for requeuing when waiting for resources during deletion
	DefaultRequeueDelay = 30 * time.Second
)

// Workload lifecycle events
const (
	// EventReasonDeleted indicates successful workload deletion
	EventReasonDeleted = "Deleted"
)

// handleDeletion handles Workload deletion with finalizer cleanup
func (r *WorkloadReconciler) handleDeletion(ctx context.Context, workload *scorev1b1.Workload) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if !reconcile.HasFinalizer(workload) {
		return ctrl.Result{}, nil
	}

	// Wait for ResourceClaims to be cleaned up by their owners (Provisioners)
	claims, err := GetResourceClaimsForWorkload(ctx, r.Client, workload)
	if err != nil {
		log.Error(err, "Failed to get ResourceClaims during deletion")
		return ctrl.Result{}, err
	}

	if len(claims) > 0 {
		log.V(1).Info("Waiting for ResourceClaims to be cleaned up", "count", len(claims))
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	// Remove finalizer
	if err := reconcile.RemoveFinalizer(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(workload, EventTypeNormal, EventReasonDeleted, "Workload cleanup completed")
	return ctrl.Result{}, nil
}
