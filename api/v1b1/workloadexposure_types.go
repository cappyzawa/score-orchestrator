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
)

// WorkloadExposureSpec defines the desired state of WorkloadExposure.
// This is managed by the Orchestrator and specifies which Workload's endpoints should be exposed.
type WorkloadExposureSpec struct {
	// WorkloadRef identifies the source Workload for endpoint exposure.
	WorkloadRef WorkloadExposureWorkloadRef `json:"workloadRef"`
	// ObservedWorkloadGeneration is the generation of the Workload used to compute this exposure.
	ObservedWorkloadGeneration int64 `json:"observedWorkloadGeneration"`
	// RuntimeClass is the runtime controller class responsible for materializing exposures.
	RuntimeClass string `json:"runtimeClass"`
}

// WorkloadExposureWorkloadRef identifies a Workload resource.
type WorkloadExposureWorkloadRef struct {
	// Name is the name of the Workload.
	Name string `json:"name"`
	// Namespace is the namespace of the Workload.
	// If not specified, assumes the same namespace as the WorkloadExposure.
	// +optional
	Namespace *string `json:"namespace,omitempty"`
	// UID is the UID of the Workload for strong identity across renames/recreates.
	// +optional
	UID string `json:"uid,omitempty"`
}

// WorkloadExposureStatus defines the observed state of WorkloadExposure.
// This is managed by Runtime Controllers and reflects actual endpoint availability.
type WorkloadExposureStatus struct {
	// Exposures contains the list of actual exposed endpoints.
	// Exposures are ordered by descending priority; the first entry is mirrored
	// into Workload.status.endpoint by the Orchestrator.
	// +optional
	Exposures []ExposureEntry `json:"exposures,omitempty"`

	// Conditions represent the current state of the WorkloadExposure resource.
	// Standard condition types:
	// - "Ready": all exposures are ready and accessible
	// - "RuntimeReady": runtime is ready to handle exposures
	// - "ExposureReady": at least one exposure is available
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExposureEntry represents a single exposed endpoint.
type ExposureEntry struct {
	// Name is a logical identifier for this exposure (e.g., "web", "api", "metrics").
	// +optional
	Name string `json:"name,omitempty"`
	// URL is the accessible endpoint URL.
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:XValidation:rule="self.startsWith('http://') || self.startsWith('https://')",message="URL must start with http:// or https://"
	URL string `json:"url"`
	// Type describes the exposure mechanism (e.g., "ingress", "nodeport", "loadbalancer").
	// +optional
	Type string `json:"type,omitempty"`
	// Ready indicates if this exposure is ready to serve traffic.
	Ready bool `json:"ready"`
	// Scope indicates exposure reachability (e.g., Public, ClusterLocal, VPC).
	// +optional
	Scope string `json:"scope,omitempty"`
	// SchemeHint suggests protocol family for consumers (HTTP, HTTPS, GRPC, TCP, OTHER).
	// +optional
	SchemeHint string `json:"schemeHint,omitempty"`
	// Reachable indicates controller-side self-check result, if any.
	// +optional
	Reachable *bool `json:"reachable,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Workload",type="string",JSONPath=".spec.workloadRef.name"
// +kubebuilder:printcolumn:name="Runtime",type="string",JSONPath=".spec.runtimeClass"
// +kubebuilder:printcolumn:name="TopURL",type="string",JSONPath=".status.exposures[0].url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkloadExposure represents runtime-specific endpoint exposure for a Workload.
// The Orchestrator creates and manages the spec, while Runtime Controllers update the status.
type WorkloadExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadExposureSpec   `json:"spec,omitempty"`
	Status WorkloadExposureStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadExposureList contains a list of WorkloadExposure
type WorkloadExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadExposure `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadExposure{}, &WorkloadExposureList{})
}
