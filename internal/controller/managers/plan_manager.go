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

package managers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/config"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
	"github.com/cappyzawa/score-orchestrator/internal/meta"
	"github.com/cappyzawa/score-orchestrator/internal/reconcile"
	"github.com/cappyzawa/score-orchestrator/internal/selection"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// Plan management events
const (
	// EventReasonPlanCreated indicates successful workload plan creation
	EventReasonPlanCreated = "PlanCreated"
	// EventReasonPlanError indicates an error in workload plan creation
	EventReasonPlanError = "PlanError"
	// EventReasonProjectionError indicates an error in workload projection
	EventReasonProjectionError = "ProjectionError"
)

// Event types
const (
	// EventTypeNormal indicates a normal event
	EventTypeNormal = "Normal"
	// EventTypeWarning indicates a warning event
	EventTypeWarning = "Warning"
)

// PlanManager handles WorkloadPlan operations for Workloads
type PlanManager struct {
	client          client.Client
	scheme          *runtime.Scheme
	recorder        record.EventRecorder
	configLoader    config.ConfigLoader
	endpointDeriver *endpoint.EndpointDeriver
	statusManager   *StatusManager
}

// NewPlanManager creates a new PlanManager instance
func NewPlanManager(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder, configLoader config.ConfigLoader, endpointDeriver *endpoint.EndpointDeriver, statusManager *StatusManager) *PlanManager {
	return &PlanManager{
		client:          c,
		scheme:          scheme,
		recorder:        recorder,
		configLoader:    configLoader,
		endpointDeriver: endpointDeriver,
		statusManager:   statusManager,
	}
}

// EnsurePlan creates WorkloadPlan if bindings are ready
func (pm *PlanManager) EnsurePlan(ctx context.Context, workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim, agg status.ClaimAggregation) error {
	log := ctrl.LoggerFrom(ctx)

	// Create WorkloadPlan if claims are ready
	if agg.Ready {
		log.V(1).Info("Claims are ready, creating WorkloadPlan")
		selectedBackend, err := pm.SelectBackend(ctx, workload)
		if err != nil {
			log.Error(err, "Failed to select backend")
			pm.statusManager.SetRuntimeReadyCondition(
				workload,
				false,
				conditions.ReasonRuntimeSelecting,
				fmt.Sprintf("Backend selection failed: %v", err))
			return err
		}

		if err := reconcile.UpsertWorkloadPlan(ctx, pm.client, workload, claims, selectedBackend); err != nil {
			log.Error(err, "Failed to upsert WorkloadPlan")

			// Check if this is a projection error (missing outputs)
			if strings.Contains(err.Error(), "missing required outputs for projection") {
				pm.statusManager.SetRuntimeReadyCondition(
					workload,
					false,
					conditions.ReasonProjectionError,
					fmt.Sprintf("Cannot create plan: %v", err))
				pm.recorder.Eventf(workload, EventTypeWarning, EventReasonProjectionError, "Missing required resource outputs: %v", err)
				return err
			}

			pm.recorder.Eventf(workload, EventTypeWarning, EventReasonPlanError, "Failed to create workload plan: %v", err)
			return err
		}
		pm.recorder.Eventf(workload, EventTypeNormal, EventReasonPlanCreated, "WorkloadPlan created successfully")
	} else {
		log.V(1).Info("Claims are not ready yet", "ready", agg.Ready, "reason", agg.Reason, "message", agg.Message)
	}

	return nil
}

// GetPlan retrieves the WorkloadPlan for a given Workload
func (pm *PlanManager) GetPlan(ctx context.Context, workload *scorev1b1.Workload) (*scorev1b1.WorkloadPlan, error) {
	planList := &scorev1b1.WorkloadPlanList{}
	key := fmt.Sprintf("%s/%s", workload.Namespace, workload.Name)

	// Try using indexer first, fallback to label selector for tests
	err := pm.client.List(ctx, planList, client.MatchingFields{meta.IndexWorkloadPlanByWorkload: key})
	if err != nil && strings.Contains(err.Error(), "no index with name") {
		// Fallback: use label selector when indexer is not available (e.g., in tests)
		err = pm.client.List(ctx, planList,
			client.InNamespace(workload.Namespace),
			client.MatchingLabels{"score.dev/workload": workload.Name})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list WorkloadPlans: %w", err)
	}

	if len(planList.Items) == 0 {
		return nil, errors.NewNotFound(scorev1b1.GroupVersion.WithResource("workloadplans").GroupResource(), workload.Name)
	}

	if len(planList.Items) > 1 {
		return nil, fmt.Errorf("multiple WorkloadPlans found for Workload %s/%s", workload.Namespace, workload.Name)
	}

	return &planList.Items[0], nil
}

// SelectBackend selects the backend for the workload using deterministic profile selection pipeline
func (pm *PlanManager) SelectBackend(ctx context.Context, workload *scorev1b1.Workload) (*selection.SelectedBackend, error) {
	log := ctrl.LoggerFrom(ctx)

	// Load Orchestrator Configuration
	orchestratorConfig, err := pm.configLoader.LoadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load orchestrator config: %w", err)
	}

	// Create ProfileSelector
	selector := selection.NewProfileSelector(orchestratorConfig, pm.client)

	// Select backend using deterministic pipeline
	selectedBackend, err := selector.SelectBackend(ctx, workload)
	if err != nil {
		return nil, fmt.Errorf("failed to select backend: %w", err)
	}

	log.V(1).Info("Selected backend for workload",
		"backend", selectedBackend.BackendID,
		"runtime", selectedBackend.RuntimeClass,
		"template", fmt.Sprintf("%s:%s", selectedBackend.Template.Kind, selectedBackend.Template.Ref))

	return selectedBackend, nil
}
