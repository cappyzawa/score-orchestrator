package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NormalizeConditions converts runtime-specific conditions to abstract vocabulary
// for consistent Workload status representation.
func NormalizeConditions(conditions []metav1.Condition) []metav1.Condition {
	if conditions == nil {
		return nil
	}

	normalized := make([]metav1.Condition, 0, len(conditions))
	for _, condition := range conditions {
		if normalizedCondition := normalizeCondition(condition); normalizedCondition != nil {
			normalized = append(normalized, *normalizedCondition)
		}
	}
	return normalized
}

// normalizeCondition converts a single condition to abstract vocabulary.
// Returns nil if the condition should be filtered out.
func normalizeCondition(condition metav1.Condition) *metav1.Condition {
	normalizedReason := normalizeReason(condition.Reason)
	if normalizedReason == "" {
		return nil // Filter out unmappable conditions
	}

	normalizedType := normalizeConditionType(condition.Type)
	if normalizedType == "" {
		return nil // Filter out unmappable condition types
	}

	normalized := condition.DeepCopy()
	normalized.Type = normalizedType
	normalized.Reason = normalizedReason
	normalized.Message = sanitizeMessage(condition.Message)

	return normalized
}

// normalizeReason maps runtime-specific reasons to abstract vocabulary.
func normalizeReason(reason string) string {
	reasonMap := map[string]string{
		// Success cases
		"Ready":     "Succeeded",
		"Available": "Succeeded",
		"Deployed":  "Succeeded",
		"Active":    "Succeeded",
		"Healthy":   "Succeeded",
		"Running":   "Succeeded",

		// Validation errors
		"InvalidSpec":          "SpecInvalid",
		"ValidationError":      "SpecInvalid",
		"SchemaViolation":      "SpecInvalid",
		"InvalidConfiguration": "SpecInvalid",

		// Policy violations
		"PolicyViolation":   "PolicyViolation",
		"SecurityViolation": "PolicyViolation",
		"ComplianceFailure": "PolicyViolation",
		"AdmissionDenied":   "PolicyViolation",

		// Binding issues
		"Pending":             "BindingPending",
		"Waiting":             "BindingPending",
		"Provisioning":        "BindingPending",
		"BindingFailed":       "BindingFailed",
		"ProvisioningFailed":  "BindingFailed",
		"ResourceUnavailable": "BindingFailed",

		// Runtime projection errors
		"ProjectionError":     "ProjectionError",
		"TransformationError": "ProjectionError",
		"MappingError":        "ProjectionError",

		// Runtime state
		"Selecting":           "RuntimeSelecting",
		"RuntimeProvisioning": "RuntimeProvisioning",
		"Degraded":            "RuntimeDegraded",
		"Unavailable":         "RuntimeDegraded",
		"Failed":              "RuntimeDegraded",

		// Resource constraints
		"QuotaExceeded":         "QuotaExceeded",
		"ResourceQuotaExceeded": "QuotaExceeded",
		"LimitExceeded":         "QuotaExceeded",

		// Permission issues
		"PermissionDenied": "PermissionDenied",
		"Forbidden":        "PermissionDenied",
		"Unauthorized":     "PermissionDenied",
		"AccessDenied":     "PermissionDenied",

		// Network issues
		"NetworkUnavailable": "NetworkUnavailable",
		"NetworkError":       "NetworkUnavailable",
		"ConnectivityError":  "NetworkUnavailable",
		"DNSError":           "NetworkUnavailable",
	}

	if normalized, exists := reasonMap[reason]; exists {
		return normalized
	}

	// Return empty string to filter out unknown reasons
	return ""
}

// normalizeConditionType maps runtime-specific condition types to standard types.
func normalizeConditionType(conditionType string) string {
	typeMap := map[string]string{
		// Standard types (pass through)
		"Ready":        "Ready",
		"ClaimsReady":  "ClaimsReady",
		"RuntimeReady": "RuntimeReady",
		"InputsValid":  "InputsValid",

		// Runtime-specific mappings
		"Available":   "RuntimeReady",
		"Deployed":    "RuntimeReady",
		"Active":      "RuntimeReady",
		"Healthy":     "RuntimeReady",
		"Running":     "RuntimeReady",
		"Progressing": "RuntimeReady",

		// Validation mappings
		"Valid":     "InputsValid",
		"Validated": "InputsValid",
		"SpecValid": "InputsValid",

		// Claims/binding mappings
		"Bound":         "ClaimsReady",
		"Provisioned":   "ClaimsReady",
		"ResourceReady": "ClaimsReady",
	}

	if normalized, exists := typeMap[conditionType]; exists {
		return normalized
	}

	// Return empty string to filter out unknown types
	return ""
}

// sanitizeMessage ensures the condition message is neutral and doesn't contain
// runtime-specific terminology that could confuse users.
func sanitizeMessage(message string) string {
	// For now, pass through the message as-is
	// In the future, we could implement more sophisticated message sanitization
	// to remove runtime-specific terminology
	return message
}
