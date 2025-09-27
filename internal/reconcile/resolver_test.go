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
	"strings"
	"testing"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

func TestResolveAllPlaceholders(t *testing.T) {
	tests := []struct {
		name        string
		workload    *scorev1b1.Workload
		claims      []scorev1b1.ResourceClaim
		expectedEnv map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "resolve environment variables successfully",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"DATABASE_URL": "${resources.db.uri}",
								"STATIC_VAR":   "static-value",
							},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "db"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: &scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://localhost/testdb"),
						},
					},
				},
			},
			expectedEnv: map[string]string{
				"DATABASE_URL": "postgres://localhost/testdb",
				"STATIC_VAR":   "static-value",
			},
			expectError: false,
		},
		{
			name: "error when resource not available",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"DATABASE_URL": "${resources.missing.uri}",
							},
						},
					},
				},
			},
			claims:      []scorev1b1.ResourceClaim{},
			expectedEnv: nil,
			expectError: true,
			errorMsg:    "resource 'missing' has no outputs available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client for testing
			fakeClient := fake.NewClientBuilder().Build()
			ctx := context.TODO()

			resolvedValues, err := resolveAllPlaceholders(ctx, fakeClient, tt.workload, tt.claims)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resolvedValues == nil {
				t.Errorf("expected resolved values but got nil")
				return
			}

			// Parse the resolved values and check the environment variables
			var values map[string]interface{}
			if err := json.Unmarshal(resolvedValues.Raw, &values); err != nil {
				t.Errorf("failed to unmarshal resolved values: %v", err)
				return
			}

			containers, ok := values["containers"].(map[string]interface{})
			if !ok {
				t.Errorf("expected containers in resolved values")
				return
			}

			appContainer, ok := containers["app"].(map[string]interface{})
			if !ok {
				t.Errorf("expected app container in resolved values")
				return
			}

			env, ok := appContainer["env"].(map[string]interface{})
			if !ok {
				t.Errorf("expected env in app container")
				return
			}

			for expectedKey, expectedValue := range tt.expectedEnv {
				actualValue, exists := env[expectedKey]
				if !exists {
					t.Errorf("expected env var %s not found", expectedKey)
					continue
				}
				if actualValue != expectedValue {
					t.Errorf("expected env var %s to be %q, got %q", expectedKey, expectedValue, actualValue)
				}
			}
		})
	}
}
