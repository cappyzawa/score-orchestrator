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

package v1b1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=wplan
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="RUNTIME",type="string",JSONPath=".spec.runtimeClass"
// +kubebuilder:printcolumn:name="WORKLOAD",type="string",JSONPath=".spec.workloadRef.name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// WorkloadPlan is an internal contract from Orchestrator to the Runtime controller.
// It must not be user-visible via RBAC.
type WorkloadPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkloadPlanSpec   `json:"spec"`
	Status            WorkloadPlanStatus `json:"status,omitempty"`
}

// WorkloadPlanWorkloadRef identifies the Workload this plan was derived from.
type WorkloadPlanWorkloadRef struct {
	// Name is the Workload name.
	Name string `json:"name"`
	// Namespace is the Workload namespace.
	Namespace string `json:"namespace"`
}

// FromClaimOutput references a single output key produced by a claim.
type FromClaimOutput struct {
	// ClaimKey is the key under Workload.spec.resources (e.g., "db").
	ClaimKey string `json:"claimKey"`
	// OutputKey is the output field exported by the claim (e.g., "uri").
	OutputKey string `json:"outputKey"`
}

// PlanClaim declares a claim requirement passed to the runtime controller.
// It mirrors the resolution keys (type/class/params) used by resolvers.
type PlanClaim struct {
	// Key is the logical key (e.g., "db", "cache").
	Key string `json:"key"`
	// Type is the resource type (e.g., "postgres", "redis").
	Type string `json:"type"`
	// Class is the resource class (e.g., "dev", "prod", "large").
	Class *string `json:"class,omitempty"`
	// Params contains extra provisioning parameters (opaque to Score).
	Params *runtime.RawExtension `json:"params,omitempty"`
}

// WorkloadPlanSpec contains the runtime class and the materialization plan.
// All fields are internal; users should not rely on their concrete shape.
type WorkloadPlanSpec struct {
	// WorkloadRef identifies the source Workload of this plan.
	WorkloadRef WorkloadPlanWorkloadRef `json:"workloadRef"`
	// ObservedWorkloadGeneration is the generation of the Workload used to compute this plan.
	ObservedWorkloadGeneration int64 `json:"observedWorkloadGeneration"`
	// RuntimeClass is the selected runtime controller class.
	RuntimeClass string `json:"runtimeClass"`
	// Template contains the reference and type information for runtime materialization.
	Template *TemplateSpec `json:"template,omitempty"`
	// ResolvedValues contains fully resolved final values with all placeholders substituted.
	// Runtime controllers should use this as the single source of truth for template values.
	// Format: { containers: { <name>: { env: { <key>: <value|valueFrom> }}}, service: {...}, ... }
	// Note: CEL validation for placeholder prevention is not implemented due to RawExtension type limitations
	ResolvedValues *runtime.RawExtension `json:"resolvedValues,omitempty"`
	// Claims declares resource requirements to be materialized by the runtime.
	Claims []PlanClaim `json:"claims,omitempty"`
}

// WorkloadPlanPhase represents the current phase of WorkloadPlan runtime provisioning.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Failed
type WorkloadPlanPhase string

const (
	// WorkloadPlanPhasePending indicates the plan is waiting to be processed
	WorkloadPlanPhasePending WorkloadPlanPhase = "Pending"
	// WorkloadPlanPhaseProvisioning indicates runtime resources are being created
	WorkloadPlanPhaseProvisioning WorkloadPlanPhase = "Provisioning"
	// WorkloadPlanPhaseReady indicates runtime resources are ready
	WorkloadPlanPhaseReady WorkloadPlanPhase = "Ready"
	// WorkloadPlanPhaseFailed indicates runtime provisioning has failed
	WorkloadPlanPhaseFailed WorkloadPlanPhase = "Failed"
)

// WorkloadPlanStatus represents the observed state of a WorkloadPlan.
type WorkloadPlanStatus struct {
	// Phase indicates the current state of the runtime provisioning.
	// +optional
	Phase WorkloadPlanPhase `json:"phase,omitempty"`

	// Conditions represent the current state of the WorkloadPlan.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Message provides human-readable status information.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// WorkloadPlanList contains a list of WorkloadPlan.
type WorkloadPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadPlan{}, &WorkloadPlanList{})
}
