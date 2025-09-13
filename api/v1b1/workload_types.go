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

// ContainerSpec defines a container within the workload
type ContainerSpec struct {
	// Image is the container image reference. Use "." for build-from-source
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Command to run in the container
	// +optional
	Command []string `json:"command,omitempty"`

	// Args for the container command
	// +optional
	Args []string `json:"args,omitempty"`

	// Variables define environment variables for the container
	// +optional
	Variables map[string]string `json:"variables,omitempty"`

	// Files to mount in the container
	// +optional
	// +kubebuilder:validation:MaxItems=20
	Files []FileSpec `json:"files,omitempty"`

	// LivenessProbe defines the liveness probe for the container
	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`

	// ReadinessProbe defines the readiness probe for the container
	// +optional
	ReadinessProbe *ProbeSpec `json:"readinessProbe,omitempty"`

	// Resources defines compute resource requirements
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

// FileSpec defines a file to be mounted in the container
type FileSpec struct {
	// Target path where the file should be mounted
	// +kubebuilder:validation:MinLength=1
	Target string `json:"target"`

	// Mode is the file permission mode
	// +optional
	Mode *string `json:"mode,omitempty"`

	// Source references an external file source
	// +optional
	Source *FileSourceSpec `json:"source,omitempty"`

	// Content is the inline file content
	// +optional
	Content *string `json:"content,omitempty"`

	// BinaryContent is base64-encoded binary file content
	// +optional
	BinaryContent *string `json:"binaryContent,omitempty"`
}

// FileSourceSpec defines an external file source
type FileSourceSpec struct {
	// URI of the external file source
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
}

// ProbeSpec defines a health check probe
type ProbeSpec struct {
	// HTTPGet specifies HTTP probe
	// +optional
	HTTPGet *HTTPGetProbe `json:"httpGet,omitempty"`

	// Exec specifies exec probe
	// +optional
	Exec *ExecProbe `json:"exec,omitempty"`
}

// HTTPGetProbe defines an HTTP health check
type HTTPGetProbe struct {
	// Path to access on the HTTP server
	// +optional
	Path string `json:"path,omitempty"`

	// Port to access on the container
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// HTTP headers to send with the request
	// +optional
	Headers map[string]string `json:"headers,omitempty"`
}

// ExecProbe defines a command execution health check
type ExecProbe struct {
	// Command to execute
	// +kubebuilder:validation:MinItems=1
	Command []string `json:"command"`
}

// ResourceRequirements defines compute resource requirements
type ResourceRequirements struct {
	// Limits defines maximum resource usage
	// +optional
	Limits map[string]string `json:"limits,omitempty"`

	// Requests defines minimum required resources
	// +optional
	Requests map[string]string `json:"requests,omitempty"`
}

// ServiceSpec defines the service configuration for the workload
type ServiceSpec struct {
	// Ports define the service ports
	// +optional
	Ports []ServicePort `json:"ports,omitempty"`
}

// ServicePort defines a service port
type ServicePort struct {
	// Port is the service port number
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Protocol is the port protocol
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default="TCP"
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// TargetPort is the container port to forward to
	// +optional
	TargetPort *int32 `json:"targetPort,omitempty"`
}

// ResourceSpec defines an external resource dependency
type ResourceSpec struct {
	// Type of the resource (e.g., "postgresql", "redis", "s3")
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`

	// Class specifies the resource class or implementation
	// +optional
	Class *string `json:"class,omitempty"`

	// Params are resource-specific parameters
	// +optional
	Params *apiextv1.JSON `json:"params,omitempty"`
}

// WorkloadSpec defines the desired state of Workload
type WorkloadSpec struct {
	// Containers define the containers in the workload
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=10
	Containers map[string]ContainerSpec `json:"containers"`

	// Service defines the service configuration
	// +optional
	Service *ServiceSpec `json:"service,omitempty"`

	// Resources define external resource dependencies
	// +optional
	Resources map[string]ResourceSpec `json:"resources,omitempty"`
}

// BindingSummary provides a summary of a resource binding status
type BindingSummary struct {
	// Key identifies the resource binding
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`

	// Phase indicates the current phase of the binding
	// +kubebuilder:validation:Enum=Pending;Binding;Bound;Failed
	Phase ResourceClaimPhase `json:"phase"`

	// Reason provides a programmatic identifier for the binding status
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message provides a human-readable description of the binding status
	// +optional
	Message string `json:"message,omitempty"`

	// OutputsAvailable indicates whether the binding outputs are available
	// +optional
	OutputsAvailable bool `json:"outputsAvailable,omitempty"`
}

// WorkloadStatus defines the observed state of Workload.
type WorkloadStatus struct {
	// Endpoint is the primary URI for accessing the workload
	// +kubebuilder:validation:Format=uri
	// +optional
	Endpoint *string `json:"endpoint,omitempty"`

	// Conditions represent the current state of the Workload resource.
	// Standard condition types:
	// - "Ready": the workload is fully functional
	// - "ClaimsReady": all resource claims are ready
	// - "RuntimeReady": runtime environment is ready
	// - "InputsValid": spec validation passed
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Bindings provide a summary of resource binding statuses
	// +optional
	Bindings []BindingSummary `json:"bindings,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=wl
// +kubebuilder:subresource:status

// Workload is the Schema for the workloads API
type Workload struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Workload
	// +required
	Spec WorkloadSpec `json:"spec"`

	// status defines the observed state of Workload
	// +optional
	Status WorkloadStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// WorkloadList contains a list of Workload
type WorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workload{}, &WorkloadList{})
}
