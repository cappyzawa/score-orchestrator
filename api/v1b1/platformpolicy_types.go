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

// PolicyTargeting selects the set of Workloads and Namespaces this policy applies to.
// Both selectors are optional; if omitted, the policy matches everything.
type PolicyTargeting struct {
	// WorkloadSelector selects Workloads by their labels (Kubernetes LabelSelector semantics).
	WorkloadSelector *metav1.LabelSelector `json:"workloadSelector,omitempty"`
	// NamespaceSelector selects Namespaces by their labels (Kubernetes LabelSelector semantics).
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// ReplicasDefault defines min/max/default replica hints used when a plan omits replicas.
// Runtimes may clamp to quotas; values are hints, not guarantees.
type ReplicasDefault struct {
	Min     *int32 `json:"min,omitempty"`
	Max     *int32 `json:"max,omitempty"`
	Default *int32 `json:"default,omitempty"`
}

// ExposureDefaults declares an abstract exposure strategy for endpoints (e.g. public/private/cluster).
// Concrete runtime details must not leak to user-visible status.
type ExposureDefaults struct {
	Strategy string `json:"strategy,omitempty"`
}

// PolicyDefaults groups platform-wide defaults applied during planning.
type PolicyDefaults struct {
	Replicas *ReplicasDefault  `json:"replicas,omitempty"`
	Exposure *ExposureDefaults `json:"exposure,omitempty"`
}

// ResolverClass selects a resolver implementation and optional parameters.
// The map key is a resource type (e.g. "postgresql"), resolved by platform policy.
type ResolverClass struct {
	// Class is the resolver class name understood by the platform (PF-facing, not user-facing).
	Class string `json:"class"`
	// Params are resolver-specific options (opaque to the orchestrator).
	Params *apiextv1.JSON `json:"params,omitempty"`
}

// ProjectionFrom declares a binding output to project into the workload (env/volumes/files).
// The tuple (bindingKey, outputKey) must refer to a produced binding output at runtime.
type ProjectionFrom struct {
	BindingKey string `json:"bindingKey"`
	OutputKey  string `json:"outputKey"`
}

// EnvProjectionRule maps a binding output to an environment variable.
type EnvProjectionRule struct {
	Name string         `json:"name"`
	From ProjectionFrom `json:"from"`
}

// VolumeProjectionRule maps a binding output to a volume (abstract, runtime-independent).
type VolumeProjectionRule struct {
	Name string          `json:"name,omitempty"`
	From *ProjectionFrom `json:"from,omitempty"`
}

// FileProjectionRule maps a binding output to a file path inside the container filesystem.
type FileProjectionRule struct {
	Path string          `json:"path,omitempty"`
	From *ProjectionFrom `json:"from,omitempty"`
}

// ProjectionDefaults provide default projection rules when the plan omits explicit mappings.
type ProjectionDefaults struct {
	Env     []EnvProjectionRule    `json:"env,omitempty"`
	Volumes []VolumeProjectionRule `json:"volumes,omitempty"`
	Files   []FileProjectionRule   `json:"files,omitempty"`
}

// EndpointPolicy defines how to derive a single canonical endpoint for a workload.
// The endpoint is user-visible as Workload.status.endpoint (format: uri).
type EndpointPolicy struct {
	// Template renders a URL; platforms may expose canonicity rules (e.g., host templating).
	Template *string `json:"template,omitempty"`
	// PreferHTTPS indicates that an HTTPS endpoint should be preferred when multiple exist.
	PreferHTTPS *bool `json:"preferHTTPS,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=ppol
// PlatformPolicy is a PF-facing cluster-scoped policy; it is hidden from regular users via RBAC.
// Orchestrator consumes it to apply defaults, select resolvers, and drive plan generation.
type PlatformPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PlatformPolicySpec `json:"spec"`
}

// PlatformPolicySpec holds targeting, runtime class selection, defaults, resolver routing,
// projection defaults, and endpoint derivation policy. No status subresource.
type PlatformPolicySpec struct {
	// RuntimeClass selects the runtime controller class (PF-facing identifier).
	RuntimeClass string `json:"runtimeClass,omitempty"`
	// Targeting restricts which Workloads/Namespaces the policy applies to.
	Targeting *PolicyTargeting `json:"targeting,omitempty"`
	// Defaults supplies platform-level defaults (applied during plan generation).
	Defaults *PolicyDefaults `json:"defaults,omitempty"`
	// ResolverRouting maps resource type -> resolver class/params.
	ResolverRouting map[string]ResolverClass `json:"resolverRouting,omitempty"`
	// ProjectionDefaults define default env/volume/file mappings from binding outputs.
	ProjectionDefaults *ProjectionDefaults `json:"projectionDefaults,omitempty"`
	// EndpointPolicy defines how to compute a single canonical endpoint for the workload.
	EndpointPolicy *EndpointPolicy `json:"endpointPolicy,omitempty"`
}

// +kubebuilder:object:root=true
// PlatformPolicyList is a list of PlatformPolicy.
type PlatformPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlatformPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlatformPolicy{}, &PlatformPolicyList{})
}
