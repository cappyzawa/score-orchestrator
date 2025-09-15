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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/meta"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// ClaimManager handles ResourceClaim operations for Workloads
type ClaimManager struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// NewClaimManager creates a new ClaimManager instance
func NewClaimManager(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder) *ClaimManager {
	return &ClaimManager{
		client:   c,
		scheme:   scheme,
		recorder: recorder,
	}
}

// EnsureClaims creates or updates ResourceClaim resources for each resource in the Workload spec
func (cm *ClaimManager) EnsureClaims(ctx context.Context, workload *scorev1b1.Workload) error {
	for key, resource := range workload.Spec.Resources {
		if err := cm.upsertResourceClaim(ctx, workload, key, resource); err != nil {
			return fmt.Errorf("failed to upsert ResourceClaim for key %q: %w", key, err)
		}
	}
	return nil
}

// upsertResourceClaim creates or updates a single ResourceClaim
func (cm *ClaimManager) upsertResourceClaim(ctx context.Context, workload *scorev1b1.Workload, key string, resource scorev1b1.ResourceSpec) error {
	claimName := fmt.Sprintf("%s-%s", workload.Name, key)
	claim := &scorev1b1.ResourceClaim{}

	log := ctrl.LoggerFrom(ctx).WithValues("claimName", claimName, "workload", workload.Name, "key", key)
	log.Info("Upserting ResourceClaim")

	// Try to get existing claim
	err := cm.client.Get(ctx, types.NamespacedName{
		Name:      claimName,
		Namespace: workload.Namespace,
	}, claim)

	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to get ResourceClaim")
		return fmt.Errorf("failed to get ResourceClaim %s: %w", claimName, err)
	}

	// Prepare the desired spec
	desiredSpec := scorev1b1.ResourceClaimSpec{
		WorkloadRef: scorev1b1.NamespacedName{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		},
		Key:  key,
		Type: resource.Type,
	}

	// Set optional fields if present
	if resource.Class != nil {
		desiredSpec.Class = resource.Class
	}
	if resource.Params != nil {
		desiredSpec.Params = resource.Params
	}

	if errors.IsNotFound(err) {
		log.Info("Creating new ResourceClaim")
		// Create new claim
		claim = &scorev1b1.ResourceClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: workload.Namespace,
				Labels: map[string]string{
					"score.dev/workload": workload.Name,
					"score.dev/key":      key,
				},
			},
			Spec: desiredSpec,
		}

		// Set owner reference
		if err := controllerutil.SetControllerReference(workload, claim, cm.scheme); err != nil {
			log.Error(err, "Failed to set owner reference")
			return fmt.Errorf("failed to set owner reference: %w", err)
		}

		if err := cm.client.Create(ctx, claim); err != nil {
			log.Error(err, "Failed to create ResourceClaim")
			return fmt.Errorf("failed to create ResourceClaim: %w", err)
		}
		log.Info("Successfully created ResourceClaim")
	} else {
		log.Info("ResourceClaim already exists, checking for updates")
		// Update existing claim if spec differs
		if !cm.resourceClaimSpecEqual(claim.Spec, desiredSpec) {
			log.Info("Updating ResourceClaim spec")
			claim.Spec = desiredSpec
			if err := cm.client.Update(ctx, claim); err != nil {
				log.Error(err, "Failed to update ResourceClaim")
				return fmt.Errorf("failed to update ResourceClaim: %w", err)
			}
			log.Info("Successfully updated ResourceClaim")
		} else {
			log.Info("ResourceClaim spec is up to date")
		}
	}

	return nil
}

// resourceClaimSpecEqual compares two ResourceClaimSpec structs for equality
func (cm *ClaimManager) resourceClaimSpecEqual(a, b scorev1b1.ResourceClaimSpec) bool {
	if a.WorkloadRef != b.WorkloadRef || a.Key != b.Key || a.Type != b.Type {
		return false
	}

	// Compare optional string pointers
	if (a.Class == nil) != (b.Class == nil) {
		return false
	}
	if a.Class != nil && *a.Class != *b.Class {
		return false
	}

	if (a.ID == nil) != (b.ID == nil) {
		return false
	}
	if a.ID != nil && *a.ID != *b.ID {
		return false
	}

	// For Params (JSON), we do a simple nil check
	// More sophisticated comparison could be added if needed
	if (a.Params == nil) != (b.Params == nil) {
		return false
	}

	return true
}

// GetClaims retrieves all ResourceClaims for a given Workload
func (cm *ClaimManager) GetClaims(ctx context.Context, workload *scorev1b1.Workload) ([]scorev1b1.ResourceClaim, error) {
	claimList := &scorev1b1.ResourceClaimList{}
	key := fmt.Sprintf("%s/%s", workload.Namespace, workload.Name)

	// Try using indexer first, fallback to label selector for tests
	err := cm.client.List(ctx, claimList, client.MatchingFields{meta.IndexResourceClaimByWorkload: key})
	if err != nil && (err.Error() == "field label not supported: resourceclaim.workloadRef" ||
		strings.Contains(err.Error(), "no index with name resourceclaim.workloadRef has been registered")) {
		// Fallback: use label selector when indexer is not available (e.g., in tests)
		return cm.getResourceClaimsWithoutIndex(ctx, workload)
	}
	if err != nil {
		return nil, err
	}

	return claimList.Items, nil
}

// getResourceClaimsWithoutIndex retrieves ResourceClaims using owner reference or labels
func (cm *ClaimManager) getResourceClaimsWithoutIndex(ctx context.Context, workload *scorev1b1.Workload) ([]scorev1b1.ResourceClaim, error) {
	claimList := &scorev1b1.ResourceClaimList{}

	// Use label selector to find claims for this workload
	err := cm.client.List(ctx, claimList,
		client.InNamespace(workload.Namespace),
		client.MatchingLabels{"score.dev/workload": workload.Name})
	if err != nil {
		return nil, err
	}

	return claimList.Items, nil
}

// AggregateStatus processes all ResourceClaims and returns aggregated status
func (cm *ClaimManager) AggregateStatus(claims []scorev1b1.ResourceClaim) status.ClaimAggregation {
	if len(claims) == 0 {
		return status.ClaimAggregation{
			Ready:   false,
			Reason:  conditions.ReasonClaimPending,
			Message: conditions.MessageNoClaimsFound,
			Claims:  []scorev1b1.ClaimSummary{},
		}
	}

	summaries := make([]scorev1b1.ClaimSummary, 0, len(claims))
	var boundCount, failedCount int

	for _, claim := range claims {
		summary := scorev1b1.ClaimSummary{
			Key:              claim.Spec.Key,
			Phase:            claim.Status.Phase,
			Reason:           claim.Status.Reason,
			Message:          claim.Status.Message,
			OutputsAvailable: claim.Status.OutputsAvailable,
		}

		// Handle empty phase as Pending
		if summary.Phase == "" {
			summary.Phase = scorev1b1.ResourceClaimPhasePending
		}

		summaries = append(summaries, summary)

		// Count phases for overall status
		switch claim.Status.Phase {
		case scorev1b1.ResourceClaimPhaseBound:
			if claim.Status.OutputsAvailable {
				boundCount++
			}
		case scorev1b1.ResourceClaimPhaseFailed:
			failedCount++
		}
	}

	// Determine overall claim readiness
	totalClaims := len(claims)
	var ready bool
	var reason, message string

	if failedCount > 0 {
		ready = false
		reason = conditions.ReasonClaimFailed
		message = conditions.MessageClaimsFailed
	} else if boundCount == totalClaims {
		ready = true
		reason = conditions.ReasonSucceeded
		message = conditions.MessageAllClaimsReady
	} else {
		ready = false
		reason = conditions.ReasonClaimPending
		message = conditions.MessageClaimsProvisioning
	}

	return status.ClaimAggregation{
		Ready:   ready,
		Reason:  reason,
		Message: message,
		Claims:  summaries,
	}
}
