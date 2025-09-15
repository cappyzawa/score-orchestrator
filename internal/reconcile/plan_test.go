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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/selection"
)

func TestValidateProjectionRequirements(t *testing.T) {
	tests := []struct {
		name        string
		workload    *scorev1b1.Workload
		claims      []scorev1b1.ResourceClaim
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid projection - all outputs available",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"DATABASE_URL": "${resources.db.outputs.uri}",
								"CACHE_URL":    "${resources.cache.outputs.uri}",
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
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://localhost/db"),
						},
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "cache"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("redis://localhost/0"),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing resource outputs",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"DATABASE_URL": "${resources.db.outputs.uri}",
							},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec:   scorev1b1.ResourceClaimSpec{Key: "db"},
					Status: scorev1b1.ResourceClaimStatus{OutputsAvailable: false},
				},
			},
			expectError: true,
			errorMsg:    "resource 'db' has no outputs available",
		},
		{
			name: "missing specific output",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"DATABASE_URL": "${resources.db.outputs.uri}",
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
						Outputs: scorev1b1.ResourceClaimOutputs{
							SecretRef: &scorev1b1.LocalObjectReference{Name: "db-secret"},
							// URI is missing
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "missing output 'uri'",
		},
		{
			name: "volume source references",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Files: []scorev1b1.FileSpec{
								{
									Target: "/data",
									Source: &scorev1b1.FileSourceSpec{
										URI: "${resources.storage.outputs.secretRef}",
									},
								},
							},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "storage"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							SecretRef: &scorev1b1.LocalObjectReference{Name: "storage-secret"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "file source references",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Files: []scorev1b1.FileSpec{
								{
									Target: "/etc/ssl/cert.pem",
									Source: &scorev1b1.FileSourceSpec{
										URI: "${resources.tls.outputs.cert}",
									},
								},
							},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "tls"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							Cert: &scorev1b1.CertificateOutput{
								Data: map[string][]byte{
									"cert.pem": []byte("certificate-data"),
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "no resource references - should pass",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"STATIC_VAR": "static-value",
							},
						},
					},
				},
			},
			claims:      []scorev1b1.ResourceClaim{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectionRequirements(tt.workload, tt.claims)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBuildProjection(t *testing.T) {
	tests := []struct {
		name       string
		workload   *scorev1b1.Workload
		claims     []scorev1b1.ResourceClaim
		expectEnv  int
		expectVol  int
		expectFile int
	}{
		{
			name: "environment variable mappings",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"DATABASE_URL": "${resources.db.outputs.uri}",
								"API_KEY":      "${resources.auth.outputs.secretRef}",
								"STATIC":       "value",
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
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://localhost/db"),
						},
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "auth"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							SecretRef: &scorev1b1.LocalObjectReference{Name: "auth-secret"},
						},
					},
				},
			},
			expectEnv: 2, // DATABASE_URL and API_KEY mappings
		},
		{
			name: "volume projections",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Files: []scorev1b1.FileSpec{
								{
									Target: "/config",
									Source: &scorev1b1.FileSourceSpec{
										URI: "${resources.config.outputs.configMapRef}",
									},
								},
								{
									Target: "/secrets",
									Source: &scorev1b1.FileSourceSpec{
										URI: "${resources.auth.outputs.secretRef}",
									},
								},
								{
									Target: "/static",
									Source: &scorev1b1.FileSourceSpec{
										URI: "/host/path",
									},
								},
							},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "config"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							ConfigMapRef: &scorev1b1.LocalObjectReference{Name: "config-map"},
						},
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "auth"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							SecretRef: &scorev1b1.LocalObjectReference{Name: "auth-secret"},
						},
					},
				},
			},
			expectVol: 2, // config and auth volume mappings
		},
		{
			name: "file projections",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Files: []scorev1b1.FileSpec{
								{
									Target: "/etc/ssl/ca.crt",
									Source: &scorev1b1.FileSourceSpec{
										URI: "${resources.tls.outputs.cert}",
									},
								},
								{
									Target:  "/config/app.yaml",
									Content: ptr.To("static content"),
								},
							},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "tls"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							Cert: &scorev1b1.CertificateOutput{
								SecretName: ptr.To("tls-secret"),
							},
						},
					},
				},
			},
			expectFile: 1, // Only the cert file projection
		},
		{
			name: "default URI mapping when no explicit references",
			workload: &scorev1b1.Workload{
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Variables: map[string]string{
								"PORT": "8080",
							},
						},
					},
				},
			},
			claims: []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "database"},
					Status: scorev1b1.ResourceClaimStatus{
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceClaimOutputs{
							URI: ptr.To("postgres://localhost/db"),
						},
					},
				},
			},
			expectEnv: 1, // Default DATABASE_URI mapping
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projection := buildProjection(tt.workload, tt.claims)

			if len(projection.Env) != tt.expectEnv {
				t.Errorf("expected %d env mappings, got %d", tt.expectEnv, len(projection.Env))
			}

			if len(projection.Volumes) != tt.expectVol {
				t.Errorf("expected %d volume projections, got %d", tt.expectVol, len(projection.Volumes))
			}

			if len(projection.Files) != tt.expectFile {
				t.Errorf("expected %d file projections, got %d", tt.expectFile, len(projection.Files))
			}

			// Verify specific mappings for the first test
			if tt.name == "environment variable mappings" && len(projection.Env) >= 2 {
				foundDB := false
				foundAuth := false
				for _, mapping := range projection.Env {
					if mapping.Name == "DATABASE_URL" && mapping.From.ClaimKey == "db" && mapping.From.OutputKey == "uri" {
						foundDB = true
					}
					if mapping.Name == "API_KEY" && mapping.From.ClaimKey == "auth" && mapping.From.OutputKey == "secretRef" {
						foundAuth = true
					}
				}
				if !foundDB {
					t.Error("expected DATABASE_URL mapping not found")
				}
				if !foundAuth {
					t.Error("expected API_KEY mapping not found")
				}
			}
		})
	}
}

func TestWorkloadPlanSpecEqual(t *testing.T) {
	baseSpec := scorev1b1.WorkloadPlanSpec{
		WorkloadRef: scorev1b1.WorkloadPlanWorkloadRef{
			Name:      "test-app",
			Namespace: "default",
		},
		ObservedWorkloadGeneration: 1,
		RuntimeClass:               "kubernetes",
		Template: &scorev1b1.TemplateSpec{
			Kind: "manifests",
			Ref:  "registry.example.com/templates/web@sha256:abc123",
		},
		Values: &runtime.RawExtension{
			Raw: []byte(`{"replicas": 3}`),
		},
		Projection: scorev1b1.WorkloadProjection{
			Env: []scorev1b1.EnvMapping{
				{
					Name: "DATABASE_URL",
					From: scorev1b1.FromClaimOutput{
						ClaimKey:  "db",
						OutputKey: "uri",
					},
				},
			},
		},
		Claims: []scorev1b1.PlanClaim{
			{
				Key:  "db",
				Type: "postgres",
			},
		},
	}

	tests := []struct {
		name     string
		a, b     scorev1b1.WorkloadPlanSpec
		expected bool
	}{
		{
			name:     "identical specs",
			a:        baseSpec,
			b:        baseSpec,
			expected: true,
		},
		{
			name: "different workload ref",
			a:    baseSpec,
			b: func() scorev1b1.WorkloadPlanSpec {
				spec := baseSpec
				spec.WorkloadRef.Name = "different-app"
				return spec
			}(),
			expected: false,
		},
		{
			name: "different generation",
			a:    baseSpec,
			b: func() scorev1b1.WorkloadPlanSpec {
				spec := baseSpec
				spec.ObservedWorkloadGeneration = 2
				return spec
			}(),
			expected: false,
		},
		{
			name: "different runtime class",
			a:    baseSpec,
			b: func() scorev1b1.WorkloadPlanSpec {
				spec := baseSpec
				spec.RuntimeClass = "ecs"
				return spec
			}(),
			expected: false,
		},
		{
			name: "different env mappings count",
			a:    baseSpec,
			b: func() scorev1b1.WorkloadPlanSpec {
				spec := baseSpec
				spec.Projection.Env = []scorev1b1.EnvMapping{}
				return spec
			}(),
			expected: false,
		},
		{
			name: "different claims count",
			a:    baseSpec,
			b: func() scorev1b1.WorkloadPlanSpec {
				spec := baseSpec
				spec.Claims = []scorev1b1.PlanClaim{}
				return spec
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := workloadPlanSpecEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIntegrationWithSelectedBackend(t *testing.T) {
	// This test demonstrates the full integration with SelectedBackend
	selectedBackend := &selection.SelectedBackend{
		BackendID:    "k8s-web-standard",
		RuntimeClass: "kubernetes",
		Template: scorev1b1.TemplateSpec{
			Kind: "manifests",
			Ref:  "registry.example.com/templates/web@sha256:abc123",
			Values: &runtime.RawExtension{
				Raw: []byte(`{"replicas": 2, "resources": {"cpu": "100m"}}`),
			},
		},
		Priority: 100,
		Version:  "1.0.0",
	}

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Spec: scorev1b1.WorkloadSpec{
			Containers: map[string]scorev1b1.ContainerSpec{
				"web": {
					Image: "nginx:1.20",
					Variables: map[string]string{
						"DATABASE_URL": "${resources.db.outputs.uri}",
					},
				},
			},
		},
	}

	claims := []scorev1b1.ResourceClaim{
		{
			Spec: scorev1b1.ResourceClaimSpec{
				Key:  "db",
				Type: "postgres",
			},
			Status: scorev1b1.ResourceClaimStatus{
				OutputsAvailable: true,
				Outputs: scorev1b1.ResourceClaimOutputs{
					URI: ptr.To("postgres://prod-db:5432/myapp"),
				},
			},
		},
	}

	// Test values composition
	values, err := composeValues(selectedBackend.Template.Values, workload, claims)
	if err != nil {
		t.Fatalf("failed to compose values: %v", err)
	}

	var composedValues map[string]interface{}
	if err := json.Unmarshal(values.Raw, &composedValues); err != nil {
		t.Fatalf("failed to unmarshal composed values: %v", err)
	}

	// Verify defaults were included
	if composedValues["replicas"] != float64(2) {
		t.Errorf("expected replicas=2, got %v", composedValues["replicas"])
	}

	// Verify workload normalization
	containers, ok := composedValues["containers"].(map[string]interface{})
	if !ok {
		t.Fatal("expected containers in composed values")
	}
	webContainer, ok := containers["web"].(map[string]interface{})
	if !ok {
		t.Fatal("expected web container in composed values")
	}
	if webContainer["image"] != "nginx:1.20" {
		t.Errorf("expected nginx:1.20, got %v", webContainer["image"])
	}

	// Verify resource outputs
	resources, ok := composedValues["resources"].(map[string]interface{})
	if !ok {
		t.Fatal("expected resources in composed values")
	}
	dbResource, ok := resources["db"].(map[string]interface{})
	if !ok {
		t.Fatal("expected db resource in composed values")
	}
	outputs, ok := dbResource["outputs"].(map[string]interface{})
	if !ok {
		t.Fatal("expected outputs in db resource")
	}
	if outputs["uri"] != "postgres://prod-db:5432/myapp" {
		t.Errorf("expected postgres URI, got %v", outputs["uri"])
	}

	// Test projection building
	projection := buildProjection(workload, claims)
	if len(projection.Env) == 0 {
		t.Error("expected environment variable mappings")
	}

	foundDBMapping := false
	for _, mapping := range projection.Env {
		if mapping.Name == "DATABASE_URL" && mapping.From.ClaimKey == "db" && mapping.From.OutputKey == "uri" {
			foundDBMapping = true
			break
		}
	}
	if !foundDBMapping {
		t.Error("expected DATABASE_URL mapping not found")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInMiddle(s, substr)))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
