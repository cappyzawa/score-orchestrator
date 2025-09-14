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
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// composeValues implements the ADR-0003 values composition rule:
// defaults ⊕ normalize(Workload) ⊕ outputs
// where right-hand values win in case of conflicts.
func composeValues(
	defaults *runtime.RawExtension,
	workload *scorev1b1.Workload,
	claims []scorev1b1.ResourceClaim,
) (*runtime.RawExtension, error) {
	// Step 1: Extract defaults as a map
	defaultsMap := make(map[string]interface{})
	if defaults != nil && defaults.Raw != nil {
		if err := json.Unmarshal(defaults.Raw, &defaultsMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal defaults: %w", err)
		}
	}

	// Step 2: Normalize workload to template values
	normalizedMap := normalizeWorkload(workload)

	// Step 3: Extract outputs from resource claims
	outputsMap := extractOutputs(claims)

	// Step 4: Merge maps with right-hand precedence
	result := mergeMaps(defaultsMap, normalizedMap, outputsMap)

	// Convert back to RawExtension
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal composed values: %w", err)
	}

	return &runtime.RawExtension{Raw: resultBytes}, nil
}

// normalizeWorkload converts a Workload specification into template values
func normalizeWorkload(workload *scorev1b1.Workload) map[string]interface{} {
	result := make(map[string]interface{})

	// Normalize containers
	if len(workload.Spec.Containers) > 0 {
		containers := make(map[string]interface{})
		for name, container := range workload.Spec.Containers {
			containerMap := map[string]interface{}{
				"image": container.Image,
			}

			if len(container.Command) > 0 {
				containerMap["command"] = container.Command
			}
			if len(container.Args) > 0 {
				containerMap["args"] = container.Args
			}
			if len(container.Variables) > 0 {
				containerMap["env"] = container.Variables
			}
			if len(container.Files) > 0 {
				containerMap["files"] = container.Files
			}

			containers[name] = containerMap
		}
		result["containers"] = containers
	}

	// Normalize service
	if workload.Spec.Service != nil {
		service := make(map[string]interface{})
		if len(workload.Spec.Service.Ports) > 0 {
			ports := make([]map[string]interface{}, 0, len(workload.Spec.Service.Ports))
			for _, port := range workload.Spec.Service.Ports {
				portMap := map[string]interface{}{
					"port": port.Port,
				}
				if port.Protocol != "" {
					portMap["protocol"] = port.Protocol
				}
				if port.TargetPort != nil {
					portMap["targetPort"] = *port.TargetPort
				}
				ports = append(ports, portMap)
			}
			service["ports"] = ports
		}
		result["service"] = service
	}

	// Normalize resources
	if len(workload.Spec.Resources) > 0 {
		resources := make(map[string]interface{})
		for key, resource := range workload.Spec.Resources {
			resourceMap := map[string]interface{}{
				"type": resource.Type,
			}
			if resource.Class != nil {
				resourceMap["class"] = *resource.Class
			}
			if resource.Params != nil {
				var params interface{}
				if err := json.Unmarshal(resource.Params.Raw, &params); err == nil {
					resourceMap["params"] = params
				}
			}
			resources[key] = resourceMap
		}
		result["resources"] = resources
	}

	// Add workload metadata
	result["name"] = workload.Name
	result["namespace"] = workload.Namespace
	if workload.Labels != nil {
		result["labels"] = workload.Labels
	}
	if workload.Annotations != nil {
		result["annotations"] = workload.Annotations
	}

	return result
}

// extractOutputs extracts outputs from ResourceClaim status and organizes them by resource key
func extractOutputs(claims []scorev1b1.ResourceClaim) map[string]interface{} {
	result := make(map[string]interface{})

	if len(claims) == 0 {
		return result
	}

	resources := make(map[string]interface{})

	for _, claim := range claims {
		if !claim.Status.OutputsAvailable {
			continue
		}

		resourceOutputs := make(map[string]interface{})

		if claim.Status.Outputs.SecretRef != nil {
			resourceOutputs["secretRef"] = map[string]interface{}{
				"name": claim.Status.Outputs.SecretRef.Name,
			}
		}

		if claim.Status.Outputs.ConfigMapRef != nil {
			resourceOutputs["configMapRef"] = map[string]interface{}{
				"name": claim.Status.Outputs.ConfigMapRef.Name,
			}
		}

		if claim.Status.Outputs.URI != nil {
			resourceOutputs["uri"] = *claim.Status.Outputs.URI
		}

		if claim.Status.Outputs.Image != nil {
			resourceOutputs["image"] = *claim.Status.Outputs.Image
		}

		if claim.Status.Outputs.Cert != nil {
			certMap := make(map[string]interface{})
			if claim.Status.Outputs.Cert.SecretName != nil {
				certMap["secretName"] = *claim.Status.Outputs.Cert.SecretName
			}
			if claim.Status.Outputs.Cert.Data != nil {
				certMap["data"] = claim.Status.Outputs.Cert.Data
			}
			resourceOutputs["cert"] = certMap
		}

		if len(resourceOutputs) > 0 {
			resources[claim.Spec.Key] = map[string]interface{}{
				"outputs": resourceOutputs,
			}
		}
	}

	if len(resources) > 0 {
		result["resources"] = resources
	}

	return result
}

// mergeMaps merges multiple maps with right-hand precedence
func mergeMaps(maps ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for _, m := range maps {
		for key, value := range m {
			// Deep merge for nested maps
			if existing, exists := result[key]; exists {
				if existingMap, ok := existing.(map[string]interface{}); ok {
					if valueMap, ok := value.(map[string]interface{}); ok {
						result[key] = mergeMaps(existingMap, valueMap)
						continue
					}
				}
			}
			// For non-map values or when key doesn't exist, right-hand wins
			result[key] = value
		}
	}

	return result
}
