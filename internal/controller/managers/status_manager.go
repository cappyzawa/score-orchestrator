package managers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
)

// Event constants for StatusManager
const (
	EventReasonReady         = "Ready"
	EventReasonNotReady      = "NotReady"
	EventReasonStatusUpdated = "StatusUpdated"
)

// StatusManager handles all Workload status management operations
type StatusManager struct {
	client          client.Client
	scheme          *runtime.Scheme
	recorder        record.EventRecorder
	endpointDeriver *endpoint.EndpointDeriver
}

// NewStatusManager creates a new StatusManager instance
func NewStatusManager(
	c client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	endpointDeriver *endpoint.EndpointDeriver,
) *StatusManager {
	return &StatusManager{
		client:          c,
		scheme:          scheme,
		recorder:        recorder,
		endpointDeriver: endpointDeriver,
	}
}

// UpdateStatus updates the Workload status and handles conflicts
func (sm *StatusManager) UpdateStatus(ctx context.Context, workload *scorev1b1.Workload) error {
	log := ctrl.LoggerFrom(ctx)

	if err := sm.client.Status().Update(ctx, workload); err != nil {
		log.Error(err, "Failed to update Workload status")
		return fmt.Errorf("failed to update workload status: %w", err)
	}

	log.V(1).Info("Successfully updated Workload status")
	return nil
}

// ComputeReadyCondition determines the Ready condition based on other conditions
// Ready = InputsValid ∧ ClaimsReady ∧ RuntimeReady
func (sm *StatusManager) ComputeReadyCondition(conditionsSlice []metav1.Condition) (metav1.ConditionStatus, string, string) {
	return conditions.ComputeReadyCondition(conditionsSlice)
}

// DeriveEndpoint derives the canonical endpoint for a Workload from its WorkloadPlan
func (sm *StatusManager) DeriveEndpoint(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) (*string, error) {
	if sm.endpointDeriver == nil {
		return nil, fmt.Errorf("endpoint deriver not configured")
	}

	derivedEndpoint, err := sm.endpointDeriver.DeriveEndpoint(ctx, workload, plan)
	if err != nil {
		return nil, fmt.Errorf("failed to derive endpoint: %w", err)
	}

	if derivedEndpoint == "" {
		return nil, nil
	}

	return &derivedEndpoint, nil
}

// SetInputsValidCondition sets the InputsValid condition on the workload
func (sm *StatusManager) SetInputsValidCondition(
	workload *scorev1b1.Workload,
	valid bool,
	reason, message string,
) {
	status := metav1.ConditionFalse
	if valid {
		status = metav1.ConditionTrue
	}

	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionInputsValid,
		status,
		reason,
		message,
	)
}

// SetClaimsReadyCondition sets the ClaimsReady condition on the workload
func (sm *StatusManager) SetClaimsReadyCondition(
	workload *scorev1b1.Workload,
	ready bool,
	reason, message string,
) {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}

	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionClaimsReady,
		status,
		reason,
		message,
	)
}

// SetRuntimeReadyCondition sets the RuntimeReady condition on the workload
func (sm *StatusManager) SetRuntimeReadyCondition(
	workload *scorev1b1.Workload,
	ready bool,
	reason, message string,
) {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}

	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionRuntimeReady,
		status,
		reason,
		message,
	)
}

// ComputeFinalStatus updates runtime status and computes Ready condition
func (sm *StatusManager) ComputeFinalStatus(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) error {
	log := ctrl.LoggerFrom(ctx)

	// Update RuntimeReady condition and endpoint based on plan
	sm.updateRuntimeStatusFromPlan(ctx, workload, plan)

	// Compute and set Ready condition
	readyStatus, readyReason, readyMessage := sm.ComputeReadyCondition(workload.Status.Conditions)
	conditions.SetCondition(
		&workload.Status.Conditions,
		conditions.ConditionReady,
		readyStatus,
		readyReason,
		readyMessage,
	)

	// Emit events based on Ready status
	if readyStatus == metav1.ConditionTrue {
		sm.recorder.Event(workload, "Normal", EventReasonReady, "Workload is ready and operational")
		log.V(1).Info("Workload is ready")
	} else {
		log.V(1).Info("Workload is not ready", "reason", readyReason, "message", readyMessage)
	}

	return nil
}

// updateRuntimeStatusFromPlan updates RuntimeReady condition and endpoint based on WorkloadPlan
func (sm *StatusManager) updateRuntimeStatusFromPlan(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) {
	log := ctrl.LoggerFrom(ctx)

	if plan == nil {
		sm.SetRuntimeReadyCondition(
			workload,
			false,
			conditions.ReasonRuntimeSelecting,
			"Runtime controller is being selected",
		)
		return
	}

	// Derive endpoint from WorkloadPlan
	derivedEndpoint, err := sm.DeriveEndpoint(ctx, workload, plan)
	if err != nil {
		log.Error(err, "Failed to derive endpoint")
		sm.SetRuntimeReadyCondition(
			workload,
			false,
			conditions.ReasonProjectionError,
			fmt.Sprintf("Failed to derive endpoint: %v", err),
		)
		return
	}

	// Update endpoint in status if derived
	if derivedEndpoint != nil && *derivedEndpoint != "" {
		workload.Status.Endpoint = derivedEndpoint
		log.V(1).Info("Derived endpoint", "endpoint", *derivedEndpoint)
	}

	// Check actual runtime resource status
	runtimeReady, reason, message := sm.checkRuntimeResourceStatus(ctx, workload, plan)
	sm.SetRuntimeReadyCondition(workload, runtimeReady, reason, message)
}

// checkRuntimeResourceStatus checks the actual state of runtime resources for kubernetes runtime
func (sm *StatusManager) checkRuntimeResourceStatus(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) (bool, string, string) {
	log := ctrl.LoggerFrom(ctx)

	// For non-kubernetes runtimes, assume ready for now
	if plan.Spec.RuntimeClass != "kubernetes" {
		return true, conditions.ReasonSucceeded, "Runtime provisioned successfully"
	}

	workloadName := plan.Spec.WorkloadRef.Name
	namespace := plan.Spec.WorkloadRef.Namespace

	// Check Deployment status
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{
		Name:      workloadName,
		Namespace: namespace,
	}

	err := sm.client.Get(ctx, deploymentKey, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			log.V(1).Info("Deployment not found, runtime provisioning in progress", "deployment", workloadName)
			return false, conditions.ReasonRuntimeProvisioning, "Runtime resources are being provisioned"
		}
		log.Error(err, "Failed to get Deployment", "deployment", workloadName)
		return false, conditions.ReasonRuntimeProvisioning, fmt.Sprintf("Failed to check runtime status: %v", err)
	}

	// Check if Deployment is ready - first check ReadyReplicas if available
	if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas >= deployment.Status.Replicas {
		log.V(1).Info("Deployment ready via ReadyReplicas", "deployment", workloadName, "readyReplicas", deployment.Status.ReadyReplicas, "replicas", deployment.Status.Replicas)
	} else {
		// ReadyReplicas might not be updated yet, check Pods directly
		pods := &corev1.PodList{}
		listOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels(deployment.Spec.Selector.MatchLabels),
		}

		err := sm.client.List(ctx, pods, listOpts...)
		if err != nil {
			log.Error(err, "Failed to list Pods", "deployment", workloadName)
			return false, conditions.ReasonRuntimeProvisioning, fmt.Sprintf("Failed to check pod status: %v", err)
		}

		readyPods := 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						readyPods++
						break
					}
				}
			}
		}

		if readyPods == 0 {
			log.V(1).Info("No ready pods found", "deployment", workloadName, "totalPods", len(pods.Items), "readyPods", readyPods)
			return false, conditions.ReasonRuntimeProvisioning, "Runtime deployment is starting up"
		}

		log.V(1).Info("Deployment ready via pod check", "deployment", workloadName, "readyPods", readyPods, "totalPods", len(pods.Items))
	}

	// Check Service status (if it should exist)
	if workload.Spec.Service != nil && len(workload.Spec.Service.Ports) > 0 {
		service := &corev1.Service{}
		serviceKey := types.NamespacedName{
			Name:      workloadName,
			Namespace: namespace,
		}

		err := sm.client.Get(ctx, serviceKey, service)
		if err != nil {
			if errors.IsNotFound(err) {
				log.V(1).Info("Service not found, runtime provisioning in progress", "service", workloadName)
				return false, conditions.ReasonRuntimeProvisioning, "Runtime service is being provisioned"
			}
			log.Error(err, "Failed to get Service", "service", workloadName)
			return false, conditions.ReasonRuntimeProvisioning, fmt.Sprintf("Failed to check service status: %v", err)
		}

		// Basic service readiness check - ensure it has a ClusterIP
		if service.Spec.ClusterIP == "" {
			log.V(1).Info("Service not ready", "service", workloadName)
			return false, conditions.ReasonRuntimeProvisioning, "Runtime service is being configured"
		}
	}

	log.Info("Runtime resources are ready", "workload", workloadName)
	return true, conditions.ReasonSucceeded, "Runtime provisioned successfully"
}
