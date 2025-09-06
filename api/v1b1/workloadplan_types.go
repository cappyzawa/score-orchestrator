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
// +kubebuilder:resource:scope=Namespaced,shortName=wplan
// WorkloadPlan is an internal contract from Orchestrator to the Runtime controller.
// It must not be user-visible via RBAC; status is intentionally omitted.
type WorkloadPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkloadPlanSpec `json:"spec"`
}

// WorkloadPlanWorkloadRef identifies the Workload this plan was derived from.
type WorkloadPlanWorkloadRef struct {
	// Name is the Workload name.
	Name string `json:"name"`
	// Namespace is the Workload namespace.
	Namespace string `json:"namespace"`
}

// FromBindingOutput references a single output key produced by a binding.
type FromBindingOutput struct {
	// BindingKey is the key under Workload.spec.resources (e.g., "db").
	BindingKey string `json:"bindingKey"`
	// OutputKey is the output field exported by the binding (e.g., "uri").
	OutputKey string `json:"outputKey"`
}

// EnvMapping projects a binding output into an environment variable.
type EnvMapping struct {
	// Name is the environment variable name.
	Name string `json:"name"`
	// From selects the binding output to inject.
	From FromBindingOutput `json:"from"`
}

// VolumeProjection projects a binding output into a volume (abstract).
type VolumeProjection struct {
	// Name is a logical volume name.
	Name string `json:"name,omitempty"`
	// From selects the binding output to project into the volume.
	From *FromBindingOutput `json:"from,omitempty"`
}

// FileProjection projects a binding output into a file path (abstract).
type FileProjection struct {
	// Path is a file path inside the container filesystem.
	Path string `json:"path,omitempty"`
	// From selects the binding output to write to the path.
	From *FromBindingOutput `json:"from,omitempty"`
}

// WorkloadProjection configures how binding outputs are injected into the workload.
type WorkloadProjection struct {
	Env     []EnvMapping       `json:"env,omitempty"`
	Volumes []VolumeProjection `json:"volumes,omitempty"`
	Files   []FileProjection   `json:"files,omitempty"`
}

// PlanBinding declares a binding requirement passed to the runtime controller.
// It mirrors the resolution keys (type/class/params) used by resolvers.
type PlanBinding struct {
	// Key is the logical key (e.g., "db", "cache").
	Key string `json:"key"`
	// Type is the abstract resource type (e.g., "postgresql").
	Type string `json:"type"`
	// Class optionally refines the type (PF-specific resolver class).
	Class *string `json:"class,omitempty"`
	// Params are opaque parameters used by the selected resolver/runtime.
	Params *apiextv1.JSON `json:"params,omitempty"`
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
	// Projection defines how binding outputs are injected into the workload.
	Projection WorkloadProjection `json:"projection,omitempty"`
	// Bindings declares resource requirements to be materialized by the runtime.
	Bindings []PlanBinding `json:"bindings,omitempty"`
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
