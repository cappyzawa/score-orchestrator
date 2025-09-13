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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

func TestProfileSelector_SelectBackend(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	tests := []struct {
		name        string
		config      *scorev1b1.OrchestratorConfig
		workload    *scorev1b1.Workload
		namespace   *corev1.Namespace
		wantBackend *SelectedBackend
		wantErr     bool
	}{
		{
			name: "user hint profile selection",
			config: &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web",
									RuntimeClass: "kubernetes",
									Priority:     100,
									Version:      "1.0.0",
								},
							},
						},
					},
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
					Annotations: map[string]string{
						"score.dev/profile": "web-service",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			wantBackend: &SelectedBackend{
				BackendID:    "k8s-web",
				RuntimeClass: "kubernetes",
				Priority:     100,
				Version:      "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "invalid user hint should fail",
			config: &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web",
									RuntimeClass: "kubernetes",
									Priority:     100,
									Version:      "1.0.0",
								},
							},
						},
					},
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
					Annotations: map[string]string{
						"score.dev/profile": "nonexistent-profile",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			wantErr: true,
		},
		{
			name: "auto-derivation for web service",
			config: &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "web-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-web",
									RuntimeClass: "kubernetes",
									Priority:     100,
									Version:      "1.0.0",
								},
							},
						},
					},
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{Port: 8080},
						},
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			wantBackend: &SelectedBackend{
				BackendID:    "k8s-web",
				RuntimeClass: "kubernetes",
				Priority:     100,
				Version:      "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "selector matching with combined labels",
			config: &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "dev-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-dev",
									RuntimeClass: "kubernetes",
									Priority:     50,
									Version:      "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Selectors: []scorev1b1.SelectorSpec{
							{
								MatchLabels: map[string]string{
									"environment": "development",
								},
								Profile: "dev-service",
							},
						},
					},
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
					Labels: map[string]string{
						"app": "myapp",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"environment": "development",
					},
				},
			},
			wantBackend: &SelectedBackend{
				BackendID:    "k8s-dev",
				RuntimeClass: "kubernetes",
				Priority:     50,
				Version:      "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "global default profile",
			config: &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "default-profile",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-default",
									RuntimeClass: "kubernetes",
									Priority:     10,
									Version:      "1.0.0",
								},
							},
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "default-profile",
					},
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			wantBackend: &SelectedBackend{
				BackendID:    "k8s-default",
				RuntimeClass: "kubernetes",
				Priority:     10,
				Version:      "1.0.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.namespace).
				Build()

			selector := NewProfileSelector(tt.config, fakeClient)

			result, err := selector.SelectBackend(context.Background(), tt.workload)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantBackend.BackendID, result.BackendID)
			assert.Equal(t, tt.wantBackend.RuntimeClass, result.RuntimeClass)
			assert.Equal(t, tt.wantBackend.Priority, result.Priority)
			assert.Equal(t, tt.wantBackend.Version, result.Version)
		})
	}
}

func TestProfileSelector_DeterministicBackendSelection(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	tests := []struct {
		name       string
		backends   []scorev1b1.BackendSpec
		expectedId string
	}{
		{
			name: "priority sorting (higher wins)",
			backends: []scorev1b1.BackendSpec{
				{BackendId: "low", Priority: 10, Version: "1.0.0"},
				{BackendId: "high", Priority: 100, Version: "1.0.0"},
				{BackendId: "medium", Priority: 50, Version: "1.0.0"},
			},
			expectedId: "high",
		},
		{
			name: "version sorting when priority equal (newer wins)",
			backends: []scorev1b1.BackendSpec{
				{BackendId: "old", Priority: 100, Version: "1.0.0"},
				{BackendId: "new", Priority: 100, Version: "2.0.0"},
				{BackendId: "middle", Priority: 100, Version: "1.5.0"},
			},
			expectedId: "new",
		},
		{
			name: "release beats prerelease",
			backends: []scorev1b1.BackendSpec{
				{BackendId: "prerelease", Priority: 100, Version: "1.0.0-rc.1"},
				{BackendId: "release", Priority: 100, Version: "1.0.0"},
			},
			expectedId: "release",
		},
		{
			name: "backendId tie-breaking when priority and version equal",
			backends: []scorev1b1.BackendSpec{
				{BackendId: "zebra", Priority: 100, Version: "1.0.0"},
				{BackendId: "alpha", Priority: 100, Version: "1.0.0"},
				{BackendId: "beta", Priority: 100, Version: "1.0.0"},
			},
			expectedId: "alpha", // lexicographically first
		},
		{
			name: "complex sorting with all criteria",
			backends: []scorev1b1.BackendSpec{
				{BackendId: "low-new", Priority: 10, Version: "2.0.0"},
				{BackendId: "high-old-z", Priority: 100, Version: "1.0.0"},
				{BackendId: "high-old-a", Priority: 100, Version: "1.0.0"},
				{BackendId: "high-new", Priority: 100, Version: "2.0.0"},
			},
			expectedId: "high-new", // highest priority, newest version
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name:     "test-profile",
							Backends: tt.backends,
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "test-profile",
					},
				},
			}

			workload := &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
			}

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace).
				Build()

			selector := NewProfileSelector(config, fakeClient)
			result, err := selector.SelectBackend(context.Background(), workload)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedId, result.BackendID)
		})
	}
}

func TestProfileSelector_BackendFiltering(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	tests := []struct {
		name        string
		backends    []scorev1b1.BackendSpec
		workload    *scorev1b1.Workload
		namespace   *corev1.Namespace
		expectedId  string
		expectError bool
	}{
		{
			name: "selector filtering",
			backends: []scorev1b1.BackendSpec{
				{
					BackendId:    "prod-only",
					RuntimeClass: "kubernetes",
					Priority:     100,
					Version:      "1.0.0",
					Constraints: &scorev1b1.ConstraintsSpec{
						Selectors: []scorev1b1.SelectorSpec{
							{
								MatchLabels: map[string]string{
									"environment": "production",
								},
							},
						},
					},
				},
				{
					BackendId:    "dev-only",
					RuntimeClass: "kubernetes",
					Priority:     50,
					Version:      "1.0.0",
					Constraints: &scorev1b1.ConstraintsSpec{
						Selectors: []scorev1b1.SelectorSpec{
							{
								MatchLabels: map[string]string{
									"environment": "development",
								},
							},
						},
					},
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"environment": "development",
					},
				},
			},
			expectedId: "dev-only",
		},
		{
			name: "feature requirement filtering",
			backends: []scorev1b1.BackendSpec{
				{
					BackendId:    "feature-required",
					RuntimeClass: "kubernetes",
					Priority:     100,
					Version:      "1.0.0",
					Constraints: &scorev1b1.ConstraintsSpec{
						Features: []string{"http-ingress", "auto-scaling"},
					},
				},
				{
					BackendId:    "no-features",
					RuntimeClass: "kubernetes",
					Priority:     50,
					Version:      "1.0.0",
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
					Annotations: map[string]string{
						"score.dev/requirements": "http-ingress,auto-scaling",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			expectedId: "feature-required",
		},
		{
			name: "no candidates after filtering",
			backends: []scorev1b1.BackendSpec{
				{
					BackendId:    "prod-only",
					RuntimeClass: "kubernetes",
					Priority:     100,
					Version:      "1.0.0",
					Constraints: &scorev1b1.ConstraintsSpec{
						Selectors: []scorev1b1.SelectorSpec{
							{
								MatchLabels: map[string]string{
									"environment": "production",
								},
							},
						},
					},
				},
			},
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"environment": "development", // doesn't match prod-only
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name:     "test-profile",
							Backends: tt.backends,
						},
					},
					Defaults: scorev1b1.DefaultsSpec{
						Profile: "test-profile",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.namespace).
				Build()

			selector := NewProfileSelector(config, fakeClient)
			result, err := selector.SelectBackend(context.Background(), tt.workload)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedId, result.BackendID)
		})
	}
}
