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

// OrchestratorConfig represents the configuration for the Score Orchestrator
// This is NOT a CRD but a configuration format that can be distributed via ConfigMap or OCI artifact
type OrchestratorConfig struct {
	APIVersion string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                 `json:"kind" yaml:"kind"`
	Metadata   OrchestratorConfigMeta `json:"metadata" yaml:"metadata"`
	Spec       OrchestratorConfigSpec `json:"spec" yaml:"spec"`
	Reconciler *runtime.RawExtension  `json:"reconciler,omitempty" yaml:"reconciler,omitempty"`
}

// OrchestratorConfigMeta contains metadata for the orchestrator configuration
type OrchestratorConfigMeta struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// OrchestratorConfigSpec defines the specification for orchestrator configuration
type OrchestratorConfigSpec struct {
	// Profiles defines abstract workload profiles and their backend mappings
	Profiles []ProfileSpec `json:"profiles" yaml:"profiles"`

	// Provisioners defines how dependency resources are provisioned
	Provisioners []ProvisionerSpec `json:"provisioners" yaml:"provisioners"`

	// Defaults defines default values and selection policies
	Defaults DefaultsSpec `json:"defaults" yaml:"defaults"`
}

// ProfileSpec defines an abstract workload profile
type ProfileSpec struct {
	// Name is the abstract profile name (e.g., "web-service")
	Name string `json:"name" yaml:"name"`

	// Description is an optional human-readable description
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Backends is an array of backend implementations for this profile
	Backends []BackendSpec `json:"backends" yaml:"backends"`
}

// BackendSpec represents a concrete runtime implementation for a profile
type BackendSpec struct {
	// BackendId is a stable identifier (not user-visible)
	BackendId string `json:"backendId" yaml:"backendId"`

	// RuntimeClass is the runtime class (e.g., "kubernetes", "ecs", "nomad")
	RuntimeClass string `json:"runtimeClass" yaml:"runtimeClass"`

	// Template defines how to materialize this backend
	Template TemplateSpec `json:"template" yaml:"template"`

	// Priority is the selection priority (higher = preferred)
	Priority int `json:"priority" yaml:"priority"`

	// Version is the backend version (semver recommended)
	Version string `json:"version" yaml:"version"`

	// Constraints define selection constraints for this backend
	Constraints *ConstraintsSpec `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}

// TemplateSpec defines template configuration for backend materialization
type TemplateSpec struct {
	// Kind is the template type: "manifests" | "helm" | "kustomize"
	Kind string `json:"kind" yaml:"kind"`

	// Ref is the immutable reference (OCI digest recommended)
	Ref string `json:"ref" yaml:"ref"`

	// Values are optional default template values
	Values *runtime.RawExtension `json:"values,omitempty" yaml:"values,omitempty"`
}

// ConstraintsSpec defines constraints for backend selection
type ConstraintsSpec struct {
	// Selectors are label selectors for conditional constraints
	Selectors []SelectorSpec `json:"selectors,omitempty" yaml:"selectors,omitempty"`

	// Features are required features for this backend
	Features []string `json:"features,omitempty" yaml:"features,omitempty"`

	// Regions are allowed regions for this backend
	Regions []string `json:"regions,omitempty" yaml:"regions,omitempty"`

	// Resources define resource constraints
	Resources *ResourceConstraints `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// ResourceConstraints define resource limits for backends
type ResourceConstraints struct {
	// CPU constraint in format "100m-4000m"
	CPU string `json:"cpu,omitempty" yaml:"cpu,omitempty"`

	// Memory constraint in format "128Mi-8Gi"
	Memory string `json:"memory,omitempty" yaml:"memory,omitempty"`

	// Storage constraint in format "1Gi-100Gi"
	Storage string `json:"storage,omitempty" yaml:"storage,omitempty"`
}

// ProvisionerSpec defines how dependency resources are provisioned
type ProvisionerSpec struct {
	// Type is the resource type (e.g., "postgres", "redis", "s3")
	Type string `json:"type" yaml:"type"`

	// Provisioner is the controller name/identifier (deprecated, use Config instead)
	Provisioner string `json:"provisioner,omitempty" yaml:"provisioner,omitempty"`

	// Config defines configuration-driven provisioning settings
	Config *ProvisionerConfig `json:"config,omitempty" yaml:"config,omitempty"`

	// Classes are available service tiers/sizes for this resource type
	Classes []ClassSpec `json:"classes" yaml:"classes"`

	// Defaults are default parameters for this provisioner
	Defaults *ProvisionerDefaults `json:"defaults,omitempty" yaml:"defaults,omitempty"`
}

// ProvisionerConfig defines configuration-driven provisioning settings
type ProvisionerConfig struct {
	// Strategy defines the provisioning strategy: "helm" | "manifests" | "external-api"
	Strategy string `json:"strategy" yaml:"strategy"`

	// Helm defines Helm chart deployment configuration (used when strategy=helm)
	Helm *HelmStrategy `json:"helm,omitempty" yaml:"helm,omitempty"`

	// Manifests defines Kubernetes manifest application (used when strategy=manifests)
	Manifests []runtime.RawExtension `json:"manifests,omitempty" yaml:"manifests,omitempty"`

	// ExternalApi defines external API provisioning configuration (used when strategy=external-api)
	ExternalApi *ExternalApiStrategy `json:"externalApi,omitempty" yaml:"externalApi,omitempty"`

	// Outputs defines how to generate ResourceClaim outputs
	Outputs map[string]string `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}

// HelmStrategy defines Helm chart deployment configuration
type HelmStrategy struct {
	// Chart is the Helm chart name (e.g., "bitnami/postgresql")
	Chart string `json:"chart" yaml:"chart"`

	// Repository is the Helm chart repository URL
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`

	// Version is the chart version to use
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// Values are chart values with template substitution support
	Values *runtime.RawExtension `json:"values,omitempty" yaml:"values,omitempty"`
}

// ExternalApiStrategy defines external API provisioning configuration
type ExternalApiStrategy struct {
	// Endpoint is the external API endpoint URL
	Endpoint string `json:"endpoint" yaml:"endpoint"`

	// Method is the HTTP method (default: POST)
	Method string `json:"method,omitempty" yaml:"method,omitempty"`

	// Path is the API path to append to endpoint
	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	// Auth defines authentication configuration
	Auth *ApiAuth `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Request defines the request body template
	Request *runtime.RawExtension `json:"request,omitempty" yaml:"request,omitempty"`

	// Polling defines how to poll for completion
	Polling *ApiPolling `json:"polling,omitempty" yaml:"polling,omitempty"`
}

// ApiAuth defines authentication for external APIs
type ApiAuth struct {
	// Type defines the auth type: "api-key" | "aws-iam" | "bearer-token"
	Type string `json:"type" yaml:"type"`

	// SecretRef references a secret containing auth credentials
	SecretRef string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`

	// RoleArn is used for AWS IAM authentication
	RoleArn string `json:"roleArn,omitempty" yaml:"roleArn,omitempty"`
}

// ApiPolling defines polling configuration for external APIs
type ApiPolling struct {
	// StatusField is the response field to check for status
	StatusField string `json:"statusField" yaml:"statusField"`

	// ReadyValue is the value indicating completion
	ReadyValue string `json:"readyValue" yaml:"readyValue"`

	// Interval is the polling interval (e.g., "30s")
	Interval string `json:"interval" yaml:"interval"`

	// Timeout is the maximum time to wait (e.g., "20m")
	Timeout string `json:"timeout" yaml:"timeout"`
}

// ClassSpec defines available service tiers/sizes for a resource type
type ClassSpec struct {
	// Name is the class identifier (e.g., "small", "large", "enterprise")
	Name string `json:"name" yaml:"name"`

	// Description is a human-readable description
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Parameters are class-specific parameters
	Parameters *runtime.RawExtension `json:"parameters,omitempty" yaml:"parameters,omitempty"`

	// Constraints define access constraints for this class
	Constraints *ConstraintsSpec `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}

// ProvisionerDefaults define default parameters for a provisioner
type ProvisionerDefaults struct {
	// Class is the default class name
	Class string `json:"class,omitempty" yaml:"class,omitempty"`

	// Params are default parameters
	Params *runtime.RawExtension `json:"params,omitempty" yaml:"params,omitempty"`
}

// DefaultsSpec defines default values and selection policies
type DefaultsSpec struct {
	// Profile is the global default profile
	Profile string `json:"profile" yaml:"profile"`

	// Selectors are conditional defaults based on label selectors
	Selectors []SelectorSpec `json:"selectors,omitempty" yaml:"selectors,omitempty"`
}

// SelectorSpec defines Kubernetes-style label selectors for conditional configuration
type SelectorSpec struct {
	// MatchLabels is a map of exact label matches
	MatchLabels map[string]string `json:"matchLabels,omitempty" yaml:"matchLabels,omitempty"`

	// MatchExpressions is an array of label selector requirements
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty" yaml:"matchExpressions,omitempty"`

	// Profile is the profile to use when this selector matches
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`

	// Constraints are additional constraints when this selector matches
	Constraints *ConstraintsSpec `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}
