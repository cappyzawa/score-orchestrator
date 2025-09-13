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

package selection

// combineLabels combines workload and namespace labels following the normative rule:
// "Selectors are evaluated against the union of Workload.metadata.labels and the target
// Namespace.metadata.labels. If a key exists in both, the Workload label value takes precedence."
func combineLabels(workloadLabels, namespaceLabels map[string]string) map[string]string {
	result := make(map[string]string)

	// First, add all namespace labels
	for key, value := range namespaceLabels {
		result[key] = value
	}

	// Then add workload labels, overriding namespace labels if keys conflict
	for key, value := range workloadLabels {
		result[key] = value
	}

	return result
}
