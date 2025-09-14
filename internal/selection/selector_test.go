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
			name: "selector matching with workload labels only (ADR-0004)",
			config: &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Profiles: []scorev1b1.ProfileSpec{
						{
							Name: "batch-service",
							Backends: []scorev1b1.BackendSpec{
								{
									BackendId:    "k8s-batch",
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
									"workload-type": "batch",
								},
								Profile: "batch-service",
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
						"app":           "myapp",
						"workload-type": "batch",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					// No environment labels - ADR-0004 cluster-level environment
				},
			},
			wantBackend: &SelectedBackend{
				BackendID:    "k8s-batch",
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
			name: "workload-based selector filtering (ADR-0004)",
			backends: []scorev1b1.BackendSpec{
				{
					BackendId:    "batch-backend",
					RuntimeClass: "kubernetes",
					Priority:     100,
					Version:      "1.0.0",
					Constraints: &scorev1b1.ConstraintsSpec{
						Selectors: []scorev1b1.SelectorSpec{
							{
								MatchLabels: map[string]string{
									"workload-type": "batch",
								},
							},
						},
					},
				},
				{
					BackendId:    "web-backend",
					RuntimeClass: "kubernetes",
					Priority:     50,
					Version:      "1.0.0",
					Constraints: &scorev1b1.ConstraintsSpec{
						Selectors: []scorev1b1.SelectorSpec{
							{
								MatchLabels: map[string]string{
									"app-type": "web",
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
					Labels: map[string]string{
						"workload-type": "batch",
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					// No environment labels - ADR-0004 cluster-level environment
				},
			},
			expectedId: "batch-backend",
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
					BackendId:    "special-only",
					RuntimeClass: "kubernetes",
					Priority:     100,
					Version:      "1.0.0",
					Constraints: &scorev1b1.ConstraintsSpec{
						Selectors: []scorev1b1.SelectorSpec{
							{
								MatchLabels: map[string]string{
									"special-workload": "true",
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
					// No special-workload label - won't match backend
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
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

func TestProfileSelector_ResourceConstraints(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	tests := []struct {
		name        string
		workload    *scorev1b1.Workload
		constraints scorev1b1.ResourceConstraints
		expectMatch bool
	}{
		{
			name: "workload within CPU constraints",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Image: "nginx:latest",
							Resources: &scorev1b1.ResourceRequirements{
								Requests: map[string]string{
									"cpu": "500m",
								},
							},
						},
					},
				},
			},
			constraints: scorev1b1.ResourceConstraints{
				CPU: "100m-1000m",
			},
			expectMatch: true,
		},
		{
			name: "workload exceeds CPU constraints",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Image: "nginx:latest",
							Resources: &scorev1b1.ResourceRequirements{
								Requests: map[string]string{
									"cpu": "2000m",
								},
							},
						},
					},
				},
			},
			constraints: scorev1b1.ResourceConstraints{
				CPU: "100m-1000m",
			},
			expectMatch: false,
		},
		{
			name: "workload within memory constraints",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Image: "nginx:latest",
							Resources: &scorev1b1.ResourceRequirements{
								Requests: map[string]string{
									"memory": "512Mi",
								},
							},
						},
					},
				},
			},
			constraints: scorev1b1.ResourceConstraints{
				Memory: "128Mi-1Gi",
			},
			expectMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &scorev1b1.OrchestratorConfig{}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			selector := NewProfileSelector(config, fakeClient).(*profileSelector)

			result := selector.validateResourceConstraints(tt.workload, tt.constraints)
			assert.Equal(t, tt.expectMatch, result)
		})
	}
}

func TestProfileSelector_FeatureMatching(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	tests := []struct {
		name             string
		workload         *scorev1b1.Workload
		requiredFeatures []string
		expectMatch      bool
	}{
		{
			name: "web service with HTTP ingress requirement",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {Image: "nginx:latest"},
					},
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{Port: 8080},
						},
					},
				},
			},
			requiredFeatures: []string{"http-ingress"},
			expectMatch:      true,
		},
		{
			name: "batch job without HTTP service",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {Image: "batch-job:latest"},
					},
				},
			},
			requiredFeatures: []string{"http-ingress"},
			expectMatch:      false,
		},
		{
			name: "explicit feature annotation",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Annotations: map[string]string{
						"score.dev/requirements": "monitoring,scale-to-zero",
					},
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {Image: "app:latest"},
					},
				},
			},
			requiredFeatures: []string{"monitoring"},
			expectMatch:      true,
		},
		{
			name: "database connectivity auto-detection",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {Image: "app:latest"},
					},
					Resources: map[string]scorev1b1.ResourceSpec{
						"db": {Type: "postgres"},
					},
				},
			},
			requiredFeatures: []string{"database-connectivity"},
			expectMatch:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &scorev1b1.OrchestratorConfig{}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			selector := NewProfileSelector(config, fakeClient).(*profileSelector)

			result := selector.validateFeatureRequirements(tt.workload, tt.requiredFeatures)
			assert.Equal(t, tt.expectMatch, result)
		})
	}
}
