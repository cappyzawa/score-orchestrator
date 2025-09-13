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
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// configCache provides in-memory caching for orchestrator configuration
type configCache struct {
	mu       sync.RWMutex
	config   *scorev1b1.OrchestratorConfig
	cachedAt time.Time
	ttl      time.Duration
}

// newConfigCache creates a new configuration cache with the specified TTL
func newConfigCache(ttl time.Duration) *configCache {
	return &configCache{
		ttl: ttl,
	}
}

// get retrieves the cached configuration if it's still valid
func (c *configCache) get() *scorev1b1.OrchestratorConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.config == nil {
		return nil
	}

	// Check if cache has expired
	if time.Since(c.cachedAt) > c.ttl {
		return nil
	}

	// Return a deep copy to prevent modification of cached data
	return c.deepCopyConfig(c.config)
}

// set stores a configuration in the cache
func (c *configCache) set(config *scorev1b1.OrchestratorConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store a deep copy to prevent external modifications
	c.config = c.deepCopyConfig(config)
	c.cachedAt = time.Now()
}

// invalidate removes the cached configuration
func (c *configCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config = nil
	c.cachedAt = time.Time{}
}

// isValid checks if the cache is valid (not expired)
func (c *configCache) isValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.config == nil {
		return false
	}

	return time.Since(c.cachedAt) <= c.ttl
}

// deepCopyConfig creates a deep copy of an OrchestratorConfig
// This is a simplified version - in production, consider using a more robust deep copy library
func (c *configCache) deepCopyConfig(original *scorev1b1.OrchestratorConfig) *scorev1b1.OrchestratorConfig {
	if original == nil {
		return nil
	}

	copy := &scorev1b1.OrchestratorConfig{
		APIVersion: original.APIVersion,
		Kind:       original.Kind,
		Metadata:   original.Metadata,
		Spec: scorev1b1.OrchestratorConfigSpec{
			Defaults: c.deepCopyDefaults(original.Spec.Defaults),
		},
	}

	// Deep copy profiles
	if len(original.Spec.Profiles) > 0 {
		copy.Spec.Profiles = make([]scorev1b1.ProfileSpec, len(original.Spec.Profiles))
		for i, profile := range original.Spec.Profiles {
			copy.Spec.Profiles[i] = c.deepCopyProfile(profile)
		}
	}

	// Deep copy provisioners
	if len(original.Spec.Provisioners) > 0 {
		copy.Spec.Provisioners = make([]scorev1b1.ProvisionerSpec, len(original.Spec.Provisioners))
		for i, provisioner := range original.Spec.Provisioners {
			copy.Spec.Provisioners[i] = c.deepCopyProvisioner(provisioner)
		}
	}

	return copy
}

// deepCopyProfile creates a deep copy of a ProfileSpec
func (c *configCache) deepCopyProfile(original scorev1b1.ProfileSpec) scorev1b1.ProfileSpec {
	copy := scorev1b1.ProfileSpec{
		Name:        original.Name,
		Description: original.Description,
	}

	if len(original.Backends) > 0 {
		copy.Backends = make([]scorev1b1.BackendSpec, len(original.Backends))
		for i, backend := range original.Backends {
			copy.Backends[i] = c.deepCopyBackend(backend)
		}
	}

	return copy
}

// deepCopyBackend creates a deep copy of a BackendSpec
func (c *configCache) deepCopyBackend(original scorev1b1.BackendSpec) scorev1b1.BackendSpec {
	copy := scorev1b1.BackendSpec{
		BackendId:    original.BackendId,
		RuntimeClass: original.RuntimeClass,
		Template:     c.deepCopyTemplate(original.Template),
		Priority:     original.Priority,
		Version:      original.Version,
	}

	if original.Constraints != nil {
		copy.Constraints = c.deepCopyConstraints(*original.Constraints)
	}

	return copy
}

// deepCopyTemplate creates a deep copy of a TemplateSpec
func (c *configCache) deepCopyTemplate(original scorev1b1.TemplateSpec) scorev1b1.TemplateSpec {
	copy := scorev1b1.TemplateSpec{
		Kind: original.Kind,
		Ref:  original.Ref,
	}

	if original.Values != nil {
		copy.Values = original.Values.DeepCopy()
	}

	return copy
}

// deepCopyConstraints creates a deep copy of a ConstraintsSpec
func (c *configCache) deepCopyConstraints(original scorev1b1.ConstraintsSpec) *scorev1b1.ConstraintsSpec {
	copySpec := &scorev1b1.ConstraintsSpec{}

	if len(original.Selectors) > 0 {
		copySpec.Selectors = make([]scorev1b1.SelectorSpec, len(original.Selectors))
		for i, selector := range original.Selectors {
			copySpec.Selectors[i] = c.deepCopySelector(selector)
		}
	}

	if len(original.Features) > 0 {
		copySpec.Features = make([]string, len(original.Features))
		copy(copySpec.Features, original.Features)
	}

	if len(original.Regions) > 0 {
		copySpec.Regions = make([]string, len(original.Regions))
		copy(copySpec.Regions, original.Regions)
	}

	if original.Resources != nil {
		copySpec.Resources = &scorev1b1.ResourceConstraints{
			CPU:     original.Resources.CPU,
			Memory:  original.Resources.Memory,
			Storage: original.Resources.Storage,
		}
	}

	return copySpec
}

// deepCopySelector creates a deep copy of a SelectorSpec
func (c *configCache) deepCopySelector(original scorev1b1.SelectorSpec) scorev1b1.SelectorSpec {
	copy := scorev1b1.SelectorSpec{
		Profile: original.Profile,
	}

	if original.MatchLabels != nil {
		copy.MatchLabels = make(map[string]string, len(original.MatchLabels))
		for k, v := range original.MatchLabels {
			copy.MatchLabels[k] = v
		}
	}

	if len(original.MatchExpressions) > 0 {
		copy.MatchExpressions = make([]metav1.LabelSelectorRequirement, len(original.MatchExpressions))
		for i, expr := range original.MatchExpressions {
			copy.MatchExpressions[i] = metav1.LabelSelectorRequirement{
				Key:      expr.Key,
				Operator: expr.Operator,
				Values:   append([]string(nil), expr.Values...),
			}
		}
	}

	if original.Constraints != nil {
		copy.Constraints = c.deepCopyConstraints(*original.Constraints)
	}

	return copy
}

// deepCopyProvisioner creates a deep copy of a ProvisionerSpec
func (c *configCache) deepCopyProvisioner(original scorev1b1.ProvisionerSpec) scorev1b1.ProvisionerSpec {
	copy := scorev1b1.ProvisionerSpec{
		Type:        original.Type,
		Provisioner: original.Provisioner,
	}

	if len(original.Classes) > 0 {
		copy.Classes = make([]scorev1b1.ClassSpec, len(original.Classes))
		for i, class := range original.Classes {
			copy.Classes[i] = c.deepCopyClass(class)
		}
	}

	if original.Defaults != nil {
		copy.Defaults = &scorev1b1.ProvisionerDefaults{
			Class: original.Defaults.Class,
		}
		if original.Defaults.Params != nil {
			copy.Defaults.Params = original.Defaults.Params.DeepCopy()
		}
	}

	return copy
}

// deepCopyClass creates a deep copy of a ClassSpec
func (c *configCache) deepCopyClass(original scorev1b1.ClassSpec) scorev1b1.ClassSpec {
	copy := scorev1b1.ClassSpec{
		Name:        original.Name,
		Description: original.Description,
	}

	if original.Parameters != nil {
		copy.Parameters = original.Parameters.DeepCopy()
	}

	if original.Constraints != nil {
		copy.Constraints = c.deepCopyConstraints(*original.Constraints)
	}

	return copy
}

// deepCopyDefaults creates a deep copy of a DefaultsSpec
func (c *configCache) deepCopyDefaults(original scorev1b1.DefaultsSpec) scorev1b1.DefaultsSpec {
	copy := scorev1b1.DefaultsSpec{
		Profile: original.Profile,
	}

	if len(original.Selectors) > 0 {
		copy.Selectors = make([]scorev1b1.SelectorSpec, len(original.Selectors))
		for i, selector := range original.Selectors {
			copy.Selectors[i] = c.deepCopySelector(selector)
		}
	}

	return copy
}

// Note: deepCopyMap and deepCopyInterface functions were removed as we now use
// runtime.RawExtension for flexible data types which has its own DeepCopy method
