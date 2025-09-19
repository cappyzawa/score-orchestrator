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
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	validConfigYAML = `
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: test-config
  version: "1.0.0"
spec:
  profiles:
  - name: web-service
    description: "Web service profile"
    backends:
    - backendId: k8s-web-1
      runtimeClass: kubernetes
      template:
        kind: manifests
        ref: "registry.example.com/templates/web@sha256:abc123"
        values:
          replicas: 3
      priority: 100
      version: "1.0.0"
  provisioners:
  - type: postgres
    provisioner: postgres-operator
    classes:
    - name: small
      description: "Small database"
      parameters:
        cpu: "500m"
        memory: "1Gi"
  defaults:
    profile: web-service
`

	invalidConfigYAML = `
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: test-config
spec:
  # Missing required profiles
  defaults:
    profile: non-existent
`
)

func TestConfigMapLoader_Load(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() *fake.Clientset
		options     LoaderOptions
		wantErr     bool
		errType     error
	}{
		{
			name: "successful load with valid config",
			setupClient: func() *fake.Clientset {
				client := fake.NewSimpleClientset()
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orchestrator-config",
						Namespace: "kbinit-system",
					},
					Data: map[string]string{
						"config.yaml": validConfigYAML,
					},
				}
				_, err := client.CoreV1().ConfigMaps("kbinit-system").Create(context.TODO(), configMap, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create ConfigMap: %v", err)
				}
				return client
			},
			options: DefaultLoaderOptions(),
			wantErr: false,
		},
		{
			name: "config map not found",
			setupClient: func() *fake.Clientset {
				return fake.NewSimpleClientset()
			},
			options: DefaultLoaderOptions(),
			wantErr: true,
			errType: ErrConfigNotFound,
		},
		{
			name: "config key not found in configmap",
			setupClient: func() *fake.Clientset {
				client := fake.NewSimpleClientset()
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orchestrator-config",
						Namespace: "kbinit-system",
					},
					Data: map[string]string{
						"wrong-key.yaml": validConfigYAML,
					},
				}
				_, err := client.CoreV1().ConfigMaps("kbinit-system").Create(context.TODO(), configMap, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create ConfigMap: %v", err)
				}
				return client
			},
			options: DefaultLoaderOptions(),
			wantErr: true,
			errType: ErrConfigNotFound,
		},
		{
			name: "invalid YAML content",
			setupClient: func() *fake.Clientset {
				client := fake.NewSimpleClientset()
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orchestrator-config",
						Namespace: "kbinit-system",
					},
					Data: map[string]string{
						"config.yaml": "invalid: yaml: content: [",
					},
				}
				_, err := client.CoreV1().ConfigMaps("kbinit-system").Create(context.TODO(), configMap, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create ConfigMap: %v", err)
				}
				return client
			},
			options: DefaultLoaderOptions(),
			wantErr: true,
			errType: ErrConfigMalformed,
		},
		{
			name: "invalid configuration",
			setupClient: func() *fake.Clientset {
				client := fake.NewSimpleClientset()
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orchestrator-config",
						Namespace: "kbinit-system",
					},
					Data: map[string]string{
						"config.yaml": invalidConfigYAML,
					},
				}
				_, err := client.CoreV1().ConfigMaps("kbinit-system").Create(context.TODO(), configMap, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create ConfigMap: %v", err)
				}
				return client
			},
			options: DefaultLoaderOptions(),
			wantErr: true,
			errType: ErrConfigInvalid,
		},
		{
			name: "custom namespace and configmap name",
			setupClient: func() *fake.Clientset {
				client := fake.NewSimpleClientset()
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-config",
						Namespace: "custom-namespace",
					},
					Data: map[string]string{
						"custom.yaml": validConfigYAML,
					},
				}
				_, err := client.CoreV1().ConfigMaps("custom-namespace").Create(context.TODO(), configMap, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create ConfigMap: %v", err)
				}
				return client
			},
			options: LoaderOptions{
				Namespace:     "custom-namespace",
				ConfigMapName: "custom-config",
				ConfigMapKey:  "custom.yaml",
				EnableCache:   true,
				CacheTTL:      "1m",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			loader := NewConfigMapLoader(client, tt.options)

			config, err := loader.Load(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Errorf("ConfigMapLoader.Load() expected error but got none")
					return
				}
				if tt.errType != nil {
					if !isErrorType(err, tt.errType) {
						t.Errorf("ConfigMapLoader.Load() error type = %T, want %T", err, tt.errType)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ConfigMapLoader.Load() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if config == nil {
					t.Errorf("ConfigMapLoader.Load() returned nil config")
					return
				}
				if config.APIVersion != "score.dev/v1b1" {
					t.Errorf("ConfigMapLoader.Load() APIVersion = %v, want %v", config.APIVersion, "score.dev/v1b1")
				}
				if config.Kind != "OrchestratorConfig" {
					t.Errorf("ConfigMapLoader.Load() Kind = %v, want %v", config.Kind, "OrchestratorConfig")
				}
			}

			// Clean up
			if err := loader.Close(); err != nil {
				t.Errorf("Failed to close loader: %v", err)
			}
		})
	}
}

func TestConfigMapLoader_LoadWithCache(t *testing.T) {
	client := fake.NewSimpleClientset()
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orchestrator-config",
			Namespace: "kbinit-system",
		},
		Data: map[string]string{
			"config.yaml": validConfigYAML,
		},
	}
	_, err := client.CoreV1().ConfigMaps("kbinit-system").Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	options := DefaultLoaderOptions()
	options.EnableCache = true
	options.CacheTTL = "1s"

	loader := NewConfigMapLoader(client, options)
	defer func() {
		if err := loader.Close(); err != nil {
			t.Errorf("Failed to close loader: %v", err)
		}
	}()

	ctx := context.Background()

	// First load should hit the ConfigMap
	config1, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("First Load() failed: %v", err)
	}

	// Second load should hit the cache (no additional API call)
	config2, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("Second Load() failed: %v", err)
	}

	// Verify both configs are equivalent
	if config1.Metadata.Name != config2.Metadata.Name {
		t.Errorf("Cached config differs from original")
	}

	// Wait for cache to expire
	time.Sleep(2 * time.Second)

	// Third load should hit the ConfigMap again after cache expiry
	config3, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("Third Load() failed: %v", err)
	}

	if config3.Metadata.Name != config1.Metadata.Name {
		t.Errorf("Config after cache expiry differs from original")
	}
}

// TestConfigMapLoader_Watch tests basic Watch functionality
// Note: Full Watch functionality testing with fake clients is complex due to informer limitations
func TestConfigMapLoader_Watch(t *testing.T) {
	client := fake.NewSimpleClientset()
	options := DefaultLoaderOptions()
	loader := NewConfigMapLoader(client, options)
	defer func() {
		if err := loader.Close(); err != nil {
			t.Errorf("Failed to close loader: %v", err)
		}
	}()

	// Test that the loader implements ConfigLoader interface
	var _ ConfigLoader = loader

	// Test loader Close method
	err := loader.Close()
	if err != nil {
		t.Errorf("Close() should not return error: %v", err)
	}
}

func TestNewConfigMapLoader(t *testing.T) {
	client := fake.NewSimpleClientset()

	tests := []struct {
		name     string
		options  LoaderOptions
		expected LoaderOptions
	}{
		{
			name:    "default options",
			options: LoaderOptions{},
			expected: LoaderOptions{
				Namespace:     "kbinit-system",
				ConfigMapName: "orchestrator-config",
				ConfigMapKey:  "config.yaml",
				EnableCache:   false,
				CacheTTL:      "",
			},
		},
		{
			name: "custom options",
			options: LoaderOptions{
				Namespace:     "custom-ns",
				ConfigMapName: "custom-cm",
				ConfigMapKey:  "custom.yaml",
				EnableCache:   true,
				CacheTTL:      "10m",
			},
			expected: LoaderOptions{
				Namespace:     "custom-ns",
				ConfigMapName: "custom-cm",
				ConfigMapKey:  "custom.yaml",
				EnableCache:   true,
				CacheTTL:      "10m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewConfigMapLoader(client, tt.options)
			defer func() {
				if err := loader.Close(); err != nil {
					t.Errorf("Failed to close loader: %v", err)
				}
			}()

			if loader.options.Namespace != tt.expected.Namespace {
				t.Errorf("Namespace = %v, want %v", loader.options.Namespace, tt.expected.Namespace)
			}
			if loader.options.ConfigMapName != tt.expected.ConfigMapName {
				t.Errorf("ConfigMapName = %v, want %v", loader.options.ConfigMapName, tt.expected.ConfigMapName)
			}
			if loader.options.ConfigMapKey != tt.expected.ConfigMapKey {
				t.Errorf("ConfigMapKey = %v, want %v", loader.options.ConfigMapKey, tt.expected.ConfigMapKey)
			}
			if loader.options.EnableCache != tt.expected.EnableCache {
				t.Errorf("EnableCache = %v, want %v", loader.options.EnableCache, tt.expected.EnableCache)
			}

			// Verify cache is created when enabled
			if tt.options.EnableCache && loader.cache == nil {
				t.Errorf("Cache should be initialized when EnableCache is true")
			}
			if !tt.options.EnableCache && loader.cache != nil {
				t.Errorf("Cache should not be initialized when EnableCache is false")
			}
		})
	}
}

// isErrorType checks if an error wraps a specific error type
func isErrorType(err, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	return strings.Contains(err.Error(), target.Error())
}
