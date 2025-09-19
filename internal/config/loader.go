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

package config

import (
	"context"
	"errors"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

var (
	// ErrConfigNotFound is returned when configuration is not found
	ErrConfigNotFound = errors.New("orchestrator configuration not found")

	// ErrConfigInvalid is returned when configuration is invalid
	ErrConfigInvalid = errors.New("orchestrator configuration is invalid")

	// ErrConfigMalformed is returned when configuration cannot be parsed
	ErrConfigMalformed = errors.New("orchestrator configuration is malformed")
)

// ConfigLoader defines the interface for loading and watching Orchestrator Configuration
type ConfigLoader interface {
	// Load loads the orchestrator configuration from the configured source
	Load(ctx context.Context) (*scorev1b1.OrchestratorConfig, error)

	// Watch returns a channel that receives configuration updates when changes occur
	// The channel is closed when the context is cancelled or an error occurs
	Watch(ctx context.Context) (<-chan ConfigEvent, error)

	// Close releases any resources held by the loader
	Close() error
}

// ConfigEvent represents a configuration change event
type ConfigEvent struct {
	// Type indicates the type of event (Added, Modified, Deleted, Error)
	Type ConfigEventType

	// Config is the updated configuration (nil for Delete and Error events)
	Config *scorev1b1.OrchestratorConfig

	// Error is the error that occurred (only set for Error events)
	Error error
}

// ConfigEventType represents the type of configuration event
type ConfigEventType string

const (
	// ConfigEventAdded indicates a new configuration was added
	ConfigEventAdded ConfigEventType = "Added"

	// ConfigEventModified indicates an existing configuration was modified
	ConfigEventModified ConfigEventType = "Modified"

	// ConfigEventDeleted indicates a configuration was deleted
	ConfigEventDeleted ConfigEventType = "Deleted"

	// ConfigEventError indicates an error occurred while watching
	ConfigEventError ConfigEventType = "Error"
)

// LoaderOptions contains configuration options for creating a ConfigLoader
type LoaderOptions struct {
	// Namespace is the Kubernetes namespace where the ConfigMap is located
	// Defaults to "score-system"
	Namespace string

	// ConfigMapName is the name of the ConfigMap containing the configuration
	// Defaults to "orchestrator-config"
	ConfigMapName string

	// ConfigMapKey is the key within the ConfigMap that contains the YAML configuration
	// Defaults to "config.yaml"
	ConfigMapKey string

	// EnableCache enables in-memory caching of the configuration
	// Defaults to true
	EnableCache bool

	// CacheTTL is the time-to-live for cached configuration
	// Defaults to 5 minutes. Only used if EnableCache is true
	CacheTTL string
}

// DefaultLoaderOptions returns the default loader options
func DefaultLoaderOptions() LoaderOptions {
	return LoaderOptions{
		Namespace:     "kbinit-system",
		ConfigMapName: "orchestrator-config",
		ConfigMapKey:  "config.yaml",
		EnableCache:   true,
		CacheTTL:      "5m",
	}
}
