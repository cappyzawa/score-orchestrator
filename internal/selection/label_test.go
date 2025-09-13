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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCombineLabels(t *testing.T) {
	tests := []struct {
		name            string
		workloadLabels  map[string]string
		namespaceLabels map[string]string
		expected        map[string]string
	}{
		{
			name: "workload labels take precedence over namespace labels",
			workloadLabels: map[string]string{
				"app":         "myapp",
				"environment": "staging", // conflicts with namespace
			},
			namespaceLabels: map[string]string{
				"environment": "production", // should be overridden
				"team":        "backend",
			},
			expected: map[string]string{
				"app":         "myapp",
				"environment": "staging", // workload wins
				"team":        "backend",
			},
		},
		{
			name:           "only namespace labels",
			workloadLabels: nil,
			namespaceLabels: map[string]string{
				"environment": "production",
				"team":        "backend",
			},
			expected: map[string]string{
				"environment": "production",
				"team":        "backend",
			},
		},
		{
			name: "only workload labels",
			workloadLabels: map[string]string{
				"app":     "myapp",
				"version": "1.0.0",
			},
			namespaceLabels: nil,
			expected: map[string]string{
				"app":     "myapp",
				"version": "1.0.0",
			},
		},
		{
			name:            "both nil",
			workloadLabels:  nil,
			namespaceLabels: nil,
			expected:        map[string]string{},
		},
		{
			name:            "empty maps",
			workloadLabels:  map[string]string{},
			namespaceLabels: map[string]string{},
			expected:        map[string]string{},
		},
		{
			name: "no conflicts",
			workloadLabels: map[string]string{
				"app":     "myapp",
				"version": "1.0.0",
			},
			namespaceLabels: map[string]string{
				"environment": "production",
				"team":        "backend",
			},
			expected: map[string]string{
				"app":         "myapp",
				"version":     "1.0.0",
				"environment": "production",
				"team":        "backend",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := combineLabels(tt.workloadLabels, tt.namespaceLabels)
			assert.Equal(t, tt.expected, result)
		})
	}
}
