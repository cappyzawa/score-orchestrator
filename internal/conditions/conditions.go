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

package conditions

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition Types
const (
	ConditionReady         = "Ready"
	ConditionBindingsReady = "ClaimsReady"
	ConditionRuntimeReady  = "RuntimeReady"
	ConditionInputsValid   = "InputsValid"
)

// Reasons (abstract vocabulary - platform-agnostic)
const (
	ReasonSucceeded           = "Succeeded"
	ReasonSpecInvalid         = "SpecInvalid"
	ReasonPolicyViolation     = "PolicyViolation"
	ReasonBindingPending      = "BindingPending"
	ReasonBindingFailed       = "BindingFailed"
	ReasonProjectionError     = "ProjectionError"
	ReasonRuntimeSelecting    = "RuntimeSelecting"
	ReasonRuntimeProvisioning = "RuntimeProvisioning"
	ReasonRuntimeDegraded     = "RuntimeDegraded"
	ReasonQuotaExceeded       = "QuotaExceeded"
	ReasonPermissionDenied    = "PermissionDenied"
	ReasonNetworkUnavailable  = "NetworkUnavailable"
)

// Standard condition messages (platform-agnostic)
const (
	MessageSpecValidationFailed      = "Workload specification validation failed"
	MessageSpecValidationPending     = "Workload specification validation pending"
	MessageClaimsNotReady            = "Resource claims are not ready"
	MessageClaimsProvisioning        = "Resource claims are being provisioned"
	MessageRuntimeProvisioningFailed = "Runtime provisioning failed"
	MessageRuntimeProvisioning       = "Runtime is being provisioned"
	MessageWorkloadReady             = "Workload is ready and operational"
	MessageAllClaimsReady            = "All resource claims are ready"
	MessageClaimsFailed              = "One or more resource claims have failed"
	MessageNoClaimsFound             = "No resource claims found"
)

// SetCondition updates a condition in the conditions slice
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason string, message string) {
	now := metav1.NewTime(time.Now())

	for i, condition := range *conditions {
		if condition.Type == conditionType {
			// Update existing condition
			if condition.Status != status || condition.Reason != reason {
				(*conditions)[i].Status = status
				(*conditions)[i].Reason = reason
				(*conditions)[i].Message = message
				(*conditions)[i].LastTransitionTime = now
			} else if condition.Message != message {
				// Update message only without changing transition time
				(*conditions)[i].Message = message
			}
			return
		}
	}

	// Add new condition
	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	})
}

// IsConditionTrue returns true if the condition is present and set to True
func IsConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}

// GetCondition returns the condition with the given type, or nil if not found
func GetCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// ComputeReadyCondition determines the Ready condition based on other conditions
// Ready = InputsValid ∧ ClaimsReady ∧ RuntimeReady
func ComputeReadyCondition(conditions []metav1.Condition) (metav1.ConditionStatus, string, string) {
	inputsValid := IsConditionTrue(conditions, ConditionInputsValid)
	claimsReady := IsConditionTrue(conditions, ConditionBindingsReady)
	runtimeReady := IsConditionTrue(conditions, ConditionRuntimeReady)

	if !inputsValid {
		inputsCond := GetCondition(conditions, ConditionInputsValid)
		if inputsCond != nil && inputsCond.Status == metav1.ConditionFalse {
			return metav1.ConditionFalse, inputsCond.Reason, MessageSpecValidationFailed
		}
		return metav1.ConditionFalse, ReasonSpecInvalid, MessageSpecValidationPending
	}

	if !claimsReady {
		claimsCond := GetCondition(conditions, ConditionBindingsReady)
		if claimsCond != nil && claimsCond.Status == metav1.ConditionFalse {
			return metav1.ConditionFalse, claimsCond.Reason, MessageClaimsNotReady
		}
		return metav1.ConditionFalse, ReasonBindingPending, MessageClaimsProvisioning
	}

	if !runtimeReady {
		runtimeCond := GetCondition(conditions, ConditionRuntimeReady)
		if runtimeCond != nil && runtimeCond.Status == metav1.ConditionFalse {
			return metav1.ConditionFalse, runtimeCond.Reason, MessageRuntimeProvisioningFailed
		}
		return metav1.ConditionFalse, ReasonRuntimeProvisioning, MessageRuntimeProvisioning
	}

	return metav1.ConditionTrue, ReasonSucceeded, MessageWorkloadReady
}
