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

package managers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/config"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// Mock implementations
type mockConfigLoader struct {
	mock.Mock
}

func (m *mockConfigLoader) LoadConfig(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
	args := m.Called(ctx)
	return args.Get(0).(*scorev1b1.OrchestratorConfig), args.Error(1)
}

func (m *mockConfigLoader) Watch(ctx context.Context) (<-chan config.ConfigEvent, error) {
	args := m.Called(ctx)
	return args.Get(0).(<-chan config.ConfigEvent), args.Error(1)
}

func (m *mockConfigLoader) Close() error {
	args := m.Called()
	return args.Error(0)
}

type mockEventRecorder struct {
	record.EventRecorder
	events []string
}

func (m *mockEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, reason)
}

func TestPlanManager_EnsurePlan(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Spec: scorev1b1.WorkloadSpec{
			Containers: map[string]scorev1b1.ContainerSpec{
				"main": {
					Image: "nginx:latest",
				},
			},
		},
	}

	claims := []scorev1b1.ResourceClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-claim",
				Namespace: "test-ns",
			},
			Spec: scorev1b1.ResourceClaimSpec{
				Key:  "database",
				Type: "postgres",
			},
			Status: scorev1b1.ResourceClaimStatus{
				Phase:            "Bound",
				OutputsAvailable: true,
				Outputs: scorev1b1.ResourceClaimOutputs{
					URI: stringPtr("postgres://localhost:5432/test"),
				},
			},
		},
	}

	t.Run("EnsurePlan creates plan when claims are ready", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockConfigLoader := &mockConfigLoader{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
		mockRecorder := &mockEventRecorder{}

		// Create a status manager for the test
		statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
		pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

		// Mock config loading
		testConfig := &scorev1b1.OrchestratorConfig{
			Spec: scorev1b1.OrchestratorConfigSpec{
				Profiles: []scorev1b1.ProfileSpec{
					{
						Name: "test-profile",
						Backends: []scorev1b1.BackendSpec{
							{
								BackendId:    "test-backend",
								RuntimeClass: "kubernetes",
								Priority:     100,
								Template: scorev1b1.TemplateSpec{
									Kind:   "manifests",
									Ref:    "test-template:latest",
									Values: nil, // Use nil for MVP test
								},
							},
						},
					},
				},
				Defaults: scorev1b1.DefaultsSpec{
					Profile: "test-profile",
				},
			},
		}
		mockConfigLoader.On("LoadConfig", mock.Anything).Return(testConfig, nil)

		agg := status.ClaimAggregation{
			Ready:   true,
			Message: "All claims are ready",
		}

		err := pm.EnsurePlan(context.Background(), workload, claims, agg)
		require.NoError(t, err)

		// Verify WorkloadPlan was created
		planList := &scorev1b1.WorkloadPlanList{}
		err = fakeClient.List(context.Background(), planList, client.InNamespace("test-ns"))
		require.NoError(t, err)
		assert.Len(t, planList.Items, 1)

		plan := planList.Items[0]
		assert.Equal(t, "test-workload", plan.Name)
		assert.Equal(t, "test-ns", plan.Namespace)
		assert.Equal(t, "kubernetes", plan.Spec.RuntimeClass)

		// Verify event was recorded
		assert.Contains(t, mockRecorder.events, EventReasonPlanCreated)

		mockConfigLoader.AssertExpectations(t)
	})

	t.Run("EnsurePlan skips when claims are not ready", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockConfigLoader := &mockConfigLoader{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
		mockRecorder := &mockEventRecorder{}

		// Create a status manager for the test
		statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
		pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

		agg := status.ClaimAggregation{
			Ready:   false,
			Message: "Claims are not ready",
		}

		err := pm.EnsurePlan(context.Background(), workload, claims, agg)
		require.NoError(t, err)

		// Verify no WorkloadPlan was created
		planList := &scorev1b1.WorkloadPlanList{}
		err = fakeClient.List(context.Background(), planList, client.InNamespace("test-ns"))
		require.NoError(t, err)
		assert.Len(t, planList.Items, 0)

		// Verify no events were recorded
		assert.Empty(t, mockRecorder.events)

		// Config loader should not have been called
		mockConfigLoader.AssertNotCalled(t, "LoadConfig")
	})

	t.Run("EnsurePlan handles backend selection error", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockConfigLoader := &mockConfigLoader{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
		mockRecorder := &mockEventRecorder{}

		// Create a status manager for the test
		statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
		pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

		// Mock config loading failure
		mockConfigLoader.On("LoadConfig", mock.Anything).Return((*scorev1b1.OrchestratorConfig)(nil), assert.AnError)

		agg := status.ClaimAggregation{
			Ready:   true,
			Message: "All claims are ready",
		}

		err := pm.EnsurePlan(context.Background(), workload, claims, agg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load orchestrator config")

		// Verify condition was set on workload
		runtimeCondition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionRuntimeReady)
		require.NotNil(t, runtimeCondition)
		assert.Equal(t, metav1.ConditionFalse, runtimeCondition.Status)
		assert.Equal(t, conditions.ReasonRuntimeSelecting, runtimeCondition.Reason)

		mockConfigLoader.AssertExpectations(t)
	})
}

func TestPlanManager_GetPlan(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
	}

	existingPlan := &scorev1b1.WorkloadPlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
			Labels: map[string]string{
				"score.dev/workload": "test-workload",
			},
		},
		Spec: scorev1b1.WorkloadPlanSpec{
			WorkloadRef: scorev1b1.WorkloadPlanWorkloadRef{
				Name:      "test-workload",
				Namespace: "test-ns",
			},
			RuntimeClass: "kubernetes",
		},
	}

	t.Run("GetPlan returns existing plan", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPlan).Build()
		mockConfigLoader := &mockConfigLoader{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
		mockRecorder := &mockEventRecorder{}

		// Create a status manager for the test
		statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
		pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

		plan, err := pm.GetPlan(context.Background(), workload)
		require.NoError(t, err)
		require.NotNil(t, plan)
		assert.Equal(t, "test-workload", plan.Name)
		assert.Equal(t, "kubernetes", plan.Spec.RuntimeClass)
	})

	t.Run("GetPlan returns NotFound when plan doesn't exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockConfigLoader := &mockConfigLoader{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
		mockRecorder := &mockEventRecorder{}

		// Create a status manager for the test
		statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
		pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

		plan, err := pm.GetPlan(context.Background(), workload)
		require.Error(t, err)
		assert.True(t, errors.IsNotFound(err))
		assert.Nil(t, plan)
	})
}

func TestPlanManager_SelectBackend(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Spec: scorev1b1.WorkloadSpec{
			Containers: map[string]scorev1b1.ContainerSpec{
				"main": {
					Image: "nginx:latest",
				},
			},
		},
	}

	t.Run("SelectBackend returns selected backend", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockConfigLoader := &mockConfigLoader{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
		mockRecorder := &mockEventRecorder{}

		// Create a status manager for the test
		statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
		pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

		testConfig := &scorev1b1.OrchestratorConfig{
			Spec: scorev1b1.OrchestratorConfigSpec{
				Profiles: []scorev1b1.ProfileSpec{
					{
						Name: "test-profile",
						Backends: []scorev1b1.BackendSpec{
							{
								BackendId:    "test-backend",
								RuntimeClass: "kubernetes",
								Priority:     100,
								Template: scorev1b1.TemplateSpec{
									Kind:   "manifests",
									Ref:    "test-template:latest",
									Values: nil, // Use nil for MVP test
								},
							},
						},
					},
				},
				Defaults: scorev1b1.DefaultsSpec{
					Profile: "test-profile",
				},
			},
		}
		mockConfigLoader.On("LoadConfig", mock.Anything).Return(testConfig, nil)

		selectedBackend, err := pm.SelectBackend(context.Background(), workload)
		require.NoError(t, err)
		require.NotNil(t, selectedBackend)
		assert.Equal(t, "test-backend", selectedBackend.BackendID)
		assert.Equal(t, "kubernetes", selectedBackend.RuntimeClass)

		mockConfigLoader.AssertExpectations(t)
	})

	t.Run("SelectBackend handles config loading error", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockConfigLoader := &mockConfigLoader{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
		mockRecorder := &mockEventRecorder{}

		// Create a status manager for the test
		statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
		pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

		mockConfigLoader.On("LoadConfig", mock.Anything).Return((*scorev1b1.OrchestratorConfig)(nil), assert.AnError)

		selectedBackend, err := pm.SelectBackend(context.Background(), workload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load orchestrator config")
		assert.Nil(t, selectedBackend)

		mockConfigLoader.AssertExpectations(t)
	})
}
