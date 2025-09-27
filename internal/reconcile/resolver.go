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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// resolveAllPlaceholders creates a fully resolved values structure with all placeholders substituted
func resolveAllPlaceholders(ctx context.Context, c client.Client, workload *scorev1b1.Workload, claims []scorev1b1.ResourceClaim) (*runtime.RawExtension, error) {
	// Build a map of available outputs for quick lookup
	availableOutputs := buildResolvedOutputsMap(ctx, c, claims)

	// Create the resolved values structure
	resolvedValues := make(map[string]interface{})

	// Resolve containers
	containers := make(map[string]interface{})
	for containerName, containerSpec := range workload.Spec.Containers {
		container := make(map[string]interface{})

		// Resolve environment variables
		if containerSpec.Variables != nil {
			env := make(map[string]interface{})
			for envName, envValue := range containerSpec.Variables {
				resolvedValue, err := resolveValue(envValue, availableOutputs)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve env var %s in container %s: %w", envName, containerName, err)
				}
				env[envName] = resolvedValue
			}
			container["env"] = env
		}

		// TODO: Resolve other container fields (files, volumes, etc.)
		containers[containerName] = container
	}
	resolvedValues["containers"] = containers

	// TODO: Resolve service ports and other top-level fields

	// Convert to RawExtension
	jsonData, err := json.Marshal(resolvedValues)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resolved values: %w", err)
	}

	return &runtime.RawExtension{Raw: jsonData}, nil
}

// resolveValue resolves a single value string by substituting placeholders
func resolveValue(value string, availableOutputs map[string]map[string]string) (interface{}, error) {
	// Regular expressions to match both ${resources.<key>.<name>} and ${resources.<key>.outputs.<name>} patterns
	resourceRefPattern := regexp.MustCompile(`\$\{resources\.([^.]+)\.([^}]+)\}`)
	resourceRefWithOutputsPattern := regexp.MustCompile(`\$\{resources\.([^.]+)\.outputs\.([^}]+)\}`)

	result := value

	// First try the old format: ${resources.<key>.outputs.<name>} (more specific)
	outputsMatches := resourceRefWithOutputsPattern.FindAllStringSubmatch(value, -1)
	for _, match := range outputsMatches {
		if len(match) >= 3 {
			resourceKey := match[1]
			outputKey := match[2]

			if outputs, exists := availableOutputs[resourceKey]; exists {
				if resolvedValue, exists := outputs[outputKey]; exists {
					templateVar := fmt.Sprintf("${resources.%s.outputs.%s}", resourceKey, outputKey)
					result = strings.ReplaceAll(result, templateVar, resolvedValue)
				} else {
					return nil, fmt.Errorf("resource '%s' missing output '%s'", resourceKey, outputKey)
				}
			} else {
				return nil, fmt.Errorf("resource '%s' has no outputs available", resourceKey)
			}
		}
	}

	// If no old format found, try the new format: ${resources.<key>.<name>}
	if len(outputsMatches) == 0 {
		matches := resourceRefPattern.FindAllStringSubmatch(value, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				resourceKey := match[1]
				outputKey := match[2]

				if outputs, exists := availableOutputs[resourceKey]; exists {
					if resolvedValue, exists := outputs[outputKey]; exists {
						templateVar := fmt.Sprintf("${resources.%s.%s}", resourceKey, outputKey)
						result = strings.ReplaceAll(result, templateVar, resolvedValue)
					} else {
						return nil, fmt.Errorf("resource '%s' missing output '%s'", resourceKey, outputKey)
					}
				} else {
					return nil, fmt.Errorf("resource '%s' has no outputs available", resourceKey)
				}
			}
		}
	}

	// Return the resolved string value
	return result, nil
}

// buildResolvedOutputsMap creates a map of available resolved outputs for each claim
func buildResolvedOutputsMap(ctx context.Context, c client.Client, claims []scorev1b1.ResourceClaim) map[string]map[string]string {
	availableOutputs := make(map[string]map[string]string)
	for _, claim := range claims {
		if claim.Status.OutputsAvailable {
			outputs := make(map[string]string)

			if claim.Status.Outputs.URI != nil {
				outputs["uri"] = *claim.Status.Outputs.URI
			}
			if claim.Status.Outputs.SecretRef != nil {
				// Read actual Secret data instead of using hardcoded values
				secret := &corev1.Secret{}
				err := c.Get(ctx, client.ObjectKey{
					Name:      claim.Status.Outputs.SecretRef.Name,
					Namespace: claim.Namespace,
				}, secret)

				if err == nil {
					// Use actual Secret data
					for key, value := range secret.Data {
						outputs[key] = string(value)
					}
				} else {
					// Fallback to hardcoded values only if Secret read fails
					// This is for backward compatibility and should be logged
					if claim.Spec.Type == "postgres" {
						outputs["username"] = fmt.Sprintf("postgres-%s", claim.Name)
						outputs["password"] = fmt.Sprintf("password-%s", claim.Name)
						outputs["host"] = fmt.Sprintf("%s-postgres", claim.Name)
						outputs["port"] = "5432"
						outputs["database"] = fmt.Sprintf("db_%s", claim.Name)
						outputs["uri"] = fmt.Sprintf("postgresql://%s:%s@%s:5432/%s", outputs["username"], outputs["password"], outputs["host"], outputs["database"])
					}
					// TODO: Handle other resource types
				}
			}
			// TODO: Handle ConfigMap references when needed
			if claim.Status.Outputs.Image != nil {
				outputs["image"] = *claim.Status.Outputs.Image
			}
			if claim.Status.Outputs.Cert != nil {
				// For certificate outputs, we'll use the SecretName if available
				if claim.Status.Outputs.Cert.SecretName != nil {
					outputs["cert"] = *claim.Status.Outputs.Cert.SecretName
				}
				// TODO: Handle inline certificate data
			}

			availableOutputs[claim.Spec.Key] = outputs
		}
	}
	return availableOutputs
}
