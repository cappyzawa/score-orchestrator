package managers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
)

func TestStatusManager_UpdateStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Status: scorev1b1.WorkloadStatus{
			Conditions: []metav1.Condition{
				{
					Type:   conditions.ConditionReady,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	t.Run("UpdateStatus succeeds", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&scorev1b1.Workload{}).Build()
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		// Create workload first
		err := fakeClient.Create(context.Background(), workload)
		require.NoError(t, err)

		err = sm.UpdateStatus(context.Background(), workload)
		assert.NoError(t, err)
	})

	t.Run("UpdateStatus handles client error", func(t *testing.T) {
		// Use a client that will fail status updates
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build() // No status subresource
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		err := sm.UpdateStatus(context.Background(), workload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update workload status")
	})
}

func TestStatusManager_ComputeReadyCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	mockRecorder := &mockEventRecorder{}
	endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

	sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

	t.Run("All conditions true results in Ready=True", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: conditions.ConditionInputsValid, Status: metav1.ConditionTrue},
			{Type: conditions.ConditionClaimsReady, Status: metav1.ConditionTrue},
			{Type: conditions.ConditionRuntimeReady, Status: metav1.ConditionTrue},
		}

		status, reason, message := sm.ComputeReadyCondition(conditions)

		assert.Equal(t, metav1.ConditionTrue, status)
		assert.Equal(t, "Succeeded", reason)
		assert.Equal(t, "Workload is ready and operational", message)
	})

	t.Run("InputsValid false results in Ready=False", func(t *testing.T) {
		conditions := []metav1.Condition{
			{
				Type:   conditions.ConditionInputsValid,
				Status: metav1.ConditionFalse,
				Reason: "SpecInvalid",
			},
			{Type: conditions.ConditionClaimsReady, Status: metav1.ConditionTrue},
			{Type: conditions.ConditionRuntimeReady, Status: metav1.ConditionTrue},
		}

		status, reason, message := sm.ComputeReadyCondition(conditions)

		assert.Equal(t, metav1.ConditionFalse, status)
		assert.Equal(t, "SpecInvalid", reason)
		assert.Equal(t, "Workload specification validation failed", message)
	})

	t.Run("ClaimsReady false results in Ready=False", func(t *testing.T) {
		conditions := []metav1.Condition{
			{Type: conditions.ConditionInputsValid, Status: metav1.ConditionTrue},
			{
				Type:   conditions.ConditionClaimsReady,
				Status: metav1.ConditionFalse,
				Reason: "ClaimPending",
			},
			{Type: conditions.ConditionRuntimeReady, Status: metav1.ConditionTrue},
		}

		status, reason, message := sm.ComputeReadyCondition(conditions)

		assert.Equal(t, metav1.ConditionFalse, status)
		assert.Equal(t, "ClaimPending", reason)
		assert.Equal(t, "Resource claims are not ready", message)
	})
}

func TestStatusManager_DeriveEndpoint(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Spec: scorev1b1.WorkloadSpec{
			Service: &scorev1b1.ServiceSpec{
				Ports: []scorev1b1.ServicePort{
					{Port: 8080},
				},
			},
		},
	}

	plan := &scorev1b1.WorkloadPlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Spec: scorev1b1.WorkloadPlanSpec{
			RuntimeClass: "kubernetes",
		},
	}

	t.Run("DeriveEndpoint succeeds", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		result, err := sm.DeriveEndpoint(context.Background(), workload, plan)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Contains(t, *result, "test-workload.test-ns.svc.cluster.local")
	})

	t.Run("DeriveEndpoint with nil plan returns nil", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		result, err := sm.DeriveEndpoint(context.Background(), workload, nil)

		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("DeriveEndpoint handles nil endpointDeriver", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockRecorder := &mockEventRecorder{}

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, nil)

		result, err := sm.DeriveEndpoint(context.Background(), workload, plan)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "endpoint deriver not configured")
		assert.Nil(t, result)
	})
}

func TestStatusManager_SetConditions(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	mockRecorder := &mockEventRecorder{}
	endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

	sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
	}

	t.Run("SetInputsValidCondition", func(t *testing.T) {
		sm.SetInputsValidCondition(workload, true, "Succeeded", "Validation passed")

		condition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionInputsValid)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionTrue, condition.Status)
		assert.Equal(t, "Succeeded", condition.Reason)
		assert.Equal(t, "Validation passed", condition.Message)
	})

	t.Run("SetClaimsReadyCondition", func(t *testing.T) {
		sm.SetClaimsReadyCondition(workload, false, "ClaimPending", "Claims are binding")

		condition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionClaimsReady)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
		assert.Equal(t, "ClaimPending", condition.Reason)
		assert.Equal(t, "Claims are binding", condition.Message)
	})

	t.Run("SetRuntimeReadyCondition", func(t *testing.T) {
		sm.SetRuntimeReadyCondition(workload, true, "Succeeded", "Runtime is ready")

		condition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionRuntimeReady)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionTrue, condition.Status)
		assert.Equal(t, "Succeeded", condition.Reason)
		assert.Equal(t, "Runtime is ready", condition.Message)
	})
}

func TestStatusManager_ComputeFinalStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Spec: scorev1b1.WorkloadSpec{
			Service: &scorev1b1.ServiceSpec{
				Ports: []scorev1b1.ServicePort{
					{Port: 8080},
				},
			},
		},
	}

	plan := &scorev1b1.WorkloadPlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Spec: scorev1b1.WorkloadPlanSpec{
			RuntimeClass: "kubernetes",
		},
	}

	t.Run("ComputeFinalStatus with plan", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		// Set some initial conditions
		sm.SetInputsValidCondition(workload, true, "Succeeded", "Valid")
		sm.SetClaimsReadyCondition(workload, true, "Succeeded", "Ready")

		err := sm.ComputeFinalStatus(context.Background(), workload, plan)

		assert.NoError(t, err)

		// Check that RuntimeReady condition was set
		runtimeCondition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionRuntimeReady)
		require.NotNil(t, runtimeCondition)
		assert.Equal(t, metav1.ConditionFalse, runtimeCondition.Status) // MVP: always false when plan exists
		assert.Equal(t, "RuntimeProvisioning", runtimeCondition.Reason)

		// Check that Ready condition was computed
		readyCondition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionReady)
		require.NotNil(t, readyCondition)

		// Check that endpoint was derived
		assert.NotNil(t, workload.Status.Endpoint)
		assert.Contains(t, *workload.Status.Endpoint, "test-workload.test-ns.svc.cluster.local")

		// Check that event was recorded for non-ready workload
		assert.Empty(t, mockRecorder.events) // Should be empty because workload is not ready
	})

	t.Run("ComputeFinalStatus with nil plan", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		err := sm.ComputeFinalStatus(context.Background(), workload, nil)

		assert.NoError(t, err)

		// Check that RuntimeReady condition was set to selecting
		runtimeCondition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionRuntimeReady)
		require.NotNil(t, runtimeCondition)
		assert.Equal(t, metav1.ConditionFalse, runtimeCondition.Status)
		assert.Equal(t, "RuntimeSelecting", runtimeCondition.Reason)
	})
}

func TestStatusManager_updateRuntimeStatusFromPlan(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, scorev1b1.AddToScheme(scheme))

	workload := &scorev1b1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "test-ns",
		},
		Spec: scorev1b1.WorkloadSpec{
			Service: &scorev1b1.ServiceSpec{
				Ports: []scorev1b1.ServicePort{
					{Port: 8080},
				},
			},
		},
	}

	t.Run("updateRuntimeStatusFromPlan with nil plan", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		sm.updateRuntimeStatusFromPlan(context.Background(), workload, nil)

		// Check RuntimeReady condition
		condition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionRuntimeReady)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
		assert.Equal(t, "RuntimeSelecting", condition.Reason)
	})

	t.Run("updateRuntimeStatusFromPlan with valid plan", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mockRecorder := &mockEventRecorder{}
		endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

		sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

		plan := &scorev1b1.WorkloadPlan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workload",
				Namespace: "test-ns",
			},
			Spec: scorev1b1.WorkloadPlanSpec{
				RuntimeClass: "kubernetes",
			},
		}

		sm.updateRuntimeStatusFromPlan(context.Background(), workload, plan)

		// Check RuntimeReady condition
		condition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionRuntimeReady)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
		assert.Equal(t, "RuntimeProvisioning", condition.Reason)

		// Check endpoint was set
		assert.NotNil(t, workload.Status.Endpoint)
		assert.Contains(t, *workload.Status.Endpoint, "test-workload.test-ns.svc.cluster.local")
	})
}
