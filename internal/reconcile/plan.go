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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/selection"
)

const (
	outputKeyURI = "uri"
)

// UpsertWorkloadPlan creates or updates the WorkloadPlan for the given Workload
func UpsertWorkloadPlan(ctx context.Context, c client.Client, workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim, selectedBackend *selection.SelectedBackend) error {
	if workload.Name == "" {
		return fmt.Errorf("workload name cannot be empty")
	}
	planName := workload.Name // Same name as Workload
	if planName == "" {
		return fmt.Errorf("plan name cannot be empty, workload.Name: %q", workload.Name)
	}
	plan := &scorev1b1.WorkloadPlan{}

	// Try to get existing plan
	getErr := c.Get(ctx, types.NamespacedName{
		Name:      planName,
		Namespace: workload.Namespace,
	}, plan)

	if getErr != nil && !errors.IsNotFound(getErr) {
		return fmt.Errorf("failed to get WorkloadPlan %s: %w", planName, getErr)
	}

	// Check for projection errors (missing outputs)
	if err := validateProjectionRequirements(workload, claims); err != nil {
		return fmt.Errorf("projection validation failed: %w", err)
	}

	// Compose values according to ADR-0003: defaults ⊕ normalize(Workload) ⊕ outputs
	values, err := composeValues(selectedBackend.Template.Values, workload, claims)
	if err != nil {
		return fmt.Errorf("failed to compose template values: %w", err)
	}

	// Build the desired spec
	desiredSpec := scorev1b1.WorkloadPlanSpec{
		WorkloadRef: scorev1b1.WorkloadPlanWorkloadRef{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		},
		ObservedWorkloadGeneration: workload.Generation,
		RuntimeClass:               selectedBackend.RuntimeClass,
		Template:                   &selectedBackend.Template,
		Values:                     values,
		Projection:                 buildProjection(workload, claims),
		Bindings:                   buildPlanBindings(claims),
	}

	if errors.IsNotFound(getErr) {
		// Create new plan
		plan := &scorev1b1.WorkloadPlan{
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
func buildProjection(workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim) scorev1b1.WorkloadProjection {
	projection := scorev1b1.WorkloadProjection{}

	// Build environment variable mappings from resource references in containers
	envMappings := generateEnvMappings(workload, claims)
	projection.Env = envMappings

	// Build volume projections from container volume specifications
	volumeProjections := generateVolumeProjections(workload, claims)
	projection.Volumes = volumeProjections

	// Build file projections from container file specifications
	fileProjections := generateFileProjections(workload, claims)
	projection.Files = fileProjections

	return projection
}

// generateEnvMappings generates environment variable mappings based on Score placeholders in container variables
func generateEnvMappings(workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim) []scorev1b1.EnvMapping {
	var envMappings []scorev1b1.EnvMapping

	// Create a map of available outputs for quick lookup
	availableOutputs := make(map[string]map[string]bool)
	for _, claim := range claims {
		if claim.Status.OutputsAvailable {
			outputs := make(map[string]bool)
			if claim.Status.Outputs.URI != nil {
				outputs[outputKeyURI] = true
			}
			if claim.Status.Outputs.SecretRef != nil {
				outputs["secretRef"] = true
			}
			if claim.Status.Outputs.ConfigMapRef != nil {
				outputs["configMapRef"] = true
			}
			if claim.Status.Outputs.Image != nil {
				outputs["image"] = true
			}
			if claim.Status.Outputs.Cert != nil {
				outputs["cert"] = true
			}
			availableOutputs[claim.Spec.Key] = outputs
		}
	}

	// Regular expression to match ${resources.<key>.outputs.<name>} patterns
	resourceRefPattern := regexp.MustCompile(`\$\{resources\.([^.]+)\.outputs\.([^}]+)\}`)

	// Scan all container environment variables for resource references
	for _, container := range workload.Spec.Containers {
		if container.Variables != nil {
			for envName, envValue := range container.Variables {
				matches := resourceRefPattern.FindAllStringSubmatch(envValue, -1)
				for _, match := range matches {
					if len(match) >= 3 {
						resourceKey := match[1]
						outputKey := match[2]

						// Check if the referenced output is available
						if outputs, exists := availableOutputs[resourceKey]; exists {
							if outputs[outputKey] {
								envMappings = append(envMappings, scorev1b1.EnvMapping{
									Name: envName,
									From: scorev1b1.FromBindingOutput{
										ClaimKey:  resourceKey,
										OutputKey: outputKey,
									},
								})
							}
						}
					}
				}
			}
		}
	}

	// Add default mappings for available outputs that weren't explicitly referenced
	for _, claim := range claims {
		if claim.Status.OutputsAvailable {
			// Create default environment variable for URI if available
			if claim.Status.Outputs.URI != nil {
				envVar := fmt.Sprintf("%s_URI", strings.ToUpper(claim.Spec.Key))
				// Only add if not already mapped
				found := false
				for _, mapping := range envMappings {
					if mapping.From.ClaimKey == claim.Spec.Key && mapping.From.OutputKey == outputKeyURI {
						found = true
						break
					}
				}
				if !found {
					envMappings = append(envMappings, scorev1b1.EnvMapping{
						Name: envVar,
						From: scorev1b1.FromBindingOutput{
							ClaimKey:  claim.Spec.Key,
							OutputKey: outputKeyURI,
						},
					})
				}
			}
		}
	}

	return envMappings
}

// generateVolumeProjections generates volume projections from container file specifications that reference secrets/configmaps
func generateVolumeProjections(workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim) []scorev1b1.VolumeProjection {
	var volumeProjections []scorev1b1.VolumeProjection

	// Create a map of available secret/configmap outputs
	availableRefs := make(map[string][]string)
	for _, claim := range claims {
		if claim.Status.OutputsAvailable {
			var refs []string
			if claim.Status.Outputs.SecretRef != nil {
				refs = append(refs, "secretRef")
			}
			if claim.Status.Outputs.ConfigMapRef != nil {
				refs = append(refs, "configMapRef")
			}
			if len(refs) > 0 {
				availableRefs[claim.Spec.Key] = refs
			}
		}
	}

	// Scan container files for resource references that could be volumes
	for _, container := range workload.Spec.Containers {
		if container.Files != nil {
			for _, file := range container.Files {
				if file.Source != nil {
					// Check if source references a resource output
					resourceRefPattern := regexp.MustCompile(`\$\{resources\.([^.]+)\.outputs\.([^}]+)\}`)
					matches := resourceRefPattern.FindStringSubmatch(file.Source.URI)

					if len(matches) >= 3 {
						resourceKey := matches[1]
						outputKey := matches[2]

						// Check if the referenced output is available and is a volume-like reference
						if refs, exists := availableRefs[resourceKey]; exists {
							for _, ref := range refs {
								if ref == outputKey {
									volumeProjections = append(volumeProjections, scorev1b1.VolumeProjection{
										Name: fmt.Sprintf("%s-%s", resourceKey, outputKey),
										From: &scorev1b1.FromBindingOutput{
											ClaimKey:  resourceKey,
											OutputKey: outputKey,
										},
									})
									break
								}
							}
						}
					}
				}
			}
		}
	}

	return volumeProjections
}

// generateFileProjections generates file projections from container file specifications
func generateFileProjections(workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim) []scorev1b1.FileProjection {
	var fileProjections []scorev1b1.FileProjection

	// Create a map of available cert outputs
	availableCerts := make(map[string]bool)
	for _, claim := range claims {
		if claim.Status.OutputsAvailable && claim.Status.Outputs.Cert != nil {
			availableCerts[claim.Spec.Key] = true
		}
	}

	// Scan container files for resource references
	for _, container := range workload.Spec.Containers {
		if container.Files != nil {
			for _, file := range container.Files {
				if file.Source != nil {
					// Check if source references a certificate resource
					resourceRefPattern := regexp.MustCompile(`\$\{resources\.([^.]+)\.outputs\.cert\}`)
					matches := resourceRefPattern.FindStringSubmatch(file.Source.URI)

					if len(matches) >= 2 {
						resourceKey := matches[1]

						// Check if the referenced cert output is available
						if availableCerts[resourceKey] {
							fileProjections = append(fileProjections, scorev1b1.FileProjection{
								Path: file.Target,
								From: &scorev1b1.FromBindingOutput{
									ClaimKey:  resourceKey,
									OutputKey: "cert",
								},
							})
						}
					}
				}
			}
		}
	}

	return fileProjections
}

// validateProjectionRequirements checks if all required resource outputs are available for projection
func validateProjectionRequirements(workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim) error {
	// Create a map of available outputs
	availableOutputs := make(map[string]map[string]bool)
	for _, claim := range claims {
		if claim.Status.OutputsAvailable {
			outputs := make(map[string]bool)
			if claim.Status.Outputs.URI != nil {
				outputs[outputKeyURI] = true
			}
			if claim.Status.Outputs.SecretRef != nil {
				outputs["secretRef"] = true
			}
			if claim.Status.Outputs.ConfigMapRef != nil {
				outputs["configMapRef"] = true
			}
			if claim.Status.Outputs.Image != nil {
				outputs["image"] = true
			}
			if claim.Status.Outputs.Cert != nil {
				outputs["cert"] = true
			}
			availableOutputs[claim.Spec.Key] = outputs
		}
	}

	// Regular expression to match ${resources.<key>.outputs.<name>} patterns
	resourceRefPattern := regexp.MustCompile(`\$\{resources\.([^.]+)\.outputs\.([^}]+)\}`)

	var missingOutputs []string

	// Check container environment variables
	for containerName, container := range workload.Spec.Containers {
		if container.Variables != nil {
			for envName, envValue := range container.Variables {
				matches := resourceRefPattern.FindAllStringSubmatch(envValue, -1)
				for _, match := range matches {
					if len(match) >= 3 {
						resourceKey := match[1]
						outputKey := match[2]

						// Check if the referenced output is available
						if outputs, exists := availableOutputs[resourceKey]; !exists {
							missingOutputs = append(missingOutputs, fmt.Sprintf("container[%s].env[%s]: resource '%s' has no outputs available", containerName, envName, resourceKey))
						} else if !outputs[outputKey] {
							missingOutputs = append(missingOutputs, fmt.Sprintf("container[%s].env[%s]: resource '%s' missing output '%s'", containerName, envName, resourceKey, outputKey))
						}
					}
				}
			}
		}

		// Container volumes are handled via Files with source references
		// No separate volumes field exists in ContainerSpec

		// Check container files
		if container.Files != nil {
			for i, file := range container.Files {
				if file.Source != nil {
					matches := resourceRefPattern.FindStringSubmatch(file.Source.URI)
					if len(matches) >= 3 {
						resourceKey := matches[1]
						outputKey := matches[2]

						// Check if the referenced output is available
						if outputs, exists := availableOutputs[resourceKey]; !exists {
							missingOutputs = append(missingOutputs, fmt.Sprintf("container[%s].files[%d]: resource '%s' has no outputs available", containerName, i, resourceKey))
						} else if !outputs[outputKey] {
							missingOutputs = append(missingOutputs, fmt.Sprintf("container[%s].files[%d]: resource '%s' missing output '%s'", containerName, i, resourceKey, outputKey))
						}
					}
				}
			}
		}
	}

	if len(missingOutputs) > 0 {
		return fmt.Errorf("missing required outputs for projection: %s", strings.Join(missingOutputs, "; "))
	}

	return nil
}

// buildPlanBindings creates the binding requirements for the runtime
func buildPlanBindings(claims []scorev1b1.ResourceClaim) []scorev1b1.PlanBinding {
	planBindings := make([]scorev1b1.PlanBinding, 0, len(claims))

	for _, claim := range claims {
		planBinding := scorev1b1.PlanBinding{
			Key:  claim.Spec.Key,
			Type: claim.Spec.Type,
		}

		if claim.Spec.Class != nil {
			planBinding.Class = claim.Spec.Class
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
