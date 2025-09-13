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
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/yaml"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// ConfigMapLoader implements ConfigLoader for Kubernetes ConfigMap-based configuration
type ConfigMapLoader struct {
	client     kubernetes.Interface
	options    LoaderOptions
	cache      *configCache
	validator  *Validator
	watchMutex sync.RWMutex
	watchers   map[string]chan ConfigEvent
	stopCh     chan struct{}
	informer   cache.SharedInformer
}

// NewConfigMapLoader creates a new ConfigMapLoader with the given Kubernetes client and options
func NewConfigMapLoader(client kubernetes.Interface, options LoaderOptions) *ConfigMapLoader {
	if options.Namespace == "" {
		options.Namespace = "score-system"
	}
	if options.ConfigMapName == "" {
		options.ConfigMapName = "orchestrator-config"
	}
	if options.ConfigMapKey == "" {
		options.ConfigMapKey = "config.yaml"
	}

	loader := &ConfigMapLoader{
		client:    client,
		options:   options,
		validator: NewValidator(),
		watchers:  make(map[string]chan ConfigEvent),
		stopCh:    make(chan struct{}),
	}

	if options.EnableCache {
		ttl, err := time.ParseDuration(options.CacheTTL)
		if err != nil {
			ttl = 5 * time.Minute
		}
		loader.cache = newConfigCache(ttl)
	}

	return loader
}

// LoadConfig loads the orchestrator configuration from the ConfigMap
func (l *ConfigMapLoader) LoadConfig(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
	// Check cache first if enabled
	if l.cache != nil {
		if config := l.cache.get(); config != nil {
			return config, nil
		}
	}

	// Load from ConfigMap
	configMap, err := l.client.CoreV1().ConfigMaps(l.options.Namespace).Get(
		ctx,
		l.options.ConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: ConfigMap %s/%s not found", ErrConfigNotFound, l.options.Namespace, l.options.ConfigMapName)
		}
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", l.options.Namespace, l.options.ConfigMapName, err)
	}

	// Extract YAML content
	yamlContent, exists := configMap.Data[l.options.ConfigMapKey]
	if !exists {
		return nil, fmt.Errorf("%w: key %s not found in ConfigMap %s/%s", ErrConfigMalformed, l.options.ConfigMapKey, l.options.Namespace, l.options.ConfigMapName)
	}

	// Parse YAML
	config, err := l.parseConfig(yamlContent)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigMalformed, err)
	}

	// Validate configuration
	if err := l.validator.Validate(config); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	}

	// Cache the configuration if caching is enabled
	if l.cache != nil {
		l.cache.set(config)
	}

	return config, nil
}

// Watch returns a channel that receives configuration updates
func (l *ConfigMapLoader) Watch(ctx context.Context) (<-chan ConfigEvent, error) {
	eventCh := make(chan ConfigEvent, 10)

	// Create a unique ID for this watcher
	watcherID := fmt.Sprintf("watcher-%d", time.Now().UnixNano())

	l.watchMutex.Lock()
	l.watchers[watcherID] = eventCh

	// Start the informer if it's not already running
	if l.informer == nil {
		if err := l.startInformer(); err != nil {
			l.watchMutex.Unlock()
			close(eventCh)
			return nil, fmt.Errorf("failed to start configuration watcher: %w", err)
		}
	}
	l.watchMutex.Unlock()

	// Start a goroutine to clean up when the context is done
	go func() {
		<-ctx.Done()
		l.watchMutex.Lock()
		delete(l.watchers, watcherID)
		close(eventCh)
		l.watchMutex.Unlock()
	}()

	return eventCh, nil
}

// Close releases resources held by the loader
func (l *ConfigMapLoader) Close() error {
	l.watchMutex.Lock()
	defer l.watchMutex.Unlock()

	// Close all watcher channels
	for id, ch := range l.watchers {
		close(ch)
		delete(l.watchers, id)
	}

	// Stop the informer
	if l.stopCh != nil {
		close(l.stopCh)
		l.stopCh = make(chan struct{})
	}

	return nil
}

// startInformer starts the Kubernetes informer for watching ConfigMap changes
func (l *ConfigMapLoader) startInformer() error {
	listWatcher := cache.NewListWatchFromClient(
		l.client.CoreV1().RESTClient(),
		"configmaps",
		l.options.Namespace,
		fields.OneTermEqualSelector("metadata.name", l.options.ConfigMapName),
	)

	l.informer = cache.NewSharedInformer(listWatcher, &corev1.ConfigMap{}, time.Minute)

	_, err := l.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			l.handleConfigMapEvent(ConfigEventAdded, obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			l.handleConfigMapEvent(ConfigEventModified, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			l.handleConfigMapEvent(ConfigEventDeleted, obj)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	go l.informer.Run(l.stopCh)

	// Wait for the cache to sync
	if !cache.WaitForCacheSync(l.stopCh, l.informer.HasSynced) {
		return fmt.Errorf("timed out waiting for cache sync")
	}

	return nil
}

// handleConfigMapEvent processes ConfigMap events and notifies watchers
func (l *ConfigMapLoader) handleConfigMapEvent(eventType ConfigEventType, obj interface{}) {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		l.broadcastEvent(ConfigEvent{
			Type:  ConfigEventError,
			Error: fmt.Errorf("unexpected object type: %T", obj),
		})
		return
	}

	var config *scorev1b1.OrchestratorConfig
	var err error

	if eventType != ConfigEventDeleted {
		yamlContent, exists := configMap.Data[l.options.ConfigMapKey]
		if !exists {
			l.broadcastEvent(ConfigEvent{
				Type:  ConfigEventError,
				Error: fmt.Errorf("key %s not found in ConfigMap", l.options.ConfigMapKey),
			})
			return
		}

		config, err = l.parseConfig(yamlContent)
		if err != nil {
			l.broadcastEvent(ConfigEvent{
				Type:  ConfigEventError,
				Error: fmt.Errorf("failed to parse configuration: %w", err),
			})
			return
		}

		if err := l.validator.Validate(config); err != nil {
			l.broadcastEvent(ConfigEvent{
				Type:  ConfigEventError,
				Error: fmt.Errorf("configuration validation failed: %w", err),
			})
			return
		}

		// Update cache if enabled
		if l.cache != nil {
			if eventType == ConfigEventDeleted {
				l.cache.invalidate()
			} else {
				l.cache.set(config)
			}
		}
	} else {
		// Handle deletion
		if l.cache != nil {
			l.cache.invalidate()
		}
	}

	l.broadcastEvent(ConfigEvent{
		Type:   eventType,
		Config: config,
	})
}

// broadcastEvent sends an event to all active watchers
func (l *ConfigMapLoader) broadcastEvent(event ConfigEvent) {
	l.watchMutex.RLock()
	defer l.watchMutex.RUnlock()

	for _, ch := range l.watchers {
		select {
		case ch <- event:
		default:
			// Channel is full, skip this watcher
		}
	}
}

// parseConfig parses YAML configuration into OrchestratorConfig
func (l *ConfigMapLoader) parseConfig(yamlContent string) (*scorev1b1.OrchestratorConfig, error) {
	var config scorev1b1.OrchestratorConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	return &config, nil
}
