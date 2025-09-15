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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/config"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
)

// WorkloadReconciler reconciles a Workload object
type WorkloadReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	ConfigLoader    config.ConfigLoader
	EndpointDeriver *endpoint.EndpointDeriver
}

// +kubebuilder:rbac:groups=score.dev,resources=workloads,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=score.dev,resources=workloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims;workloadplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=score.dev,resources=resourceclaims/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile handles Workload reconciliation - the single writer of Workload.status
func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("workload", req.NamespacedName)
	log.V(1).Info("Reconcile called", "namespace", req.Namespace, "name", req.Name)

	// Get the Workload
	workload := &scorev1b1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Workload not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Workload")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Processing Workload", "generation", workload.Generation, "resourceVersion", workload.ResourceVersion)

	// Handle deletion
	if !workload.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, workload)
	}

	// Ensure finalizer
	if err := reconcile.EnsureFinalizer(ctx, r.Client, workload); err != nil {
		log.Error(err, "Failed to ensure finalizer")
		return ctrl.Result{}, err
	}

	// Validate inputs and apply policy
	inputsValid, reason, message := r.validateInputsAndPolicy(ctx, workload)
	conditions.SetCondition(&workload.Status.Conditions,
		conditions.ConditionInputsValid,
		inputsValidToConditionStatus(inputsValid),
		reason, message)

	if !inputsValid {
		log.V(1).Info("Inputs validation failed", "reason", reason, "message", message)
		return r.updateStatusAndReturn(ctx, workload)
	}

	log.V(1).Info("Inputs validated successfully, proceeding to resource claims")

	// Handle ResourceClaims
	claims, agg, err := r.handleResourceClaims(ctx, workload)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Handle WorkloadPlan
	if err := r.handleWorkloadPlan(ctx, workload, claims, agg); err != nil {
		// Check if this is a status-only error that should trigger immediate return
		if strings.Contains(err.Error(), "missing required outputs for projection") {
			return r.updateStatusAndReturn(ctx, workload)
		}
		return ctrl.Result{}, err
	}

	// Compute final status
	r.computeFinalStatus(ctx, workload)

	return r.updateStatusAndReturn(ctx, workload)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scorev1b1.Workload{}).
		Owns(&scorev1b1.ResourceClaim{}).
		Owns(&scorev1b1.WorkloadPlan{}).
		Watches(&scorev1b1.ResourceClaim{}, EnqueueRequestForOwningWorkload()).
		Watches(&scorev1b1.WorkloadPlan{}, EnqueueRequestForOwningWorkload()).
		Named("workload").
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
