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
)

// UpsertResourceClaims creates or updates ResourceClaim resources for each resource in the Workload spec
func UpsertResourceClaims(ctx context.Context, c client.Client, workload *scorev1b1.Workload) error {
	for key, resource := range workload.Spec.Resources {
		if err := upsertResourceClaim(ctx, c, workload, key, resource); err != nil {
			return fmt.Errorf("failed to upsert ResourceClaim for key %q: %w", key, err)
		}
	}
	return nil
}

// upsertResourceClaim creates or updates a single ResourceClaim
func upsertResourceClaim(ctx context.Context, c client.Client, workload *scorev1b1.Workload, key string, resource scorev1b1.ResourceSpec) error {
	claimName := fmt.Sprintf("%s-%s", workload.Name, key)
	claim := &scorev1b1.ResourceClaim{}

	// Try to get existing claim
	err := c.Get(ctx, types.NamespacedName{
		Name:      claimName,
		Namespace: workload.Namespace,
	}, claim)

	if err != nil && !errors.IsNotFound(err) {
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
		if err := controllerutil.SetControllerReference(workload, claim, c.Scheme()); err != nil {
			return fmt.Errorf("failed to set owner reference: %w", err)
		}

		if err := c.Create(ctx, claim); err != nil {
			return fmt.Errorf("failed to create ResourceClaim: %w", err)
		}
	} else {
		// Update existing claim if spec differs
		if !resourceClaimSpecEqual(claim.Spec, desiredSpec) {
			claim.Spec = desiredSpec
			if err := c.Update(ctx, claim); err != nil {
				return fmt.Errorf("failed to update ResourceClaim: %w", err)
			}
		}
	}

	return nil
}

// resourceClaimSpecEqual compares two ResourceClaimSpec structs for equality
func resourceClaimSpecEqual(a, b scorev1b1.ResourceClaimSpec) bool {
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
