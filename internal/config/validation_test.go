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

package config

import (
	"testing"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

func TestValidator_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *scorev1b1.OrchestratorConfig
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "score.dev/v1b1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name:    "test-config",
					Version: "1.0.0",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name:        "web-service",
							Description: "Web service profile",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-1",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web@sha256:abc123",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
					},
					Provisioners: []scorev1b1.ProvisionerSpec{
						{
							Type:        "postgres",
							Provisioner: "postgres-operator",
							Classes: []scorev1b1.ClassSpec{
								{
									Name:        "small",
									Description: "Small database",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "web-service",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid API version",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "invalid/v1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name: "test-config",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-1",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "web-service",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing required profile name",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "score.dev/v1b1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name: "test-config",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							// Missing Name
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-1",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "web-service",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate profile names",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "score.dev/v1b1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name: "test-config",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-1",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
						{
							Name: "web-service", // Duplicate
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-2",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "web-service",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate backend IDs",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "score.dev/v1b1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name: "test-config",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-1",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
								{
									BackendId:    "k8s-web-1", // Duplicate
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 90,
									Version:  "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "web-service",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid runtime class",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "score.dev/v1b1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name: "test-config",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "invalid-runtime",
									RuntimeClass: "invalid-runtime", // Invalid
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "web-service",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid template kind",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "score.dev/v1b1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name: "test-config",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-1",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "invalid-template", // Invalid
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "web-service",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "cross-reference error - invalid default profile",
			config: &scorev1b1.OrchestratorConfig{
				APIVersion: "score.dev/v1b1",
				Kind:       "OrchestratorConfig",
				Metadata: scorev1b1.OrchestratorConfigMeta{
					Name: "test-config",
				},
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web-1",
									RuntimeClass: "kubernetes",
									Template: scorev1b1.TemplateSpec{
										Kind: "manifests",
										Ref:  "registry.example.com/templates/web",
									},
									Priority: 100,
									Version:  "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "non-existent", // Invalid reference
					},
				},
			},
			wantErr: true,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validator.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateResourceConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraints *scorev1b1.ResourceConstraints
		wantErr     bool
	}{
		{
			name: "valid constraints",
			constraints: &scorev1b1.ResourceConstraints{
				CPU:     "100m-2000m",
				Memory:  "128Mi-4Gi",
				Storage: "1Gi-100Gi",
			},
			wantErr: false,
		},
		{
			name: "valid single value constraints",
			constraints: &scorev1b1.ResourceConstraints{
				CPU:     "500m",
				Memory:  "1Gi",
				Storage: "10Gi",
			},
			wantErr: false,
		},
		{
			name: "valid min-only constraints",
			constraints: &scorev1b1.ResourceConstraints{
				CPU:     "100m-",
				Memory:  "128Mi-",
				Storage: "1Gi-",
			},
			wantErr: false,
		},
		{
			name: "valid max-only constraints",
			constraints: &scorev1b1.ResourceConstraints{
				CPU:     "-2000m",
				Memory:  "-4Gi",
				Storage: "-100Gi",
			},
			wantErr: false,
		},
		{
			name: "invalid CPU format",
			constraints: &scorev1b1.ResourceConstraints{
				CPU: "invalid-format",
			},
			wantErr: true,
		},
		{
			name: "invalid memory format",
			constraints: &scorev1b1.ResourceConstraints{
				Memory: "128-4-Gi",
			},
			wantErr: true,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateResourceConstraints(tt.constraints, nil)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("validateResourceConstraints() error = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}
