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
	"testing"
	"time"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

func TestConfigCache_GetSet(t *testing.T) {
	cache := newConfigCache(1 * time.Minute)

	// Initially cache should be empty
	config := cache.get()
	if config != nil {
		t.Errorf("Expected nil config from empty cache, got %v", config)
	}

	// Set a configuration
	testConfig := &scorev1b1.OrchestratorConfig{
		APIVersion: "score.dev/v1b1",
		Kind:       "OrchestratorConfig",
		Metadata: scorev1b1.OrchestratorConfigMeta{
			Name:    "test-config",
			Version: "1.0.0",
		},
		Spec: scorev1b1.OrchestratorConfigSpec{
			Defaults: scorev1b1.DefaultsSpec{
				Profile: "test-profile",
			},
		},
	}

	cache.set(testConfig)

	// Retrieve the configuration
	retrievedConfig := cache.get()
	if retrievedConfig == nil {
		t.Fatalf("Expected config from cache, got nil")
	}

	// Verify the configuration content
	if retrievedConfig.APIVersion != testConfig.APIVersion {
		t.Errorf("APIVersion = %v, want %v", retrievedConfig.APIVersion, testConfig.APIVersion)
	}
	if retrievedConfig.Kind != testConfig.Kind {
		t.Errorf("Kind = %v, want %v", retrievedConfig.Kind, testConfig.Kind)
	}
	if retrievedConfig.Metadata.Name != testConfig.Metadata.Name {
		t.Errorf("Metadata.Name = %v, want %v", retrievedConfig.Metadata.Name, testConfig.Metadata.Name)
	}
	if retrievedConfig.Spec.Defaults.Profile != testConfig.Spec.Defaults.Profile {
		t.Errorf("Spec.Defaults.Profile = %v, want %v", retrievedConfig.Spec.Defaults.Profile, testConfig.Spec.Defaults.Profile)
	}

	// Verify that modification of returned config doesn't affect cached config
	retrievedConfig.Metadata.Name = "modified-name"
	secondRetrieved := cache.get()
	if secondRetrieved.Metadata.Name != "test-config" {
		t.Errorf("Cache was affected by modification of returned config")
	}
}

func TestConfigCache_TTLExpiration(t *testing.T) {
	cache := newConfigCache(100 * time.Millisecond) // Very short TTL for testing

	testConfig := &scorev1b1.OrchestratorConfig{
		APIVersion: "score.dev/v1b1",
		Kind:       "OrchestratorConfig",
		Metadata: scorev1b1.OrchestratorConfigMeta{
			Name: "test-config",
		},
	}

	// Set the configuration
	cache.set(testConfig)

	// Should be valid immediately
	if !cache.isValid() {
		t.Errorf("Cache should be valid immediately after set")
	}

	config := cache.get()
	if config == nil {
		t.Errorf("Should get config before TTL expiration")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be invalid after TTL expiration
	if cache.isValid() {
		t.Errorf("Cache should be invalid after TTL expiration")
	}

	config = cache.get()
	if config != nil {
		t.Errorf("Should get nil config after TTL expiration, got %v", config)
	}
}

func TestConfigCache_Invalidate(t *testing.T) {
	cache := newConfigCache(1 * time.Minute)

	testConfig := &scorev1b1.OrchestratorConfig{
		APIVersion: "score.dev/v1b1",
		Kind:       "OrchestratorConfig",
		Metadata: scorev1b1.OrchestratorConfigMeta{
			Name: "test-config",
		},
	}

	// Set the configuration
	cache.set(testConfig)

	// Verify it's cached
	config := cache.get()
	if config == nil {
		t.Errorf("Should get config after set")
	}

	// Invalidate the cache
	cache.invalidate()

	// Should be invalid after invalidation
	if cache.isValid() {
		t.Errorf("Cache should be invalid after invalidation")
	}

	// Should return nil after invalidation
	config = cache.get()
	if config != nil {
		t.Errorf("Should get nil config after invalidation, got %v", config)
	}
}

func TestConfigCache_DeepCopy(t *testing.T) {
	cache := newConfigCache(1 * time.Minute)

	// Create a config with nested structures
	testConfig := &scorev1b1.OrchestratorConfig{
		APIVersion: "score.dev/v1b1",
		Kind:       "OrchestratorConfig",
		Metadata: scorev1b1.OrchestratorConfigMeta{
			Name:    "test-config",
			Version: "1.0.0",
		},
		Spec: scorev1b1.OrchestratorConfigSpec{
			Profiles: []scorev1b1.ProfileSpec{
				{
					Name:        "web-service",
					Description: "Web service profile",
					Backends: []scorev1b1.BackendSpec{
						{
							BackendId:    "k8s-web-1",
							RuntimeClass: "kubernetes",
							Template: scorev1b1.TemplateSpec{
								Kind:   "manifests",
								Ref:    "registry.example.com/templates/web",
								Values: nil, // Simplified for test
							},
							Priority: 100,
							Version:  "1.0.0",
						},
					},
				},
			},
			Defaults: scorev1b1.DefaultsSpec{
				Profile: "web-service",
			},
		},
	}

	cache.set(testConfig)

	// Get the config and modify it
	retrievedConfig := cache.get()
	if retrievedConfig == nil {
		t.Fatalf("Should get config from cache")
	}

	// Modify the retrieved config
	retrievedConfig.Metadata.Name = "modified-name"
	retrievedConfig.Spec.Profiles[0].Name = "modified-profile"
	retrievedConfig.Spec.Profiles[0].Backends[0].BackendId = "modified-backend"

	// Get again and verify the cached config is unchanged
	secondRetrieved := cache.get()
	if secondRetrieved == nil {
		t.Fatalf("Should get config from cache again")
	}

	if secondRetrieved.Metadata.Name != "test-config" {
		t.Errorf("Cached config metadata was modified: %v", secondRetrieved.Metadata.Name)
	}
	if secondRetrieved.Spec.Profiles[0].Name != "web-service" {
		t.Errorf("Cached config profile was modified: %v", secondRetrieved.Spec.Profiles[0].Name)
	}
	if secondRetrieved.Spec.Profiles[0].Backends[0].BackendId != "k8s-web-1" {
		t.Errorf("Cached config backend was modified: %v", secondRetrieved.Spec.Profiles[0].Backends[0].BackendId)
	}

	// Template values deep copy testing is simplified since we're using RawExtension
	// In real usage, values would be properly marshaled/unmarshaled
}

func TestConfigCache_ConcurrentAccess(t *testing.T) {
	cache := newConfigCache(1 * time.Minute)

	testConfig := &scorev1b1.OrchestratorConfig{
		APIVersion: "score.dev/v1b1",
		Kind:       "OrchestratorConfig",
		Metadata: scorev1b1.OrchestratorConfigMeta{
			Name: "test-config",
		},
	}

	// Test concurrent reads and writes
	done := make(chan bool, 10)

	// Start multiple goroutines reading from cache
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				config := cache.get()
				_ = config // Use the config to avoid compiler optimization
			}
			done <- true
		}()
	}

	// Start multiple goroutines writing to cache
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				modifiedConfig := &scorev1b1.OrchestratorConfig{
					APIVersion: "score.dev/v1b1",
					Kind:       "OrchestratorConfig",
					Metadata: scorev1b1.OrchestratorConfigMeta{
						Name: "test-config",
					},
				}
				cache.set(modifiedConfig)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify cache is still functional after concurrent access
	cache.set(testConfig)
	config := cache.get()
	if config == nil {
		t.Errorf("Cache should be functional after concurrent access")
	}
}
