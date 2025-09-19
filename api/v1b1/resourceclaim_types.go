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
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=rc
// +kubebuilder:subresource:status
// ResourceClaim represents a single resource dependency resolution contract.
// Provisioners drive it to Bound and publish standardized outputs for consumption by runtimes.
type ResourceClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ResourceClaimSpec   `json:"spec"`
	Status            ResourceClaimStatus `json:"status,omitempty"`
}

// NamespacedName identifies a namespaced Kubernetes object.
type NamespacedName struct {
	// Name is the object name.
	Name string `json:"name"`
	// Namespace is the object namespace.
	Namespace string `json:"namespace"`
}

// DeprovisionPolicy controls behavior when a claim is no longer needed.
// Delete removes provisioned resources; Retain keeps them; Orphan detaches ownership.
// +kubebuilder:validation:Enum=Delete;Retain;Orphan
type DeprovisionPolicy string

const (
	DeprovisionDelete DeprovisionPolicy = "Delete"
	DeprovisionRetain DeprovisionPolicy = "Retain"
	DeprovisionOrphan DeprovisionPolicy = "Orphan"
)

// ResourceClaimSpec declares the requested dependency and parameters the resolver needs.
type ResourceClaimSpec struct {
	// WorkloadRef points back to the owning Workload.
	WorkloadRef NamespacedName `json:"workloadRef"`
	// Key is the logical key under Workload.spec.resources (e.g., "db", "cache").
	Key string `json:"key"`
	// Type is an abstract resource type (e.g., "postgresql", "redis", "s3-bucket").
	Type string `json:"type"`
	// Class optionally selects a resolver subclass/plan (PF-specific).
	Class *string `json:"class,omitempty"`
	// ID optionally pins to an existing resource instance (resolver decides semantics).
	ID *string `json:"id,omitempty"`
	// Params are resolver-specific inputs (opaque to the orchestrator/runtime).
	Params *apiextv1.JSON `json:"params,omitempty"`
	// DeprovisionPolicy controls lifecycle of provisioned resources when unbound.
	DeprovisionPolicy *DeprovisionPolicy `json:"deprovisionPolicy,omitempty"`
}

// ResourceClaimPhase indicates coarse-grained resolver progress.
// Reasons/messages are abstract and must not leak runtime-specific nouns.
type ResourceClaimPhase string

const (
	ResourceClaimPhasePending  ResourceClaimPhase = "Pending"
	ResourceClaimPhaseClaiming ResourceClaimPhase = "Claiming"
	ResourceClaimPhaseBound    ResourceClaimPhase = "Bound"
	ResourceClaimPhaseFailed   ResourceClaimPhase = "Failed"
)

// LocalObjectReference references a namespaced local object by name.
type LocalObjectReference struct {
	// Name is the object name (namespace is implicit from the claim).
	Name string `json:"name"`
}

// CertificateOutput carries certificate material or a reference to where it is stored.
type CertificateOutput struct {
	// SecretName optionally points to a Secret containing certificate/key data.
	SecretName *string `json:"secretName,omitempty"`
	// Data allows inlined certificate/key material (base64-encoded in JSON).
	Data map[string][]byte `json:"data,omitempty"`
}

// ResourceClaimOutputs groups standardized outputs published by the resolver.
// At least one field must be set; platforms may define additional conventions by profile.
// +kubebuilder:validation:XValidation:rule="has(self.secretRef) || has(self.configMapRef) || has(self.uri) || has(self.image) || has(self.cert)",message="at least one of secretRef|configMapRef|uri|image|cert must be set"
type ResourceClaimOutputs struct {
	// SecretRef points to a Secret containing credentials or connection data.
	SecretRef *LocalObjectReference `json:"secretRef,omitempty"`
	// ConfigMapRef points to a ConfigMap containing configuration data.
	ConfigMapRef *LocalObjectReference `json:"configMapRef,omitempty"`
	// URI exposes a connection endpoint (e.g., jdbc:, redis:, https:).
	URI *string `json:"uri,omitempty"`
	// Image exposes container image reference for image-based resources.
	Image *string `json:"image,omitempty"`
	// Cert provides certificate/key material or a reference to it.
	Cert *CertificateOutput `json:"cert,omitempty"`
}

// ResourceClaimStatus is written by resolvers to report progress and outputs.
type ResourceClaimStatus struct {
	// Phase summarizes resolver progress (Pending/Claiming/Bound/Failed).
	// +kubebuilder:validation:Enum=Pending;Claiming;Bound;Failed
	Phase ResourceClaimPhase `json:"phase,omitempty"`
	// Reason is an abstract machine-readable reason; avoid runtime-specific nouns.
	Reason string `json:"reason,omitempty"`
	// Message is a short, single-sentence human message aligned with the reason.
	Message string `json:"message,omitempty"`
	// Outputs are standardized resolver outputs for consumption by runtimes.
	Outputs *ResourceClaimOutputs `json:"outputs,omitempty"`
	// OutputsAvailable indicates whether outputs are ready for consumption.
	OutputsAvailable bool `json:"outputsAvailable,omitempty"`

	// ObservedGeneration is the last reconciled spec generation.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastTransitionTime records when the phase last changed.
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// +kubebuilder:object:root=true
// ResourceClaimList contains a list of ResourceClaim.
type ResourceClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceClaim{}, &ResourceClaimList{})
}
