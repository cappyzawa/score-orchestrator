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
	"testing"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

func TestComposeValues(t *testing.T) {
	tests := []struct {
		name        string
		defaults    *runtime.RawExtension
		workload    *scorev1b1.Workload
		claims      []scorev1b1.ResourceClaim
		expected    map[string]interface{}
		expectError bool
	}{
		{
			name: "basic composition with all sources",
			defaults: &runtime.RawExtension{
				Raw: []byte(`{"replicas": 1, "resources": {"cpu": "100m"}}`),
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {
							Image: "nginx:latest",
							Variables: map[string]string{
								"PORT": "8080",
							},
						},
					},
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{Port: 80},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{
						Key: "database",
					},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://localhost:5432/testdb"),
						},
					},
				},
			},
			expected: map[string]interface{}{
				"replicas": float64(1), // JSON numbers are float64
				"resources": map[string]interface{}{
					"cpu": "100m",
					"database": map[string]interface{}{
						"outputs": map[string]interface{}{
							"uri": "postgres://localhost:5432/testdb",
						},
					},
				},
				"name":      "test-app",
				"namespace": "default",
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image": "nginx:latest",
						"env": map[string]interface{}{
							"PORT": "8080",
						},
					},
				},
				"service": map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port": float64(80),
						},
					},
				},
			},
		},
		{
			name:     "nil defaults",
			defaults: nil,
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {Image: "nginx"},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{},
			expected: map[string]interface{}{
				"name":      "test-app",
				"namespace": "",
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image": "nginx",
					},
				},
			},
		},
		{
			name: "empty defaults raw",
			defaults: &runtime.RawExtension{
				Raw: nil,
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {Image: "nginx"},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{},
			expected: map[string]interface{}{
				"name":      "test-app",
				"namespace": "",
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image": "nginx",
					},
				},
			},
		},
		{
			name: "right-hand precedence - outputs override defaults",
			defaults: &runtime.RawExtension{
				Raw: []byte(`{"database_url": "default://localhost"}`),
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {Image: "nginx"},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{
						Key: "db",
					},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://prod:5432/mydb"),
						},
					},
				},
			},
			expected: map[string]interface{}{
				"database_url": "default://localhost", // from defaults
				"name":         "test-app",            // from workload
				"namespace":    "",                    // from workload
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image": "nginx",
					},
				},
				"resources": map[string]interface{}{
					"db": map[string]interface{}{
						"outputs": map[string]interface{}{
							"uri": "postgres://prod:5432/mydb", // from outputs
						},
					},
				},
			},
		},
		{
			name: "deep merge of nested maps",
			defaults: &runtime.RawExtension{
				Raw: []byte(`{"resources": {"cpu": "100m", "memory": "128Mi"}}`),
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {Image: "nginx"},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{
						Key: "db",
					},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://localhost/db"),
						},
					},
				},
			},
			expected: map[string]interface{}{
				"name":      "test-app",
				"namespace": "",
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image": "nginx",
					},
				},
				"resources": map[string]interface{}{
					"cpu":    "100m",  // from defaults
					"memory": "128Mi", // from defaults
					"db": map[string]interface{}{ // from outputs
						"outputs": map[string]interface{}{
							"uri": "postgres://localhost/db",
						},
					},
				},
			},
		},
		{
			name: "invalid defaults JSON",
			defaults: &runtime.RawExtension{
				Raw: []byte(`{invalid json`),
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {Image: "nginx"},
					},
				},
			},
			claims:      []scorev1b1.ResourceClaim{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := composeValues(tt.defaults, tt.workload, tt.claims)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil || result.Raw == nil {
				t.Fatalf("expected non-nil result")
			}

			var actual map[string]interface{}
			if err := json.Unmarshal(result.Raw, &actual); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			if !equalMaps(actual, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, actual)
			}
		})
	}
}

func TestNormalizeWorkload(t *testing.T) {
	tests := []struct {
		name     string
		workload *scorev1b1.Workload
		expected map[string]interface{}
	}{
		{
			name: "complete workload",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "test",
					},
					Annotations: map[string]string{
						"version": "1.0",
					},
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {
							Image:   "nginx:1.20",
							Command: []string{"/bin/sh"},
							Args:    []string{"-c", "nginx"},
							Variables: map[string]string{
								"ENV": "prod",
							},
						},
						"sidecar": {
							Image: "busybox",
						},
					},
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{
								Port:     80,
								Protocol: "TCP",
							},
							{
								Port: 443,
							},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"name":      "test-app",
				"namespace": "test-ns",
				"labels": map[string]interface{}{
					"app": "test",
				},
				"annotations": map[string]interface{}{
					"version": "1.0",
				},
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image":   "nginx:1.20",
						"command": []interface{}{"/bin/sh"},
						"args":    []interface{}{"-c", "nginx"},
						"env": map[string]interface{}{
							"ENV": "prod",
						},
					},
					"sidecar": map[string]interface{}{
						"image": "busybox",
					},
				},
				"service": map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port":     80,
							"protocol": "TCP",
						},
						map[string]interface{}{
							"port": 443,
						},
					},
				},
			},
		},
		{
			name: "minimal workload",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "minimal-app",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Image: "hello-world",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"name":      "minimal-app",
				"namespace": "",
				"containers": map[string]interface{}{
					"app": map[string]interface{}{
						"image": "hello-world",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeWorkload(tt.workload)

			// JSON経由で型の一貫性を保証
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("failed to marshal result: %v", err)
			}

			expectedJSON, err := json.Marshal(tt.expected)
			if err != nil {
				t.Fatalf("failed to marshal expected: %v", err)
			}

			var resultNormalized, expectedNormalized map[string]interface{}
			if err := json.Unmarshal(resultJSON, &resultNormalized); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal(expectedJSON, &expectedNormalized); err != nil {
				t.Fatalf("failed to unmarshal expected: %v", err)
			}

			if !equalMaps(resultNormalized, expectedNormalized) {
				t.Errorf("expected %+v, got %+v", expectedNormalized, resultNormalized)
			}
		})
	}
}

func TestNormalizeWorkloadWithResources(t *testing.T) {
	tests := []struct {
		name     string
		workload *scorev1b1.Workload
		expected map[string]interface{}
	}{
		{
			name: "workload with resources",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {
							Image: "nginx",
						},
					},
					Resources: map[string]scorev1b1.ResourceSpec{
						"database": {
							Type:  "postgres",
							Class: ptr.To("large"),
							Params: &apiextv1.JSON{
								Raw: []byte(`{"version": "13", "storage": "100Gi"}`),
							},
						},
						"cache": {
							Type: "redis",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"name":      "test-app",
				"namespace": "default",
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image": "nginx",
					},
				},
				"resources": map[string]interface{}{
					"database": map[string]interface{}{
						"type":  "postgres",
						"class": "large",
						"params": map[string]interface{}{
							"version": "13",
							"storage": "100Gi",
						},
					},
					"cache": map[string]interface{}{
						"type": "redis",
					},
				},
			},
		},
		{
			name: "workload with invalid params JSON",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {Image: "nginx"},
					},
					Resources: map[string]scorev1b1.ResourceSpec{
						"broken": {
							Type: "postgres",
							Params: &apiextv1.JSON{
								Raw: []byte(`{invalid json`), // Invalid JSON should be ignored
							},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"name":      "test-app",
				"namespace": "",
				"containers": map[string]interface{}{
					"main": map[string]interface{}{
						"image": "nginx",
					},
				},
				"resources": map[string]interface{}{
					"broken": map[string]interface{}{
						"type": "postgres",
						// params should not be included due to invalid JSON
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeWorkload(tt.workload)

			// JSON経由で型の一貫性を保証
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("failed to marshal result: %v", err)
			}

			expectedJSON, err := json.Marshal(tt.expected)
			if err != nil {
				t.Fatalf("failed to marshal expected: %v", err)
			}

			var resultNormalized, expectedNormalized map[string]interface{}
			if err := json.Unmarshal(resultJSON, &resultNormalized); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal(expectedJSON, &expectedNormalized); err != nil {
				t.Fatalf("failed to unmarshal expected: %v", err)
			}

			if !equalMaps(resultNormalized, expectedNormalized) {
				t.Errorf("expected %+v, got %+v", expectedNormalized, resultNormalized)
			}
		})
	}
}

func TestExtractOutputs(t *testing.T) {
	tests := []struct {
		name     string
		claims   []scorev1b1.ResourceClaim
		expected map[string]interface{}
	}{
		{
			name: "multiple claims with different outputs",
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{
						Key: "database",
					},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://localhost:5432/db"),
							SecretRef: &scorev1b1.LocalObjectReference{
								Name: "db-secret",
							},
						},
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{
						Key: "cache",
					},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("redis://localhost:6379"),
						},
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{
						Key: "storage",
					},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: false, // Not available yet
					},
				},
			},
			expected: map[string]interface{}{
				"resources": map[string]interface{}{
					"database": map[string]interface{}{
						"outputs": map[string]interface{}{
							"uri": "postgres://localhost:5432/db",
							"secretRef": map[string]interface{}{
								"name": "db-secret",
							},
						},
					},
					"cache": map[string]interface{}{
						"outputs": map[string]interface{}{
							"uri": "redis://localhost:6379",
						},
					},
				},
			},
		},
		{
			name:     "empty claims",
			claims:   []scorev1b1.ResourceClaim{},
			expected: map[string]interface{}{},
		},
		{
			name: "claims with no available outputs",
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{
						Key: "pending",
					},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: false,
					},
				},
			},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOutputs(tt.claims)

			if !equalMaps(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		maps     []map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "right-hand precedence",
			maps: []map[string]interface{}{
				{"a": 1, "b": 2},
				{"b": 3, "c": 4},
				{"c": 5, "d": 6},
			},
			expected: map[string]interface{}{
				"a": 1, // from first
				"b": 3, // from second (overrides first)
				"c": 5, // from third (overrides second)
				"d": 6, // from third
			},
		},
		{
			name: "deep merge of nested maps",
			maps: []map[string]interface{}{
				{
					"config": map[string]interface{}{
						"database": map[string]interface{}{
							"host": "localhost",
							"port": 5432,
						},
						"cache": map[string]interface{}{
							"ttl": 300,
						},
					},
				},
				{
					"config": map[string]interface{}{
						"database": map[string]interface{}{
							"port": 5433, // overrides
							"ssl":  true, // adds new
						},
						"logging": map[string]interface{}{
							"level": "info",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"config": map[string]interface{}{
					"database": map[string]interface{}{
						"host": "localhost", // from first
						"port": 5433,        // from second (overrides)
						"ssl":  true,        // from second (new)
					},
					"cache": map[string]interface{}{
						"ttl": 300, // from first
					},
					"logging": map[string]interface{}{
						"level": "info", // from second
					},
				},
			},
		},
		{
			name: "non-map values override completely",
			maps: []map[string]interface{}{
				{
					"value": map[string]interface{}{
						"nested": "original",
					},
				},
				{
					"value": "replacement", // completely replaces the map
				},
			},
			expected: map[string]interface{}{
				"value": "replacement",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeMaps(tt.maps...)

			if !equalMaps(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

// Helper function to compare maps deeply
func equalMaps(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}

	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists {
			return false
		}

		if !equalValues(valueA, valueB) {
			return false
		}
	}

	return true
}

// Helper function to compare values deeply
func equalValues(a, b interface{}) bool {
	switch aVal := a.(type) {
	case map[string]interface{}:
		bVal, ok := b.(map[string]interface{})
		if !ok {
			return false
		}
		return equalMaps(aVal, bVal)
	case []interface{}:
		bVal, ok := b.([]interface{})
		if !ok || len(aVal) != len(bVal) {
			return false
		}
		for i, itemA := range aVal {
			if !equalValues(itemA, bVal[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
